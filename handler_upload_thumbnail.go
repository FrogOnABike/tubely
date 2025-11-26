package main

import (
	"encoding/base64"
	"fmt"
	"net/http"

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

	// Read image data into a byte slice
	imageData := make([]byte, header.Size)
	_, err = file.Read(imageData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to read file data", err)
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

	// Convert image data to a base64-encoded string
	imageStr := base64.StdEncoding.EncodeToString(imageData)

	// Create a data URL for the thumbnail
	dataURL := fmt.Sprintf("data:%s;base64,%s", mediaType, imageStr)

	// // Store the thumbnail in the in-memory map
	// videoThumbnails[videoID] = thumbnail{
	// 	mediaType: mediaType,
	// 	data:      imageData,
	// }

	// Update the video's thumbnail URL in the database
	// updatedTNUrl := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, videoID.String())
	videoMeta.ThumbnailURL = &dataURL
	err = cfg.db.UpdateVideo(videoMeta)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video thumbnail URL", err)
		return
	}

	// Marshal and send the response

	respondWithJSON(w, http.StatusOK, videoMeta)
}
