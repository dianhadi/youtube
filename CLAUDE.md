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

Single-file Go web server (`main.go`) with a vanilla JS frontend (`static/index.html`). Requires `ffmpeg` installed on the system for HD downloads.

**Request flow:**
1. `GET /api/formats?url=<youtube-url>` — fetches video metadata via `github.com/kkdai/youtube/v2`, returns `VideoInfo` (title, author, `[]FormatInfo`). Each `FormatInfo` includes `audioOnly` and `videoOnly` booleans.
2. `POST /api/download` with `{url, itag}` — two paths depending on format type:
   - **Combined stream** (≤720p, `audioOnly=false`, `videoOnly=false`): calls `downloadStream()` and writes to `output/<title>_<itag>.<ext>`
   - **Video-only stream** (1080p+, `videoOnly=true`): calls `downloadAndMerge()` — downloads video + best matching audio stream to `.tmp_*` files, merges with `ffmpeg -c:v copy -c:a copy`, then removes temp files
3. `GET /` — serves `static/` as a file server

**Key helpers in main.go:**
- `downloadStream()` — wraps `GetStreamContext(context.Background(), ...)` with up to 3 retries and exponential backoff (3s, 6s). Always uses `context.Background()` so downloads are not tied to the HTTP request lifecycle.
- `downloadAndMerge()` — orchestrates video-only + audio download and ffmpeg merge.
- `findBestAudio()` — selects highest-bitrate audio format, preferring same container as video (e.g. `audio/mp4` for `video/mp4`).
- `mergeWithFFmpeg()` — runs ffmpeg with `-c:v copy -c:a copy` (no re-encode).

**Video-only detection:** `f.AudioChannels == 0` and mime is not `audio/*`.

**Output naming:** `output/<title>_<itag>.<ext>` — title sanitized to ≤100 chars. Temp files prefixed with `.tmp_video_` / `.tmp_audio_` and removed after merge.

**No concurrency control** — each download handler blocks until complete. Long downloads hold the HTTP connection open.
