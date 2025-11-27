package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	const maxMemory = 10 << 20 // 10 MB
	r.ParseMultipartForm(maxMemory)

	// "thumbnail" should match the HTML form input name - Extract the file from form data
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// Extract media type from the uploaded file header
	mediaType := header.Header.Get("Content-Type")

	// Validate media type - Only accept images (jpeg, png)
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}

	if mimeType != "image/jpeg" && mimeType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Unsupported media type. Only JPEG and PNG are allowed.", nil)
		return
	}

	// We'll stream the uploaded file directly to disk instead of reading it all into memory.
	// This avoids using a []byte as an io.Reader and is more memory efficient for larger files.

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

	// Create thumbnail filepath, ready to store in "assets" directory

	// Fetch file extension from media type
	thumbnailFileExt := mediaType[strings.Index(mediaType, "/")+1:]

	// Gnerate a unique filename for the thumbnail
	key := make([]byte, 32)
	rand.Read(key)
	thumbnailFileName := fmt.Sprintf("%s.%s", base64.RawURLEncoding.EncodeToString(key), thumbnailFileExt)

	// Join with assets root to get full path
	thumbnailPath := filepath.Join(cfg.assetsRoot, thumbnailFileName)

	// Save the thumbnail file to the assets directory
	tnFile, err := os.Create(thumbnailPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create thumbnail file", err)
		return
	}
	defer tnFile.Close()

	if _, err := io.Copy(tnFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to save thumbnail file", err)
		return
	}

	// Create URL for the thumbnail
	tnURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, thumbnailFileName)

	videoMeta.ThumbnailURL = &tnURL

	err = cfg.db.UpdateVideo(videoMeta)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video thumbnail URL", err)
		return
	}

	// Marshal and send the response

	respondWithJSON(w, http.StatusOK, videoMeta)
}
