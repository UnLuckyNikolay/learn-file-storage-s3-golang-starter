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

	// Getting JWT
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	// Validating JWT
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// Prasing files
	err = r.ParseMultipartForm(10 << 20)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Files are too big", err)
		return
	}

	// Getting thumbnail
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse the file form", err)
		return
	}
	defer file.Close()

	// Getting mediatype
	mediatype, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if mediatype != "image/jpeg" && mediatype != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}

	// Getting video
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find the video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	// Saving thumbnail
	thumbTypeSplit := strings.Split(header.Header.Get("Content-Type"), "/")
	if len(thumbTypeSplit) != 2 {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}
	thumbNameBytes := make([]byte, 32)
	rand.Read(thumbNameBytes[:])
	thumbName := base64.RawURLEncoding.EncodeToString(thumbNameBytes)
	thumbFilepath := filepath.Join(cfg.assetsRoot, fmt.Sprintf("/%v.%s", thumbName, thumbTypeSplit[1]))
	thumbFile, err := os.Create(thumbFilepath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving the file", err)
		return
	}
	defer thumbFile.Close()
	io.Copy(thumbFile, file)

	// Updating video
	thumbURL := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, thumbName, thumbTypeSplit[1])
	video.ThumbnailURL = &thumbURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusServiceUnavailable, "Error updating video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
