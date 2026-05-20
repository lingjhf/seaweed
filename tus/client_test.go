package tus_test

import (
	"context"
	"errors"
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
		if r.Header.Get("Tus-Resumable") != tus.Version {
			t.Fatalf("Tus-Resumable = %q", r.Header.Get("Tus-Resumable"))
		}
		w.Header().Set("Tus-Resumable", tus.Version)
		w.Header().Set("Tus-Version", "1.0.0")
		w.Header().Set("Tus-Extension", "creation,termination")
		w.Header().Set("Tus-Max-Size", "1048576")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	options, err := client.Options(context.Background(), tus.OptionsOptions{})
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

func TestRequestsUsePerRequestAuthorization(t *testing.T) {
	t.Parallel()

	const requestAuth = "Bearer request-token"

	tests := []struct {
		name string
		call func(context.Context, *tus.Client) error
	}{
		{
			name: "options",
			call: func(ctx context.Context, client *tus.Client) error {
				_, err := client.Options(ctx, tus.OptionsOptions{Authorization: requestAuth})
				return err
			},
		},
		{
			name: "create",
			call: func(ctx context.Context, client *tus.Client) error {
				_, err := client.Create(ctx, "/secure/create.txt", tus.CreateOptions{
					Size:          1,
					Authorization: requestAuth,
				})
				return err
			},
		},
		{
			name: "create with upload",
			call: func(ctx context.Context, client *tus.Client) error {
				_, err := client.CreateWithUpload(ctx, "/secure/create-with-upload.txt", strings.NewReader("x"), tus.CreateOptions{
					Size:          1,
					Authorization: requestAuth,
				})
				return err
			},
		},
		{
			name: "head",
			call: func(ctx context.Context, client *tus.Client) error {
				_, err := client.Head(ctx, "/.tus/.uploads/abc", tus.HeadOptions{Authorization: requestAuth})
				return err
			},
		},
		{
			name: "patch",
			call: func(ctx context.Context, client *tus.Client) error {
				_, err := client.Patch(ctx, "/.tus/.uploads/abc", 2, strings.NewReader("llo"), 3, tus.PatchOptions{Authorization: requestAuth})
				return err
			},
		},
		{
			name: "terminate",
			call: func(ctx context.Context, client *tus.Client) error {
				return client.Terminate(ctx, "/.tus/.uploads/abc", tus.TerminateOptions{Authorization: requestAuth})
			},
		},
		{
			name: "upload creation with upload",
			call: func(ctx context.Context, client *tus.Client) error {
				_, err := client.Upload(ctx, "/secure/upload.txt", strings.NewReader("hello"), tus.UploadOptions{
					Size:          5,
					Authorization: requestAuth,
				})
				return err
			},
		},
		{
			name: "upload chunked",
			call: func(ctx context.Context, client *tus.Client) error {
				_, err := client.Upload(ctx, "/secure/chunked.txt", strings.NewReader("hello"), tus.UploadOptions{
					Size:          5,
					ChunkSize:     3,
					Authorization: requestAuth,
				})
				return err
			},
		},
		{
			name: "resume",
			call: func(ctx context.Context, client *tus.Client) error {
				_, err := client.Resume(ctx, "/.tus/.uploads/abc", strings.NewReader("hello"), tus.ResumeOptions{Authorization: requestAuth})
				return err
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != requestAuth {
					t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), requestAuth)
				}
				if r.Header.Get("Tus-Resumable") != tus.Version {
					t.Fatalf("Tus-Resumable = %q, want %q", r.Header.Get("Tus-Resumable"), tus.Version)
				}
				_, _ = io.Copy(io.Discard, r.Body)
				switch r.Method {
				case http.MethodOptions:
					w.Header().Set("Tus-Resumable", tus.Version)
					w.Header().Set("Tus-Version", tus.Version)
					w.Header().Set("Tus-Extension", "creation,creation-with-upload,termination")
					w.Header().Set("Tus-Max-Size", "1048576")
					w.WriteHeader(http.StatusOK)
				case http.MethodPost:
					w.Header().Set("Location", "/.tus/.uploads/abc")
					w.Header().Set("Upload-Offset", "5")
					w.WriteHeader(http.StatusCreated)
				case http.MethodHead:
					w.Header().Set("Upload-Offset", "2")
					w.Header().Set("Upload-Length", "5")
					w.WriteHeader(http.StatusOK)
				case http.MethodPatch:
					w.Header().Set("Upload-Offset", "5")
					w.WriteHeader(http.StatusNoContent)
				case http.MethodDelete:
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Fatalf("unexpected method %s", r.Method)
				}
			}))
			defer server.Close()

			client, err := tus.New(tus.Config{
				FilerURLs:   []string{server.URL},
				BasePath:    "/.tus",
				HTTPClient:  server.Client(),
				BearerToken: "global-token",
			})
			if err != nil {
				t.Fatalf("tus.New() error = %v", err)
			}
			defer client.Close()

			if err := tt.call(context.Background(), client); err != nil {
				t.Fatalf("%s call error = %v", tt.name, err)
			}
		})
	}
}

