# YouTube Downloader

Aplikasi web sederhana untuk mendownload video YouTube. Pilih URL, pilih resolusi, download — selesai.

## Prasyarat

- [Go](https://golang.org/) 1.23+

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

## Catatan Format

| Tipe | Ekstensi | Keterangan |
|------|----------|------------|
| `video/mp4` | `.mp4` | Video + audio (biasanya ≤720p) |
| `video/webm` | `.webm` | Video saja (1080p+, tanpa audio) |
| `audio/mp4` | `.m4a` | Audio saja |
| `audio/webm` | `.webm` | Audio saja |

> Format 1080p ke atas di YouTube menggunakan adaptive streaming (video dan audio terpisah). Untuk menggabungkannya, diperlukan `ffmpeg` secara manual setelah download.

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

### `POST /api/download`
```json
{ "url": "https://youtube.com/watch?v=...", "itag": 22 }
```
Mendownload video ke server dan mengembalikan nama file yang disimpan.
