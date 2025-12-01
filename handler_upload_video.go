package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// Limit upload size
	const maxUpload = 10 << 30 // 1 GB
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)

	// Parse video ID from path
	videoIDStr := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video ID", err)
		return
	}

	// Authenticate
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Missing JWT", err)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid JWT", err)
		return
	}

	// Retrieve metadata and check ownership
	videoMeta, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to retrieve video metadata", err)
		return
	}
	if videoMeta.ID == uuid.Nil {
		respondWithError(w, http.StatusNotFound, "Video not found", nil)
		return
	}
	if videoMeta.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You don't own this video", nil)
		return
	}

	// Get uploaded file
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse uploaded file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if !strings.HasPrefix(mimeType, "video/") {
		respondWithError(w, http.StatusBadRequest, "Unsupported media type", nil)
		return
	}

	// Save to temp file then run ffprobe-based helper
	tmp, err := os.CreateTemp("", "tubely-upload-*.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create temp file", err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to save upload", err)
		return
	}

	// Pre-process video to enable fast start
	processedVideoPath, err := processVideoForFastStart(tmp.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to process video for fast start", err)
		return
	}
	defer os.Remove(processedVideoPath)
	tmp.Close() // close original temp file before re-opening processed file

	// Re-open processed video file
	tmp, err = os.Open(processedVideoPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to open processed video file", err)
		return
	}
	defer tmp.Close()

	// Determine aspect and construct storage key
	aspect, err := getVideoAspectRatio(tmp.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to determine video aspect", err)
		return
	}

	// if _, err := tmp.Seek(0, io.SeekStart); err != nil {
	// 	respondWithError(w, http.StatusInternalServerError, "Unable to rewind temp file", err)
	// 	return
	// }

	// determine extension from mime type
	partsIdx := strings.Index(mimeType, "/")
	ext := "mp4"
	if partsIdx > -1 && partsIdx < len(mimeType)-1 {
		ext = mimeType[partsIdx+1:]
	}

	var keyBytes [32]byte
	if _, err := rand.Read(keyBytes[:]); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to generate key", err)
		return
	}
	objectKey := fmt.Sprintf("%s/%s.%s", aspect, base64.RawURLEncoding.EncodeToString(keyBytes[:]), ext)

	params := &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &objectKey,
		Body:        tmp,
		ContentType: &mimeType,
	}

	if _, err := cfg.s3Client.PutObject(context.TODO(), params); err != nil {
		respondWithError(w, http.StatusInternalServerError, "S3 upload failed", err)
		return
	}

	// Update video metadata with Cloudfront URL
	url := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, objectKey)
	videoMeta.VideoURL = &url
	if err := cfg.db.UpdateVideo(videoMeta); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video metadata", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMeta)
}
