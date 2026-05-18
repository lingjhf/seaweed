package tus_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lingjhf/seaweed/internal/httpx"
	"github.com/lingjhf/seaweed/tus"
)

func TestCreateEncodesHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/.tus/videos/file.mp4" {
			t.Fatalf("path = %s, want /.tus/videos/file.mp4", r.URL.Path)
		}
		if r.Header.Get("Tus-Resumable") != tus.Version {
			t.Fatalf("Tus-Resumable = %q", r.Header.Get("Tus-Resumable"))
		}
		if r.Header.Get("Upload-Length") != "10" {
			t.Fatalf("Upload-Length = %q", r.Header.Get("Upload-Length"))
		}
		if r.Header.Get("Upload-Metadata") != "filename ZmlsZS5tcDQ=" {
			t.Fatalf("Upload-Metadata = %q", r.Header.Get("Upload-Metadata"))
		}
		w.Header().Set("Location", "/.tus/.uploads/abc")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := tus.New(tus.Config{
		FilerURL: server.URL,
		BasePath: "/.tus",
		HTTP:     httpx.NewClient(httpx.Config{HTTPClient: server.Client()}),
	})
	upload, err := client.Create(context.Background(), "/videos/file.mp4", tus.CreateOptions{
		Size: 10,
		Metadata: map[string]string{
			"filename": "file.mp4",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if upload.Location != server.URL+"/.tus/.uploads/abc" {
		t.Fatalf("Location = %q", upload.Location)
	}
}

func TestResumeSeeksToOffset(t *testing.T) {
	t.Parallel()

	var patchBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Upload-Offset", "5")
			w.Header().Set("Upload-Length", "10")
			w.WriteHeader(http.StatusOK)
		case http.MethodPatch:
			if r.Header.Get("Upload-Offset") != "5" {
				t.Fatalf("Upload-Offset = %q, want 5", r.Header.Get("Upload-Offset"))
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			patchBody = string(body)
			w.Header().Set("Upload-Offset", "10")
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	client := tus.New(tus.Config{
		FilerURL: server.URL,
		BasePath: "/.tus",
		HTTP:     httpx.NewClient(httpx.Config{HTTPClient: server.Client()}),
	})
	status, err := client.Resume(context.Background(), server.URL+"/.tus/.uploads/abc", strings.NewReader("helloworld"), tus.ResumeOptions{})
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if status.Offset != 10 {
		t.Fatalf("Offset = %d, want 10", status.Offset)
	}
	if patchBody != "world" {
		t.Fatalf("patch body = %q, want world", patchBody)
	}
}
