# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run the server (serves on http://localhost:8080)
go run main.go

# Build binary
go build -o youtube-dl .

# Add a dependency
go get <package>

# Tidy dependencies
go mod tidy
```

## Architecture

Single-file Go web server (`main.go`) with a vanilla JS frontend (`static/index.html`).

**Request flow:**
1. `GET /api/formats?url=<youtube-url>` — fetches video metadata and available formats via `github.com/kkdai/youtube/v2`, returns `VideoInfo` (title, author, `[]FormatInfo`)
2. `POST /api/download` with `{url, itag}` — streams the selected format from YouTube and writes it to `output/<sanitized-title>_<itag>.<ext>`
3. `GET /` — serves `static/` as a file server (the HTML UI)

**Key constraint:** High-resolution formats (1080p+) on YouTube are served as separate video-only and audio-only streams (adaptive streaming). This app downloads a single stream per request — no muxing. Formats with combined audio+video are typically ≤720p (`video/mp4` mime type). Audio-only formats are `audio/mp4` (saved as `.m4a`) or `audio/webm`/`video/webm` (saved as `.webm`).

**Output naming:** `output/<title>_<itag>.<ext>` — title is sanitized to ≤100 chars with special characters replaced by `_`.

**No authentication, no concurrency control** — the download handler blocks until the full stream is copied. Long downloads will hold the HTTP connection open.