func TestCreateResolvesAbsoluteLocation(t *testing.T) {
	t.Parallel()

	var deletePath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Location", "http://"+r.Host+"/.tus/.uploads/absolute")
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			deletePath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	upload, err := client.Create(context.Background(), "/file", tus.CreateOptions{Size: 1})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if upload.Location != server.URL+"/.tus/.uploads/absolute" {
		t.Fatalf("Location = %q, want absolute server location", upload.Location)
	}
	if err := client.Terminate(context.Background(), upload.Location, tus.TerminateOptions{}); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	if deletePath != "/.tus/.uploads/absolute" {
		t.Fatalf("DELETE path = %q, want /.tus/.uploads/absolute", deletePath)
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
	status, err := client.Head(context.Background(), "/.tus/.uploads/abc", tus.HeadOptions{})
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if status.Offset != 2 || status.Size != 5 {
		t.Fatalf("Head() = %+v, want offset 2 size 5", status)
	}
	status, err = client.Patch(context.Background(), "/.tus/.uploads/abc", 2, strings.NewReader("llo"), 3, tus.PatchOptions{})
	if err != nil {
		t.Fatalf("Patch() error = %v", err)
	}
	if status.Offset != 5 {
		t.Fatalf("Patch().Offset = %d, want 5", status.Offset)
	}
	if err := client.Terminate(context.Background(), "/.tus/.uploads/abc", tus.TerminateOptions{}); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
}

func TestTerminateReturnsStatusAPIError(t *testing.T) {
	t.Parallel()

	client, err := tus.New(tus.Config{
		FilerURLs: []string{"http://filer.test"},
		BasePath:  "/.tus",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodDelete {
					t.Fatalf("method = %s, want DELETE", req.Method)
				}
				if req.URL.String() != "http://filer.test/.tus/.uploads/abc" {
					t.Fatalf("url = %q, want http://filer.test/.tus/.uploads/abc", req.URL.String())
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Status:     "204 No Content",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"error":"terminate failed"}`)),
					Request:    req,
				}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("tus.New() error = %v", err)
	}
	defer client.Close()

	err = client.Terminate(context.Background(), "/.tus/.uploads/abc", tus.TerminateOptions{})
	if err == nil {
		t.Fatal("Terminate() error = nil, want API error")
	}
	assertAPIError(t, err, "terminate failed")
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

func TestResumeReturnsSeekError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Upload-Offset", "5")
			w.Header().Set("Upload-Length", "10")
			w.WriteHeader(http.StatusOK)
		case http.MethodPatch:
			t.Fatal("PATCH should not be called when seek fails")
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.Resume(context.Background(), "/.tus/.uploads/abc", failingReadSeeker{}, tus.ResumeOptions{})
	if err == nil {
		t.Fatal("Resume() error = nil, want seek error")
	}
	if !strings.Contains(err.Error(), "seek to offset 5") {
		t.Fatalf("Resume() error = %q, want offset context", err.Error())
	}
}

func TestResumeAlreadyCompleteSkipsPatch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Upload-Offset", "5")
			w.Header().Set("Upload-Length", "5")
			w.WriteHeader(http.StatusOK)
		case http.MethodPatch:
			t.Fatal("PATCH should not be called for a completed upload")
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	status, err := client.Resume(context.Background(), "/.tus/.uploads/abc", strings.NewReader("hello"), tus.ResumeOptions{})
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if status.Offset != 5 || status.Size != 5 {
		t.Fatalf("Resume() = %+v, want complete status", status)
	}
}

func TestUploadReturnsCreateError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if _, err := client.Upload(context.Background(), "/file", strings.NewReader("hello"), tus.UploadOptions{
		Size:      5,
		ChunkSize: 3,
	}); err == nil {
		t.Fatal("Upload() error = nil, want create error")
	}
}

func TestPatchReturnsStatusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "conflict", http.StatusConflict)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if _, err := client.Patch(context.Background(), "/.tus/.uploads/abc", 0, strings.NewReader("hello"), 5, tus.PatchOptions{}); err == nil {
		t.Fatal("Patch() error = nil, want status error")
	}
}

func TestValidationAndResponseErrors(t *testing.T) {
	t.Parallel()

	if _, err := tus.New(tus.Config{}); err == nil {
		t.Fatal("tus.New() error = nil, want missing filer urls error")
	}
	if _, err := tus.New(tus.Config{FilerURLs: []string{"relative"}}); err == nil {
		t.Fatal("tus.New() error = nil, want invalid filer url error")
	}
	if _, err := tus.New(tus.Config{
		FilerURLs: []string{"http://example.test"},
		EndpointPolicy: tus.EndpointPolicy{
			Mode: "random",
		},
	}); err == nil {
		t.Fatal("tus.New() error = nil, want invalid endpoint policy error")
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
	if _, err := client.Head(context.Background(), "", tus.HeadOptions{}); err == nil {
		t.Fatal("Head() error = nil, want location error")
	}
	if err := client.Terminate(context.Background(), "relative", tus.TerminateOptions{}); err == nil {
		t.Fatal("Terminate() error = nil, want relative location error")
	}
}

func TestInvalidHeadersAndStatuses(t *testing.T) {
	t.Parallel()

	t.Run("options non-ok status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not allowed", http.StatusMethodNotAllowed)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.Options(context.Background(), tus.OptionsOptions{}); err == nil {
			t.Fatal("Options() error = nil, want status error")
		}
	})

	t.Run("options invalid max size", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Tus-Max-Size", "NaN")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.Options(context.Background(), tus.OptionsOptions{}); err == nil {
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

	t.Run("create with upload invalid location", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "relative")
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.CreateWithUpload(context.Background(), "/file", strings.NewReader("x"), tus.CreateOptions{Size: 1}); err == nil {
			t.Fatal("CreateWithUpload() error = nil, want invalid location error")
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

	t.Run("head missing upload offset", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Upload-Length", "1")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.Head(context.Background(), "/.tus/.uploads/abc", tus.HeadOptions{}); err == nil {
			t.Fatal("Head() error = nil, want missing offset error")
		}
	})

	t.Run("head missing upload length", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Upload-Offset", "1")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.Head(context.Background(), "/.tus/.uploads/abc", tus.HeadOptions{}); err == nil {
			t.Fatal("Head() error = nil, want missing length error")
		}
	})

	t.Run("patch invalid upload offset", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Upload-Offset", "NaN")
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		client := newTestClient(t, server)
		if _, err := client.Patch(context.Background(), "/.tus/.uploads/abc", 0, strings.NewReader("x"), 1, tus.PatchOptions{}); err == nil {
			t.Fatal("Patch() error = nil, want invalid offset error")
		}
	})
}

func TestClientClose(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client := newTestClient(t, server)
	client.Close()
	client.Close()
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

func assertAPIError(t *testing.T, err error, want string) {
	t.Helper()
	var apiErr *httpx.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *httpx.APIError", err)
	}
	if apiErr.Message != want {
		t.Fatalf("APIError.Message = %q, want %q", apiErr.Message, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingReadSeeker struct{}

func (failingReadSeeker) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (failingReadSeeker) Seek(int64, int) (int64, error) {
	return 0, errors.New("seek failed")
}
