package tus_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	client := newTestClient(t, server)
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

func TestOptionsReturnsServerCapabilities(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodOptions {
			t.Fatalf("method = %s, want OPTIONS", r.Method)
		}
		if r.URL.Path != "/.tus/" {
			t.Fatalf("path = %s, want /.tus/", r.URL.Path)
		}
		w.Header().Set("Tus-Resumable", tus.Version)
		w.Header().Set("Tus-Version", "1.0.0")
		w.Header().Set("Tus-Extension", "creation,termination")
		w.Header().Set("Tus-Max-Size", "1048576")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	options, err := client.Options(context.Background())
	if err != nil {
		t.Fatalf("Options() error = %v", err)
	}
	if options.Version != tus.Version || options.Versions != "1.0.0" || options.Extensions != "creation,termination" || options.MaxSize != 1048576 {
		t.Fatalf("Options() = %+v, want server capabilities", options)
	}
}

func TestCreateWithUploadSendsBodyAndHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.EscapedPath() != "/uploads/folder/a%20b.txt" {
			t.Fatalf("path = %s, want /uploads/folder/a%%20b.txt", r.URL.EscapedPath())
		}
		if r.Header.Get("Content-Type") != "application/custom" {
			t.Fatalf("Content-Type = %q, want application/custom", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Upload-Length") != "5" {
			t.Fatalf("Upload-Length = %q, want 5", r.Header.Get("Upload-Length"))
		}
		if r.Header.Get("Upload-Metadata") != "filename aGVsbG8udHh0" {
			t.Fatalf("Upload-Metadata = %q", r.Header.Get("Upload-Metadata"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "hello" {
			t.Fatalf("body = %q, want hello", body)
		}
		w.Header().Set("Location", "/uploads/.uploads/abc")
		w.Header().Set("Upload-Offset", "5")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client, err := tus.New(tus.Config{
		FilerURLs:   []string{server.URL},
		BasePath:    "uploads",
		HTTPClient:  server.Client(),
		ContentType: "application/custom",
	})
	if err != nil {
		t.Fatalf("tus.New() error = %v", err)
	}
	upload, err := client.CreateWithUpload(context.Background(), "folder/a b.txt", strings.NewReader("hello"), tus.CreateOptions{
		Size: 5,
		Metadata: map[string]string{
			"filename": "hello.txt",
		},
	})
	if err != nil {
		t.Fatalf("CreateWithUpload() error = %v", err)
	}
	if upload.Location != server.URL+"/uploads/.uploads/abc" || upload.Offset != 5 || upload.Size != 5 {
		t.Fatalf("CreateWithUpload() = %+v, want resolved upload", upload)
	}
}

func TestHeadPatchAndTerminate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			if r.Header.Get("Tus-Resumable") != tus.Version {
				t.Fatalf("Tus-Resumable = %q", r.Header.Get("Tus-Resumable"))
			}
			w.Header().Set("Upload-Offset", "2")
			w.Header().Set("Upload-Length", "5")
			w.WriteHeader(http.StatusOK)
		case http.MethodPatch:
			if r.Header.Get("Upload-Offset") != "2" {
				t.Fatalf("Upload-Offset = %q, want 2", r.Header.Get("Upload-Offset"))
			}
			if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
				t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read patch body: %v", err)
			}
			if string(body) != "llo" {
				t.Fatalf("patch body = %q, want llo", body)
			}
			w.Header().Set("Upload-Offset", "5")
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	status, err := client.Head(context.Background(), "/.tus/.uploads/abc")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if status.Offset != 2 || status.Size != 5 {
		t.Fatalf("Head() = %+v, want offset 2 size 5", status)
	}
	status, err = client.Patch(context.Background(), "/.tus/.uploads/abc", 2, strings.NewReader("llo"), 3)
	if err != nil {
		t.Fatalf("Patch() error = %v", err)
	}
	if status.Offset != 5 {
		t.Fatalf("Patch().Offset = %d, want 5", status.Offset)
	}
	if err := client.Terminate(context.Background(), "/.tus/.uploads/abc"); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
}

func TestUploadChunksBody(t *testing.T) {
	t.Parallel()

	var patches []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Location", "/.tus/.uploads/chunked")
			w.WriteHeader(http.StatusCreated)
		case http.MethodPatch:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read patch body: %v", err)
			}
			patches = append(patches, r.Header.Get("Upload-Offset")+":"+string(body))
			switch r.Header.Get("Upload-Offset") {
			case "0":
				w.Header().Set("Upload-Offset", "3")
			case "3":
				w.Header().Set("Upload-Offset", "5")
			default:
				t.Fatalf("unexpected Upload-Offset %q", r.Header.Get("Upload-Offset"))
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	upload, err := client.Upload(context.Background(), "/chunked.txt", strings.NewReader("hello"), tus.UploadOptions{
		Size:      5,
		ChunkSize: 3,
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if upload.Offset != 5 || upload.Size != 5 {
		t.Fatalf("Upload() = %+v, want offset 5 size 5", upload)
	}
	want := []string{"0:hel", "3:lo"}
	if len(patches) != len(want) {
		t.Fatalf("patch count = %d, want %d", len(patches), len(want))
	}
	for i := range want {
		if patches[i] != want[i] {
			t.Fatalf("patches[%d] = %q, want %q", i, patches[i], want[i])
		}
	}
}

func TestUploadUsesCreationWithUploadWhenUnchunked(t *testing.T) {
	t.Parallel()

	patchCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
				t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
			}
			if r.Header.Get("Upload-Length") != "5" {
				t.Fatalf("Upload-Length = %q, want 5", r.Header.Get("Upload-Length"))
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if string(body) != "hello" {
				t.Fatalf("body = %q, want hello", body)
			}
			w.Header().Set("Location", "/.tus/.uploads/one-shot")
			w.Header().Set("Upload-Offset", "5")
			w.WriteHeader(http.StatusCreated)
		case http.MethodPatch:
			patchCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	upload, err := client.Upload(context.Background(), "/one-shot.txt", strings.NewReader("hello"), tus.UploadOptions{Size: 5})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if upload.Offset != 5 || upload.Size != 5 {
		t.Fatalf("Upload() = %+v, want offset 5 size 5", upload)
	}
	if patchCalled {
		t.Fatal("Upload() issued PATCH for unchunked upload")
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

	client := newTestClient(t, server)
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

func TestValidationAndResponseErrors(t *testing.T) {
	t.Parallel()

	if _, err := tus.New(tus.Config{}); err == nil {
		t.Fatal("tus.New() error = nil, want missing filer urls error")
	}
	client, err := tus.New(tus.Config{
		FilerURLs:  []string{"http://example.test"},
		HTTPClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatalf("tus.New() error = %v", err)
	}
	if _, err := client.Upload(context.Background(), "/file", strings.NewReader(""), tus.UploadOptions{Size: -1}); err == nil {
		t.Fatal("Upload() error = nil, want size error")
	}
	if _, err := client.Head(context.Background(), ""); err == nil {
		t.Fatal("Head() error = nil, want location error")
	}
	if err := client.Terminate(context.Background(), "relative"); err == nil {
		t.Fatal("Terminate() error = nil, want relative location error")
	}
}

func TestInvalidHeadersAndStatuses(t *testing.T) {
	t.Parallel()

	t.Run("options invalid max size", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Tus-Max-Size", "NaN")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.Options(context.Background()); err == nil {
			t.Fatal("Options() error = nil, want invalid max size error")
		}
	})

	t.Run("create missing location", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.Create(context.Background(), "/file", tus.CreateOptions{Size: 1}); err == nil {
			t.Fatal("Create() error = nil, want missing location error")
		}
	})

	t.Run("create invalid optional offset", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/.tus/.uploads/abc")
			w.Header().Set("Upload-Offset", "NaN")
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.Create(context.Background(), "/file", tus.CreateOptions{Size: 1}); err == nil {
			t.Fatal("Create() error = nil, want invalid offset error")
		}
	})

	t.Run("create with upload non-created status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.CreateWithUpload(context.Background(), "/file", strings.NewReader("x"), tus.CreateOptions{Size: 1}); err == nil {
			t.Fatal("CreateWithUpload() error = nil, want status error")
		}
	})

	t.Run("head missing upload length", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Upload-Offset", "1")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.Head(context.Background(), "/.tus/.uploads/abc"); err == nil {
			t.Fatal("Head() error = nil, want missing length error")
		}
	})
}

func newTestClient(t *testing.T, server *httptest.Server) *tus.Client {
	t.Helper()
	client, err := tus.New(tus.Config{
		FilerURLs:  []string{server.URL},
		BasePath:   "/.tus",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("tus.New() error = %v", err)
	}
	return client
}
