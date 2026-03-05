# YouTube Downloader

Aplikasi web sederhana untuk mendownload video YouTube. Pilih URL, pilih resolusi, download — selesai.

## Prasyarat

- [Go](https://golang.org/) 1.23+
- [ffmpeg](https://ffmpeg.org/) — diperlukan untuk format 1080p ke atas

## Cara Pakai

```bash
# Clone & masuk ke direktori
git clone https://github.com/dianhadi/youtube
cd youtube

# Install dependencies
go mod tidy

# Jalankan server
go run main.go
```

Buka browser ke **http://localhost:8080**

## Alur

1. Masukan URL video YouTube
2. Klik **Cek Format** — daftar resolusi yang tersedia akan muncul
3. Pilih format yang diinginkan
4. Klik **Download** — video akan didownload ke folder `output/`

## Format & Kualitas

| Badge | Tipe | Keterangan |
|-------|------|------------|
| **Video** | `video/mp4` | Video + audio dalam satu file (≤720p) |
| **HD** | `video/mp4` / `video/webm` | Video-only 1080p+, otomatis di-merge dengan audio via ffmpeg |
| **Audio** | `audio/mp4` / `audio/webm` | Audio saja (`.m4a` / `.webm`) |

Format **HD** akan otomatis mendownload stream video dan audio secara terpisah, lalu menggabungkannya tanpa re-encode menggunakan ffmpeg.

Jika download gagal karena koneksi terputus, server akan mencoba ulang otomatis hingga 3 kali.

## Struktur

```
.
├── main.go         # Web server & API handler
├── static/
│   └── index.html  # Frontend UI
└── output/         # Hasil download (dibuat otomatis)
```

## API

### `GET /api/formats?url=<youtube-url>`
Mengembalikan info video dan daftar format yang tersedia.

**Response:**
```json
{
  "title": "Judul Video",
  "author": "Nama Channel",
  "formats": [
    {
      "itag": 22,
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

### `POST /api/download`
```json
{ "url": "https://youtube.com/watch?v=...", "itag": 22 }
```
Mendownload video ke server dan mengembalikan nama file yang disimpan di folder `output/`. Untuk format video-only (HD), proses mencakup download video, download audio, dan merge dengan ffmpeg.
