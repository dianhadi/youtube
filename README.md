# YouTube Downloader

A simple web app to download YouTube videos. Enter a URL, pick a resolution, and download.

## Prerequisites

- [Go](https://golang.org/) 1.23+
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) — handles downloading and format extraction
- [ffmpeg](https://ffmpeg.org/) — required for merging 1080p+ video and audio streams

```bash
# macOS
brew install yt-dlp ffmpeg
```

## Usage

```bash
git clone https://github.com/dianhadi/youtube
cd youtube

go mod tidy
go run main.go
```

Open **http://localhost:8080** in your browser.

## How it works

1. Enter a YouTube URL and click **Check Formats**
2. Select a resolution or audio format
3. Click **Download** — the file is saved to the `output/` folder with a real-time progress bar

## Format types

| Badge | Type | Description |
|-------|------|-------------|
| **Video** | `video/mp4` | Combined video + audio (≤720p) |
| **HD** | `video/mp4` / `video/webm` | Video-only 1080p+, automatically merged with best audio via yt-dlp + ffmpeg |
| **Audio** | `audio/mp4` / `audio/webm` | Audio only (`.m4a` / `.webm`) |

HD formats automatically download the video and audio streams separately and merge them without re-encoding.

## Project structure

```
.
├── main.go         # Go web server & API handlers
├── static/
│   └── index.html  # Frontend UI
└── output/         # Downloaded files (created automatically)
```

## API

### `GET /api/formats?url=<youtube-url>`
Returns video metadata and available formats.

**Response:**
```json
{
  "title": "Video Title",
  "author": "Channel Name",
  "formats": [
    {
      "itag": "22",
      "quality": "hd720",
      "mimeType": "video/mp4",
      "resolution": "720p",
      "fps": 30,
      "audioOnly": false,
      "videoOnly": false
    }
  ]
}
```

### `GET /api/download?url=<youtube-url>&itag=<id>&videoOnly=<bool>`
Streams download progress via **Server-Sent Events**. Each event is a JSON object:

| `type` | Payload | Description |
|--------|---------|-------------|
| `progress` | `pct`, `phase`, `speed`, `eta` | Download progress |
| `merging` | — | ffmpeg merge in progress |
| `done` | `filename` | Download complete |
| `error` | `message` | Something went wrong |
