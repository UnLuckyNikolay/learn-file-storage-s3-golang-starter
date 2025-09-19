package main

import (
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

	err = r.ParseMultipartForm(10 << 20)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Files are too big", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse the file form", err)
		return
	}
	defer file.Close()

	mediatype, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if mediatype != "image/jpeg" && mediatype != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find the video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	thumbTypeSplit := strings.Split(header.Header.Get("Content-Type"), "/")
	if len(thumbTypeSplit) != 2 {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}
	thumbURL := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, videoID, thumbTypeSplit[1])
	video.ThumbnailURL = &thumbURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusServiceUnavailable, "Error updating video", err)
		return
	}

	thumbFilepath := filepath.Join(cfg.assetsRoot, fmt.Sprintf("/%v.%s", videoID, thumbTypeSplit[1]))
	thumbFile, err := os.Create(thumbFilepath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving the file", err)
		return
	}
	defer thumbFile.Close()

	io.Copy(thumbFile, file)

	respondWithJSON(w, http.StatusOK, video)
}
