package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// handleDownload streams download progress via Server-Sent Events (GET).
// Query params: url, itag, videoOnly (true/false)
func handleDownload(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	videoURL := r.URL.Query().Get("url")
	itag := r.URL.Query().Get("itag")
	videoOnly := r.URL.Query().Get("videoOnly") == "true"

	if videoURL == "" || itag == "" {
		send(map[string]string{"type": "error", "message": "url and itag are required"})
		return
	}

	slog.Info("download request", "url", videoURL, "itag", itag, "video_only", videoOnly)

	formatSpec := itag
	if videoOnly {
		formatSpec = itag + "+bestaudio[ext=m4a]/bestaudio"
	}

	outTemplate := filepath.Join(outputDir, "%(title).100B_%(format_id)s.%(ext)s")

	var stdoutBuf bytes.Buffer
	cmd := exec.Command("yt-dlp",
		"-f", formatSpec,
		"--merge-output-format", "mp4",
		"-o", outTemplate,
		"--no-part",
		"--newline",
		"--print", "after_move:filepath",
		"--no-playlist",
		videoURL,
	)
	cmd.Stdout = &stdoutBuf

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		send(map[string]string{"type": "error", "message": err.Error()})
		return
	}

	if err := cmd.Start(); err != nil {
		send(map[string]string{"type": "error", "message": err.Error()})
		return
	}

	slog.Info("yt-dlp started", "format", formatSpec)

	// Track which phase (video download → audio download → merge)
	downloadPhase := 0 // 0=video, 1=audio
	var lastErrLine string

	scanner := bufio.NewScanner(stderrPipe)
	for scanner.Scan() {
		line := scanner.Text()
		slog.Debug("yt-dlp", "line", line)

		switch {
		case strings.HasPrefix(line, "[download] Destination:"):
			// New file being downloaded — advance phase on second occurrence
			if downloadPhase > 0 {
				downloadPhase++
			} else {
				downloadPhase = 1
			}

		case strings.HasPrefix(line, "[download]") && strings.Contains(line, "%"):
			pct, speed, eta := parseProgress(line)
			phase := "video"
			if videoOnly && downloadPhase > 1 {
				phase = "audio"
			}
			send(map[string]any{
				"type":  "progress",
				"phase": phase,
				"pct":   pct,
				"speed": speed,
				"eta":   eta,
			})

		case strings.HasPrefix(line, "[Merger]") || strings.HasPrefix(line, "[ffmpeg]"):
			send(map[string]any{"type": "merging"})

		case strings.Contains(line, "ERROR"):
			lastErrLine = line
			slog.Error("yt-dlp error line", "line", line)
		}
	}

	if err := cmd.Wait(); err != nil {
		msg := lastErrLine
		if msg == "" {
			msg = err.Error()
		}
		slog.Error("yt-dlp failed", "err", err, "msg", msg)
		send(map[string]string{"type": "error", "message": msg})
		return
	}

	outPath := strings.TrimSpace(stdoutBuf.String())
	filename := filepath.Base(outPath)
	slog.Info("download complete", "path", outPath)
	send(map[string]any{"type": "done", "filename": filename})
}

// parseProgress extracts pct, speed, eta from a yt-dlp [download] progress line.
func parseProgress(line string) (pct float64, speed, eta string) {
	parts := strings.Fields(line)
	// parts: ["[download]", "23.4%", "of", "1.10GiB", "at", "5.23MiB/s", "ETA", "03:12"]
	if len(parts) < 2 {
		return
	}
	pct, _ = strconv.ParseFloat(strings.TrimSuffix(parts[1], "%"), 64)
	for i, p := range parts {
		if p == "at" && i+1 < len(parts) {
			speed = parts[i+1]
		}
		if p == "ETA" && i+1 < len(parts) {
			eta = parts[i+1]
		}
	}
	return
}
