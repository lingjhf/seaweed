package tus

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func BenchmarkEscapePath(b *testing.B) {
	paths := []string{
		"/videos/file.mp4",
		"/videos/folder with spaces/file 2026.mp4",
		"/中文/目录/文件.mp4",
		"/deep/a/b/c/d/e/f/g/file.mp4",
	}

	b.ReportAllocs()
	for b.Loop() {
		for _, path := range paths {
			if _, err := escapePath(path); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkAddMetadata(b *testing.B) {
	metadata := map[string]string{
		"filename":    "video.mp4",
		"contentType": "video/mp4",
		"owner":       "sdk",
	}

	b.ReportAllocs()
	for b.Loop() {
		header := http.Header{}
		addMetadata(header, metadata)
		if header.Get("Upload-Metadata") == "" {
			b.Fatal("missing metadata")
		}
	}
}

func BenchmarkUploadChunks(b *testing.B) {
	body := strings.Repeat("a", 128*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Location", "/.tus/.uploads/bench")
			w.WriteHeader(http.StatusCreated)
		case http.MethodPatch:
			_, _ = io.Copy(io.Discard, r.Body)
			offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Upload-Offset", strconv.FormatInt(offset+r.ContentLength, 10))
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client, err := New(Config{
		FilerURLs:  []string{server.URL},
		HTTPClient: server.Client(),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		upload, err := client.Create(context.Background(), "/bench.bin", CreateOptions{Size: int64(len(body))})
		if err != nil {
			b.Fatal(err)
		}
		offset, err := client.patchChunks(context.Background(), upload.Location, strings.NewReader(body), 0, int64(len(body)), 32*1024)
		if err != nil {
			b.Fatal(err)
		}
		if offset != int64(len(body)) {
			b.Fatalf("offset = %d, want %d", offset, len(body))
		}
	}
}

func BenchmarkUploadCreationWithUpload(b *testing.B) {
	body := strings.Repeat("a", 128*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Location", "/.tus/.uploads/bench")
			w.Header().Set("Upload-Offset", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client, err := New(Config{
		FilerURLs:  []string{server.URL},
		HTTPClient: server.Client(),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		upload, err := client.Upload(context.Background(), "/bench.bin", strings.NewReader(body), UploadOptions{Size: int64(len(body))})
		if err != nil {
			b.Fatal(err)
		}
		if upload.Offset != int64(len(body)) {
			b.Fatalf("offset = %d, want %d", upload.Offset, len(body))
		}
	}
}
