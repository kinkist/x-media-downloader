package twitterscraper

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	vidio "github.com/AlexEidt/Vidio"
)

type Media struct {
	ID        int
	Type      string
	Size      int
	Parts     int
	ExpiresAt time.Time
}

type uploadInitResponse struct {
	ID           int `json:"media_id"`
	ExpiresAfter int `json:"expires_after_secs"`
}

type ProcessingInfo struct {
	State      string `json:"state"`
	CheckAfter int    `json:"check_after_secs"`
	Progress   int    `json:"progress_percent"`
}

type uploadStatusResponse struct {
	ProcessingInfo ProcessingInfo `json:"processing_info"`
}

// Uploads photo, video or gif for further posting or scheduling. Expires in 24 hours if not used.
func (s *Scraper) UploadMedia(filePath string) (*Media, error) {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	media, err := s.uploadInit(filePath, fileContent)
	if err != nil {
		return nil, err
	}

	err = s.uploadAppend(media, fileContent)
	if err != nil {
		return nil, err
	}

	var status *ProcessingInfo

	status, err = s.uploadFinalize(media)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(media.Type, "image") {
		return media, nil
	}

	for status.State != "succeeded" {
		time.Sleep(2 * time.Second)
		status, err = s.uploadStatus(media)
		if err != nil {
			return nil, err
		}
	}

	return media, nil
}

func (s *Scraper) uploadInit(filePath string, fileContent []byte) (*Media, error) {
	var (
		videoDuration float64
		fileType      string
		mediaCategory = "tweet_"
	)

	fileType = http.DetectContentType(fileContent)

	if fileType == "image/jpeg" || fileType == "image/png" {
		mediaCategory += "image"
	} else if fileType == "image/gif" {
		mediaCategory += "gif"
	} else if fileType == "video/mp4" || fileType == "video/quicktime" {
		mediaCategory += "video"

		video, err := vidio.NewVideo(filePath)
		if err != nil {
			return nil, err
		}
		videoDuration = video.Duration()
		video.Close()
	} else {
		return nil, fmt.Errorf("file type %s unsupported by twitter, make sure you uploading photo, video or gif", fileType)
	}

	req, err := s.newRequest("POST", "https://upload.x.com/i/media/upload.json")
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("command", "INIT")
	query.Set("total_bytes", strconv.Itoa(len(fileContent)))
	query.Set("media_type", fileType)
	query.Set("media_category", mediaCategory)
	if mediaCategory == "tweet_video" {
		query.Set("video_duration_ms", strconv.FormatFloat(videoDuration*1000, 'f', -1, 64))
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Origin", "https://x.com")
	req.Header.Set("Referer", "https://x.com/")

	var uploadInit uploadInitResponse

	err = s.RequestAPI(req, &uploadInit)
	if err != nil {
		return nil, err
	}

	return &Media{
		ID:        uploadInit.ID,
		Type:      fileType,
		Size:      len(fileContent),
		ExpiresAt: time.Now().Add(time.Duration(uploadInit.ExpiresAfter) * time.Second),
		Parts:     len(fileContent) / 2_000_000,
	}, nil
}

func (s *Scraper) uploadAppend(media *Media, fileContent []byte) error {
	for i := 0; i <= media.Parts; i++ {
		var partData []byte

		if i+1 <= media.Parts {
			partData = fileContent[i*2_000_000 : (i+1)*2_000_000]
		} else {
			partData = fileContent[i*2_000_000:]
		}

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		fw, err := w.CreateFormFile("media", "blob")
		if err != nil {
			log.Fatal(err)
		}
		if _, err = io.Copy(fw, bytes.NewReader(partData)); err != nil {
			return err
		}
		w.Close()

		req, err := s.newRequest("POST", "https://upload.x.com/i/media/upload.json")
		if err != nil {
			return err
		}

		query := url.Values{}
		query.Set("command", "APPEND")
		query.Set("media_id", strconv.Itoa(media.ID))
		query.Set("segment_index", strconv.Itoa(i))
		req.URL.RawQuery = query.Encode()
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Origin", "https://x.com")
		req.Header.Set("Referer", "https://x.com/")
		req.Body = io.NopCloser(&buf)

		err = s.RequestAPI(req, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Scraper) uploadFinalize(media *Media) (*ProcessingInfo, error) {
	req, err := s.newRequest("POST", "https://upload.x.com/i/media/upload.json")
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("command", "FINALIZE")
	query.Set("media_id", strconv.Itoa(media.ID))
	query.Set("allow_async", "true")
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Origin", "https://x.com")
	req.Header.Set("Referer", "https://x.com/")

	var response uploadStatusResponse

	err = s.RequestAPI(req, &response)
	if err != nil {
		return nil, err
	}

	return &response.ProcessingInfo, nil
}

func (s *Scraper) uploadStatus(media *Media) (*ProcessingInfo, error) {
	req, err := s.newRequest("GET", "https://upload.x.com/i/media/upload.json")
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("command", "STATUS")
	query.Set("media_id", strconv.Itoa(media.ID))
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Origin", "https://x.com")
	req.Header.Set("Referer", "https://x.com/")

	var response uploadStatusResponse

	err = s.RequestAPI(req, &response)
	if err != nil {
		return nil, err
	}

	return &response.ProcessingInfo, nil
}
