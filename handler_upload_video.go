package main

import (
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
	/*body := */ http.MaxBytesReader(w, r.Body, 10<<30)

	// JWT
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "unauthorized", err)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "unauthorized", err)
		return
	}

	// Video ID
	videoIDsrt := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDsrt)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "error parsing video ID", err)
		return
	}

	// Video metadata
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "video not found", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	// Video
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "error parsing video", err)
		return
	}
	defer file.Close()
	videoTypeMime, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "error parsing media type", err)
		return
	}
	if videoTypeMime != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "wrong video format", fmt.Errorf("wrong video format: %s", videoTypeMime))
		return
	}
	videoType := strings.Split(videoTypeMime, "/")[1]

	// Local temporary file
	localFile, err := os.CreateTemp("", "tubely-videoupload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error saving video", err)
		return
	}
	defer localFile.Close()
	defer os.Remove(localFile.Name())
	io.Copy(localFile, file)
	localFile.Seek(0, io.SeekStart)

	processedVideoPath, err := processVideoForFastStart(localFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error processing video", err)
		return
	}
	processedFile, err := os.Open(processedVideoPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error getting processed video", err)
		return
	}
	defer processedFile.Close()
	defer os.Remove(processedFile.Name())

	// Putting into S3 Bucket
	videoAspectStr, err := getVideoAspectRatio(localFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error getting video aspect ratio", err)
		return
	}
	var videoAspect string
	switch videoAspectStr {
	case "16:9":
		videoAspect = "landscape"
	case "9:16":
		videoAspect = "portrait"
	case "other":
		videoAspect = "other"
	}
	fileKeyBytes := make([]byte, 32)
	rand.Read(fileKeyBytes)
	fileKey := fmt.Sprintf("%s/%s.%s", videoAspect, base64.RawURLEncoding.EncodeToString(fileKeyBytes), videoType)

	cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileKey,
		Body:        processedFile,
		ContentType: &videoTypeMime,
	})

	// Updating video in DB
	//videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fileKey)
	//videoURL := fmt.Sprintf("%s,%s", cfg.s3Bucket, fileKey)
	videoURL := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, fileKey)
	video.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error updating video", err)
		return
	}

	// Getting signed URL
	/*video, err = cfg.dbVideoToSignedVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error getting signed URL", err)
		return
	}*/

	respondWithJSON(w, http.StatusOK, video)
}
