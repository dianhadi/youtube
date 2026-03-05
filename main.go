package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkdai/youtube/v2"
)

const outputDir = "output"

type FormatInfo struct {
	Itag       int    `json:"itag"`
	Quality    string `json:"quality"`
	MimeType   string `json:"mimeType"`
	Resolution string `json:"resolution"`
	FPS        int    `json:"fps"`
	AudioOnly  bool   `json:"audioOnly"`
}

type VideoInfo struct {
	Title   string       `json:"title"`
	Author  string       `json:"author"`
	Formats []FormatInfo `json:"formats"`
}

type DownloadRequest struct {
	URL  string `json:"url"`
	Itag int    `json:"itag"`
}

type DownloadResponse struct {
	Success  bool   `json:"success"`
	Filename string `json:"filename,omitempty"`
	Error    string `json:"error,omitempty"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}))
	slog.SetDefault(logger)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		slog.Error("failed to create output dir", "err", err)
		os.Exit(1)
	}

	http.Handle("/", http.FileServer(http.Dir("static")))
	http.HandleFunc("/api/formats", handleFormats)
	http.HandleFunc("/api/download", handleDownload)

	port := "8080"
	slog.Info("server running", "url", "http://localhost:"+port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func handleFormats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	videoURL := r.URL.Query().Get("url")
	if videoURL == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "url parameter required"})
		return
	}

	slog.Info("fetching video info", "url", videoURL)

	client := youtube.Client{}
	video, err := client.GetVideo(videoURL)
	if err != nil {
		slog.Error("GetVideo failed", "url", videoURL, "err", err)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	slog.Info("video found", "title", video.Title, "id", video.ID, "formats", len(video.Formats))

	var formats []FormatInfo
	seen := map[string]bool{}

	for _, f := range video.Formats {
		mimeBase := strings.Split(f.MimeType, ";")[0]
		isAudioOnly := strings.HasPrefix(mimeBase, "audio/")

		resolution := f.QualityLabel
		if resolution == "" && isAudioOnly {
			resolution = "audio only"
		}

		key := fmt.Sprintf("%d", f.ItagNo)
		if seen[key] {
			continue
		}
		seen[key] = true

		formats = append(formats, FormatInfo{
			Itag:       f.ItagNo,
			Quality:    f.Quality,
			MimeType:   mimeBase,
			Resolution: resolution,
			FPS:        f.FPS,
			AudioOnly:  isAudioOnly,
		})
	}

	info := VideoInfo{
		Title:   video.Title,
		Author:  video.Author,
		Formats: formats,
	}

	json.NewEncoder(w).Encode(info)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(DownloadResponse{Error: "method not allowed"})
		return
	}

	var req DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(DownloadResponse{Error: "invalid request body"})
		return
	}

	if req.URL == "" || req.Itag == 0 {
		json.NewEncoder(w).Encode(DownloadResponse{Error: "url and itag are required"})
		return
	}

	slog.Info("download request", "url", req.URL, "itag", req.Itag)

	client := youtube.Client{}
	video, err := client.GetVideo(req.URL)
	if err != nil {
		slog.Error("GetVideo failed", "url", req.URL, "err", err)
		json.NewEncoder(w).Encode(DownloadResponse{Error: err.Error()})
		return
	}

	slog.Info("video found", "title", video.Title, "id", video.ID)

	var selectedFormat *youtube.Format
	for i, f := range video.Formats {
		if f.ItagNo == req.Itag {
			selectedFormat = &video.Formats[i]
			break
		}
	}

	if selectedFormat == nil {
		slog.Warn("itag not found", "itag", req.Itag, "total_formats", len(video.Formats))
		json.NewEncoder(w).Encode(DownloadResponse{Error: "format not found"})
		return
	}

	slog.Debug("selected format",
		"itag", selectedFormat.ItagNo,
		"mime", selectedFormat.MimeType,
		"quality", selectedFormat.QualityLabel,
		"url_len", len(selectedFormat.URL),
	)
	if selectedFormat.URL == "" {
		slog.Warn("format URL is empty, may require decryption or is unavailable",
			"itag", selectedFormat.ItagNo,
			"mime", selectedFormat.MimeType,
		)
	}

	slog.Info("opening stream", "itag", selectedFormat.ItagNo)
	stream, _, err := client.GetStream(video, selectedFormat)
	if err != nil {
		slog.Error("GetStream failed",
			"itag", selectedFormat.ItagNo,
			"mime", selectedFormat.MimeType,
			"err", err,
		)
		json.NewEncoder(w).Encode(DownloadResponse{Error: err.Error()})
		return
	}
	defer stream.Close()
	slog.Info("stream opened", "itag", selectedFormat.ItagNo)

	ext := "mp4"
	mimeBase := strings.Split(selectedFormat.MimeType, ";")[0]
	switch mimeBase {
	case "video/webm", "audio/webm":
		ext = "webm"
	case "audio/mp4":
		ext = "m4a"
	}

	safeTitle := sanitizeFilename(video.Title)
	filename := fmt.Sprintf("%s_%d.%s", safeTitle, req.Itag, ext)
	outPath := filepath.Join(outputDir, filename)

	outFile, err := os.Create(outPath)
	if err != nil {
		slog.Error("failed to create output file", "path", outPath, "err", err)
		json.NewEncoder(w).Encode(DownloadResponse{Error: err.Error()})
		return
	}
	defer outFile.Close()

	slog.Info("writing file", "path", outPath)
	n, err := io.Copy(outFile, stream)
	if err != nil {
		slog.Error("io.Copy failed", "path", outPath, "bytes_written", n, "err", err)
		os.Remove(outPath)
		json.NewEncoder(w).Encode(DownloadResponse{Error: err.Error()})
		return
	}

	slog.Info("download complete", "path", outPath, "size_mb", fmt.Sprintf("%.2f", float64(n)/1024/1024))
	json.NewEncoder(w).Encode(DownloadResponse{Success: true, Filename: filename})
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	result := replacer.Replace(name)
	if len(result) > 100 {
		result = result[:100]
	}
	return strings.TrimSpace(result)
}
