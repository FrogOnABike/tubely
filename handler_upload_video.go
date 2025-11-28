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
	// Set upload size limit
	const maxUpload = 10 << 30 // 1 GB
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)

	// Extract video ID from URL path
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// Authenticate the user via JWT
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	// Validate JWT and get user ID
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	// Retrieve video metadata
	videoMeta, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to retrieve video metadata", err)
		return
	}

	// Check if the authenticated user is the owner of the video
	if videoMeta.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You do not have permission to upload a thumbnail for this video", nil)
		return
	}

	// Extract the file from form data - "video" should match the HTML form input name
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// Extract media type from the uploaded file header
	mediaType := header.Header.Get("Content-Type")

	// Validate media type - Only accept videos (mp4)
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}

	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Unsupported media type. Only MP4 videos are allowed.", nil)
		return
	}

	// Store video to temp file on disk
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy uploaded file to temp file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to save uploaded file", err)
		return
	}

	// Reset file pointer to beginning of temp file
	tempFile.Seek(0, io.SeekStart)

	// Fetch file extension from media type (I know, we only accept mp4 for now - but future proofing)
	videolFileExt := mediaType[strings.Index(mediaType, "/")+1:]

	// Generate a unique filename for the thumbnail
	key := make([]byte, 32)
	rand.Read(key)
	videoFileName := fmt.Sprintf("%s.%s", base64.RawURLEncoding.EncodeToString(key), videolFileExt)

	fmt.Println("uploading video", videoID, "to S3 by user", userID)

	// Define S3 parmeters for upload
	params := &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &videoFileName,
		Body:        tempFile,
		ContentType: &mediaType,
	}

	// Upload video to S3
	_, err = cfg.s3Client.PutObject(context.TODO(), params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to upload video to S3", err)
		return
	}

	// Create URL for the video in S3 and update in database
	vidURL := fmt.Sprintf("http://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, videoFileName)

	videoMeta.VideoURL = &vidURL

	err = cfg.db.UpdateVideo(videoMeta)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video URL", err)
		return
	}

	// Marshal and send the response

	respondWithJSON(w, http.StatusOK, videoMeta)
}
