package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const outputDir = "output"

type FormatInfo struct {
	Itag       string `json:"itag"`
	Quality    string `json:"quality"`
	MimeType   string `json:"mimeType"`
	Resolution string `json:"resolution"`
	FPS        int    `json:"fps"`
	AudioOnly  bool   `json:"audioOnly"`
	VideoOnly  bool   `json:"videoOnly"`
}

type VideoInfo struct {
	Title   string       `json:"title"`
	Author  string       `json:"author"`
	Formats []FormatInfo `json:"formats"`
}

type DownloadRequest struct {
	URL       string `json:"url"`
	Itag      string `json:"itag"`
	VideoOnly bool   `json:"videoOnly"`
}

type DownloadResponse struct {
	Success  bool   `json:"success"`
	Filename string `json:"filename,omitempty"`
	Error    string `json:"error,omitempty"`
}

// yt-dlp -J output structs
type ytDLPFormat struct {
	FormatID   string  `json:"format_id"`
	Ext        string  `json:"ext"`
	FPS        float64 `json:"fps"`
	ACodec     string  `json:"acodec"`
	VCodec     string  `json:"vcodec"`
	FormatNote string  `json:"format_note"`
	Height     int     `json:"height"`
}

type ytDLPInfo struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Uploader string        `json:"uploader"`
	Formats  []ytDLPFormat `json:"formats"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}))
	slog.SetDefault(logger)

	if _, err := exec.LookPath("yt-dlp"); err != nil {
		slog.Error("yt-dlp not found, install with: brew install yt-dlp")
		os.Exit(1)
	}

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

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("yt-dlp", "-J", "--no-playlist", videoURL)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		slog.Error("yt-dlp -J failed", "url", videoURL, "err", err, "stderr", errMsg)
		json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
		return
	}

	var info ytDLPInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		slog.Error("parse yt-dlp output failed", "err", err)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to parse video info"})
		return
	}

	slog.Info("video found", "title", info.Title, "id", info.ID, "formats", len(info.Formats))

	var formats []FormatInfo
	seen := map[string]bool{}

	for _, f := range info.Formats {
		if seen[f.FormatID] {
			continue
		}
		// Skip storyboard/manifest formats
		if f.Ext == "mhtml" || f.Ext == "none" || (f.ACodec == "none" && f.VCodec == "none") {
			continue
		}
		seen[f.FormatID] = true

		audioOnly := f.VCodec == "none"
		videoOnly := f.ACodec == "none" && !audioOnly

		mimeType := "video/" + f.Ext
		if audioOnly {
			mimeType = "audio/" + f.Ext
		}

		resolution := f.FormatNote
		if !audioOnly && f.Height > 0 && f.FormatNote == "" {
			resolution = fmt.Sprintf("%dp", f.Height)
		}
		if audioOnly && resolution == "" {
			resolution = "audio only"
		}

		formats = append(formats, FormatInfo{
			Itag:       f.FormatID,
			Quality:    f.FormatNote,
			MimeType:   mimeType,
			Resolution: resolution,
			FPS:        int(f.FPS),
			AudioOnly:  audioOnly,
			VideoOnly:  videoOnly,
		})
	}

	json.NewEncoder(w).Encode(VideoInfo{
		Title:   info.Title,
		Author:  info.Uploader,
		Formats: formats,
	})
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

	if req.URL == "" || req.Itag == "" {
		json.NewEncoder(w).Encode(DownloadResponse{Error: "url and itag are required"})
		return
	}

	slog.Info("download request", "url", req.URL, "itag", req.Itag, "video_only", req.VideoOnly)

	formatSpec := req.Itag
	if req.VideoOnly {
		formatSpec = req.Itag + "+bestaudio[ext=m4a]/bestaudio"
	}

	outTemplate := filepath.Join(outputDir, "%(title).100B_%(format_id)s.%(ext)s")

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("yt-dlp",
		"-f", formatSpec,
		"--merge-output-format", "mp4",
		"-o", outTemplate,
		"--no-part",
		"--print", "after_move:filepath",
		"--no-playlist",
		req.URL,
	)
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr) // real-time progress + capture for errors

	slog.Info("running yt-dlp", "format", formatSpec)

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		// yt-dlp writes full progress to stderr; extract just the last error line
		if lines := strings.Split(errMsg, "\n"); len(lines) > 0 {
			for i := len(lines) - 1; i >= 0; i-- {
				if strings.Contains(lines[i], "ERROR") {
					errMsg = strings.TrimSpace(lines[i])
					break
				}
			}
		}
		slog.Error("yt-dlp failed", "err", err, "msg", errMsg)
		json.NewEncoder(w).Encode(DownloadResponse{Error: errMsg})
		return
	}

	outPath := strings.TrimSpace(stdout.String())
	filename := filepath.Base(outPath)
	slog.Info("download complete", "path", outPath)
	json.NewEncoder(w).Encode(DownloadResponse{Success: true, Filename: filename})
}
