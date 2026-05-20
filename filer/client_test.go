package filer_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lingjhf/seaweed/filer"
	"github.com/lingjhf/seaweed/internal/httpx"
)

func TestPutBuildsRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/docs/report.txt" {
			t.Fatalf("path = %s, want /docs/report.txt", r.URL.Path)
		}
		query := r.URL.Query()
		assertQuery(t, query.Get("dataCenter"), "dc1")
		assertQuery(t, query.Get("rack"), "rack1")
		assertQuery(t, query.Get("dataNode"), "node1")
		assertQuery(t, query.Get("collection"), "photos")
		assertQuery(t, query.Get("replication"), "001")
		assertQuery(t, query.Get("ttl"), "3d")
		assertQuery(t, query.Get("maxMB"), "32")
		assertQuery(t, query.Get("mode"), "0755")
		assertQuery(t, query.Get("offset"), "7")
		assertQuery(t, query.Get("fsync"), "true")
		assertQuery(t, query.Get("saveInside"), "true")
		assertQuery(t, query.Get("skipCheckParentDir"), "true")
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Content-Disposition") != `inline; filename="report.txt"` {
			t.Fatalf("Content-Disposition = %q", r.Header.Get("Content-Disposition"))
		}
		if r.Header.Get("Seaweed-Owner") != "sdk" {
			t.Fatalf("Seaweed-Owner = %q", r.Header.Get("Seaweed-Owner"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "hello" {
			t.Fatalf("body = %q, want hello", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "report.txt",
			"size": 5,
		})
	}))
	defer server.Close()

	offset := int64(7)
	client := newTestClient(t, server)
	resp, err := client.Put(context.Background(), "/docs/report.txt", strings.NewReader("hello"), filer.WriteOptions{
		DataCenter:         "dc1",
		Rack:               "rack1",
		DataNode:           "node1",
		Collection:         "photos",
		Replication:        "001",
		TTL:                "3d",
		MaxMB:              32,
		Mode:               "0755",
		Offset:             &offset,
		Fsync:              true,
		SaveInside:         true,
		SkipCheckParentDir: true,
		ContentType:        "text/plain",
		ContentDisposition: `inline; filename="report.txt"`,
		ContentLength:      5,
		SeaweedHeaders: map[string]string{
			"Seaweed-Owner": "sdk",
		},
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if resp.Size != 5 {
		t.Fatalf("Size = %d, want 5", resp.Size)
	}
}

func TestAppendBuildsRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/docs/report.txt" {
			t.Fatalf("path = %s, want /docs/report.txt", r.URL.Path)
		}
		if r.URL.Query().Get("op") != "append" {
			t.Fatalf("op = %q, want append", r.URL.Query().Get("op"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "-tail" {
			t.Fatalf("body = %q, want -tail", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "report.txt",
			"size": 10,
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	resp, err := client.Append(context.Background(), "/docs/report.txt", strings.NewReader("-tail"), filer.AppendOptions{ContentLength: 5})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if resp.Size != 10 {
		t.Fatalf("Size = %d, want 10", resp.Size)
	}
}

func TestUploadMultipartBuildsStreamingRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/uploads/" {
			t.Fatalf("path = %s, want /uploads/", r.URL.Path)
		}
		query := r.URL.Query()
		assertQuery(t, query.Get("collection"), "photos")
		assertQuery(t, query.Get("replication"), "001")
		assertQuery(t, query.Get("ttl"), "3d")
		assertQuery(t, query.Get("maxMB"), "32")
		assertQuery(t, query.Get("mode"), "0755")
		assertQuery(t, query.Get("fsync"), "true")
		assertQuery(t, query.Get("saveInside"), "true")
		assertQuery(t, query.Get("skipCheckParentDir"), "true")
		if r.Header.Get("Seaweed-Owner") != "sdk" {
			t.Fatalf("Seaweed-Owner = %q, want sdk", r.Header.Get("Seaweed-Owner"))
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data; boundary=") {
			t.Fatalf("Content-Type = %q, want multipart form", r.Header.Get("Content-Type"))
		}

		multipartReader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("MultipartReader() error = %v", err)
		}
		part, err := multipartReader.NextPart()
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}
		if part.FormName() != "asset" {
			t.Fatalf("FormName = %q, want asset", part.FormName())
		}
		if part.FileName() != "report.txt" {
			t.Fatalf("FileName = %q, want report.txt", part.FileName())
		}
		if part.Header.Get("Content-Type") != "text/plain" {
			t.Fatalf("part Content-Type = %q, want text/plain", part.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		if string(body) != "hello multipart" {
			t.Fatalf("part body = %q, want hello multipart", body)
		}
		if next, err := multipartReader.NextPart(); err != io.EOF || next != nil {
			t.Fatalf("NextPart() = %v, %v; want EOF", next, err)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "report.txt",
			"size": len("hello multipart"),
			"eTag": "etag",
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	resp, err := client.UploadMultipart(context.Background(), "/uploads/", strings.NewReader("hello multipart"), filer.MultipartUploadOptions{
		Collection:         "photos",
		Replication:        "001",
		TTL:                "3d",
		MaxMB:              32,
		Mode:               "0755",
		Fsync:              true,
		SaveInside:         true,
		SkipCheckParentDir: true,
		Filename:           "report.txt",
		FileContentType:    "text/plain",
		FieldName:          "asset",
		SeaweedHeaders: map[string]string{
			"Owner": "sdk",
		},
	})
	if err != nil {
		t.Fatalf("UploadMultipart() error = %v", err)
	}
	if resp.Name != "report.txt" || resp.Size != int64(len("hello multipart")) || resp.ETag != "etag" {
		t.Fatalf("UploadMultipart() = %+v, want decoded write result", resp)
	}
}

func TestUploadMultipartFilePathUsesBaseName(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uploads/report.txt" {
			t.Fatalf("path = %s, want /uploads/report.txt", r.URL.Path)
		}
		multipartReader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("MultipartReader() error = %v", err)
		}
		part, err := multipartReader.NextPart()
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}
		if part.FileName() != "report.txt" {
			t.Fatalf("FileName = %q, want report.txt", part.FileName())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "report.txt",
			"size": len("hello multipart"),
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if _, err := client.UploadMultipart(context.Background(), "/uploads/report.txt", strings.NewReader("hello multipart"), filer.MultipartUploadOptions{}); err != nil {
		t.Fatalf("UploadMultipart() error = %v", err)
	}
}

func TestUploadMultipartDefaultsFieldNameAndContentType(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		multipartReader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("MultipartReader() error = %v", err)
		}
		part, err := multipartReader.NextPart()
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}
		if part.FormName() != "file" {
			t.Fatalf("FormName = %q, want file", part.FormName())
		}
		if part.Header.Get("Content-Type") != "application/octet-stream" {
			t.Fatalf("part Content-Type = %q, want application/octet-stream", part.Header.Get("Content-Type"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "default.bin",
			"size": 4,
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if _, err := client.UploadMultipart(context.Background(), "/uploads/default.bin", strings.NewReader("data"), filer.MultipartUploadOptions{}); err != nil {
		t.Fatalf("UploadMultipart() error = %v", err)
	}
}

func TestRequestsUsePerRequestAuthorization(t *testing.T) {
	t.Parallel()

	const requestAuth = "Bearer request-token"

	tests := []struct {
		name       string
		wantMethod string
		wantPath   string
		call       func(context.Context, *filer.Client) error
		respond    func(http.ResponseWriter)
	}{
		{
			name:       "put",
			wantMethod: http.MethodPut,
			wantPath:   "/docs/report.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				_, err := client.Put(ctx, "/docs/report.txt", strings.NewReader("x"), filer.WriteOptions{
					ContentLength: 1,
					Authorization: requestAuth,
				})
				return err
			},
			respond: writeResultResponse("report.txt", 1),
		},
		{
			name:       "append",
			wantMethod: http.MethodPut,
			wantPath:   "/docs/report.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				_, err := client.Append(ctx, "/docs/report.txt", strings.NewReader("x"), filer.AppendOptions{
					ContentLength: 1,
					Authorization: requestAuth,
				})
				return err
			},
			respond: writeResultResponse("report.txt", 1),
		},
		{
			name:       "upload multipart",
			wantMethod: http.MethodPost,
			wantPath:   "/uploads/report.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				_, err := client.UploadMultipart(ctx, "/uploads/report.txt", strings.NewReader("x"), filer.MultipartUploadOptions{
					Authorization: requestAuth,
				})
				return err
			},
			respond: writeResultResponse("report.txt", 1),
		},
		{
			name:       "get",
			wantMethod: http.MethodGet,
			wantPath:   "/docs/report.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				resp, err := client.Get(ctx, "/docs/report.txt", filer.GetOptions{Authorization: requestAuth})
				if resp != nil {
					_ = resp.Body.Close()
				}
				return err
			},
			respond: func(w http.ResponseWriter) {
				_, _ = w.Write([]byte("x"))
			},
		},
		{
			name:       "head",
			wantMethod: http.MethodHead,
			wantPath:   "/docs/report.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				_, err := client.Head(ctx, "/docs/report.txt", filer.HeadOptions{Authorization: requestAuth})
				return err
			},
		},
		{
			name:       "tags",
			wantMethod: http.MethodHead,
			wantPath:   "/docs/report.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				_, err := client.Tags(ctx, "/docs/report.txt", filer.HeadOptions{Authorization: requestAuth})
				return err
			},
		},
		{
			name:       "stat",
			wantMethod: http.MethodGet,
			wantPath:   "/docs/report.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				_, err := client.Stat(ctx, "/docs/report.txt", filer.StatOptions{Authorization: requestAuth})
				return err
			},
			respond: func(w http.ResponseWriter) {
				if err := json.NewEncoder(w).Encode(map[string]string{"FullPath": "/docs/report.txt"}); err != nil {
					t.Fatalf("encode stat response: %v", err)
				}
			},
		},
		{
			name:       "list page",
			wantMethod: http.MethodGet,
			wantPath:   "/docs/",
			call: func(ctx context.Context, client *filer.Client) error {
				_, err := client.ListPage(ctx, "/docs", filer.ListOptions{Authorization: requestAuth})
				return err
			},
			respond: listPageResponse,
		},
		{
			name:       "walk",
			wantMethod: http.MethodGet,
			wantPath:   "/docs/",
			call: func(ctx context.Context, client *filer.Client) error {
				return client.Walk(ctx, "/docs", filer.WalkOptions{Authorization: requestAuth}, func(filer.Entry) error {
					return nil
				})
			},
			respond: listPageResponse,
		},
		{
			name:       "delete",
			wantMethod: http.MethodDelete,
			wantPath:   "/docs/report.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				return client.Delete(ctx, "/docs/report.txt", filer.DeleteOptions{Authorization: requestAuth})
			},
		},
		{
			name:       "mkdir",
			wantMethod: http.MethodPost,
			wantPath:   "/docs/",
			call: func(ctx context.Context, client *filer.Client) error {
				return client.Mkdir(ctx, "/docs", filer.MkdirOptions{Authorization: requestAuth})
			},
		},
		{
			name:       "copy",
			wantMethod: http.MethodPost,
			wantPath:   "/dst.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				return client.Copy(ctx, "/src.txt", "/dst.txt", filer.CopyOptions{Authorization: requestAuth})
			},
		},
		{
			name:       "move",
			wantMethod: http.MethodPost,
			wantPath:   "/moved.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				return client.Move(ctx, "/dst.txt", "/moved.txt", filer.MoveOptions{Authorization: requestAuth})
			},
		},
		{
			name:       "set tags",
			wantMethod: http.MethodPut,
			wantPath:   "/docs/report.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				return client.SetTags(ctx, "/docs/report.txt", map[string]string{"Owner": "sdk"}, filer.TagOptions{Authorization: requestAuth})
			},
		},
		{
			name:       "delete tags",
			wantMethod: http.MethodDelete,
			wantPath:   "/docs/report.txt",
			call: func(ctx context.Context, client *filer.Client) error {
				return client.DeleteTags(ctx, "/docs/report.txt", filer.TagOptions{Authorization: requestAuth}, "Owner")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.wantMethod {
					t.Fatalf("method = %s, want %s", r.Method, tt.wantMethod)
				}
				if r.URL.Path != tt.wantPath {
					t.Fatalf("path = %s, want %s", r.URL.Path, tt.wantPath)
				}
				if r.Header.Get("Authorization") != requestAuth {
					t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), requestAuth)
				}
				_, _ = io.Copy(io.Discard, r.Body)
				if tt.respond != nil {
					tt.respond(w)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			client, err := filer.New(filer.Config{
				BaseURLs:    []string{server.URL},
				HTTPClient:  server.Client(),
				BearerToken: "global-token",
			})
			if err != nil {
				t.Fatalf("filer.New() error = %v", err)
			}

			if err := tt.call(context.Background(), client); err != nil {
				t.Fatalf("%s call error = %v", tt.name, err)
			}
		})
	}
}

func TestWriteRequestsReturnJSONAPIErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(*filer.Client) (*filer.WriteResult, error)
	}{
		{
			name: "put",
			call: func(client *filer.Client) (*filer.WriteResult, error) {
				return client.Put(context.Background(), "/docs/report.txt", strings.NewReader("x"), filer.WriteOptions{ContentLength: 1})
			},
		},
		{
			name: "append",
			call: func(client *filer.Client) (*filer.WriteResult, error) {
				return client.Append(context.Background(), "/docs/report.txt", strings.NewReader("x"), filer.AppendOptions{ContentLength: 1})
			},
		},
		{
			name: "upload multipart",
			call: func(client *filer.Client) (*filer.WriteResult, error) {
				return client.UploadMultipart(context.Background(), "/uploads/report.txt", strings.NewReader("x"), filer.MultipartUploadOptions{})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "write failed",
				})
			}))
			defer server.Close()

			client := newTestClient(t, server)
			_, err := tt.call(client)
			if err == nil {
				t.Fatal("write error = nil, want API error")
			}
			assertAPIError(t, err, "write failed")
		})
	}
}

func TestListPageBuildsJSONRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/docs/" {
			t.Fatalf("path = %s, want /docs/", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("Accept = %q, want application/json", r.Header.Get("Accept"))
		}
		query := r.URL.Query()
		assertQuery(t, query.Get("limit"), "2")
		assertQuery(t, query.Get("lastFileName"), "a.txt")
		assertQuery(t, query.Get("namePattern"), "*.txt")
		assertQuery(t, query.Get("namePatternExclude"), "*.tmp")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Path": "/docs",
			"Entries": []map[string]any{
				{"FullPath": "/docs/report.txt", "FileSize": 5, "Uid": 1001, "Gid": 1002},
			},
			"Limit":        2,
			"LastFileName": "report.txt",
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	resp, err := client.ListPage(context.Background(), "/docs", filer.ListOptions{
		Limit:              2,
		LastFileName:       "a.txt",
		NamePattern:        "*.txt",
		NamePatternExclude: "*.tmp",
	})
	if err != nil {
		t.Fatalf("ListPage() error = %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("Entries len = %d, want 1", len(resp.Entries))
	}
	if resp.Entries[0].UID != 1001 || resp.Entries[0].GID != 1002 {
		t.Fatalf("entry UID/GID = %d/%d, want 1001/1002", resp.Entries[0].UID, resp.Entries[0].GID)
	}
}

func TestWalkPaginatesAndStopsOnCallbackError(t *testing.T) {
	t.Parallel()

	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/docs/" {
			t.Fatalf("path = %s, want /docs/", r.URL.Path)
		}
		query := r.URL.Query()
		assertQuery(t, query.Get("limit"), "1")
		assertQuery(t, query.Get("namePattern"), "*.txt")
		calls = append(calls, query.Get("lastFileName"))
		switch query.Get("lastFileName") {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Entries": []map[string]any{
					{"FullPath": "/docs/a.txt"},
				},
				"LastFileName":          "a.txt",
				"ShouldDisplayLoadMore": true,
			})
		case "a.txt":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Entries": []map[string]any{
					{"FullPath": "/docs/b.txt"},
				},
				"LastFileName":          "b.txt",
				"ShouldDisplayLoadMore": false,
			})
		default:
			t.Fatalf("unexpected lastFileName %q", query.Get("lastFileName"))
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	paths := []string{}
	err := client.Walk(context.Background(), "/docs", filer.WalkOptions{
		Limit:       1,
		NamePattern: "*.txt",
	}, func(entry filer.Entry) error {
		paths = append(paths, entry.FullPath)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if strings.Join(paths, ",") != "/docs/a.txt,/docs/b.txt" {
		t.Fatalf("walk paths = %#v", paths)
	}
	if strings.Join(calls, ",") != ",a.txt" {
		t.Fatalf("lastFileName calls = %#v", calls)
	}

	stop := errors.New("stop")
	err = client.Walk(context.Background(), "/docs", filer.WalkOptions{
		Limit:       1,
		NamePattern: "*.txt",
	}, func(entry filer.Entry) error {
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("Walk() error = %v, want stop", err)
	}
}

func TestWalkValidationAndPaginationErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("lastFileName") {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Entries": []map[string]any{
					{"FullPath": "/docs/a.txt"},
				},
				"ShouldDisplayLoadMore": true,
			})
		case "repeat":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Entries": []map[string]any{
					{"FullPath": "/docs/b.txt"},
				},
				"LastFileName":          "repeat",
				"ShouldDisplayLoadMore": true,
			})
		default:
			t.Fatalf("unexpected lastFileName %q", r.URL.Query().Get("lastFileName"))
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if err := client.Walk(context.Background(), "/docs", filer.WalkOptions{}, nil); err == nil {
		t.Fatal("Walk() error = nil, want callback error")
	}
	err := client.Walk(context.Background(), "/docs", filer.WalkOptions{}, func(entry filer.Entry) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "missing last file name") {
		t.Fatalf("Walk() error = %v, want missing last file name", err)
	}
	err = client.Walk(context.Background(), "/docs", filer.WalkOptions{LastFileName: "repeat"}, func(entry filer.Entry) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "repeated last file name") {
		t.Fatalf("Walk() error = %v, want repeated last file name", err)
	}
}

func TestMkdirGetHeadStatAndDeleteRequests(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/docs/":
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/docs/report.txt" && r.URL.Query().Get("metadata") == "":
			assertQuery(t, r.URL.Query().Get("response-content-disposition"), `attachment; filename="report.txt"`)
			assertQuery(t, r.URL.Query().Get("resolveManifest"), "true")
			_, _ = w.Write([]byte("hello"))
		case r.Method == http.MethodHead && r.URL.Path == "/docs/report.txt":
			w.Header().Set("Seaweed-Owner", "sdk")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/docs/report.txt" && r.URL.Query().Get("metadata") == "true":
			assertQuery(t, r.URL.Query().Get("resolveManifest"), "true")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"FullPath": "/docs/report.txt",
				"Mtime":    "2026-05-18T12:00:00Z",
				"Crtime":   "2026-05-18T11:00:00Z",
				"Mode":     420,
				"Uid":      1001,
				"Gid":      1002,
				"DiskType": "ssd",
				"Md5":      "checksum",
				"FileSize": 5,
				"Rdev":     7,
				"Inode":    9,
				"Quota":    11,
				"chunks": []map[string]any{
					{
						"file_id": "7,abc",
						"size":    5,
						"e_tag":   "tag",
						"fid": map[string]any{
							"volume_id": 7,
							"file_key":  123,
							"cookie":    456,
						},
					},
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/docs/report.txt":
			query := r.URL.Query()
			assertQuery(t, query.Get("recursive"), "true")
			assertQuery(t, query.Get("ignoreRecursiveError"), "true")
			assertQuery(t, query.Get("skipChunkDeletion"), "true")
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if err := client.Mkdir(context.Background(), "/docs", filer.MkdirOptions{}); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	resp, err := client.Get(context.Background(), "/docs/report.txt", filer.GetOptions{
		ResponseContentDisposition: `attachment; filename="report.txt"`,
		ResolveManifest:            true,
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}
	head, err := client.Head(context.Background(), "/docs/report.txt", filer.HeadOptions{})
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if head.Header.Get("Seaweed-Owner") != "sdk" {
		t.Fatalf("Seaweed-Owner = %q, want sdk", head.Header.Get("Seaweed-Owner"))
	}
	if head.Tags["Owner"] != "sdk" {
		t.Fatalf("Tags[Owner] = %q, want sdk", head.Tags["Owner"])
	}
	tags, err := client.Tags(context.Background(), "/docs/report.txt", filer.HeadOptions{})
	if err != nil {
		t.Fatalf("Tags() error = %v", err)
	}
	if tags["Owner"] != "sdk" {
		t.Fatalf("Tags()[Owner] = %q, want sdk", tags["Owner"])
	}
	entry, err := client.Stat(context.Background(), "/docs/report.txt", filer.StatOptions{ResolveManifest: true})
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if entry.FullPath != "/docs/report.txt" || entry.FileSize != 5 || len(entry.Chunks) != 1 {
		t.Fatalf("Stat() = %+v, want decoded entry", entry)
	}
	wantMtime := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	if !entry.Mtime.Equal(wantMtime) || entry.DiskType != "ssd" || entry.MD5 != "checksum" {
		t.Fatalf("Stat() metadata = %+v, want typed metadata", entry)
	}
	if entry.Rdev != 7 || entry.Inode != 9 || entry.Quota != 11 {
		t.Fatalf("Stat() numeric metadata = %+v", entry)
	}
	if entry.UID != 1001 || entry.GID != 1002 {
		t.Fatalf("Stat() UID/GID = %d/%d, want 1001/1002", entry.UID, entry.GID)
	}
	if entry.Chunks[0].FID.VolumeID != 7 || entry.Chunks[0].FID.FileKey != 123 || entry.Chunks[0].FID.Cookie != 456 {
		t.Fatalf("Stat() chunk fid = %+v", entry.Chunks[0].FID)
	}
	if err := client.Delete(context.Background(), "/docs/report.txt", filer.DeleteOptions{
		Recursive:            true,
		IgnoreRecursiveError: true,
		SkipChunkDeletion:    true,
	}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestCopyMoveAndTaggingRequests(t *testing.T) {
	t.Parallel()

	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.String())
		switch {
		case r.URL.Query().Get("cp.from") == "/src.txt":
			if r.URL.Path != "/dst.txt" {
				t.Fatalf("copy path = %s, want /dst.txt", r.URL.Path)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Query().Get("mv.from") == "/dst.txt":
			if r.URL.Path != "/moved.txt" {
				t.Fatalf("move path = %s, want /moved.txt", r.URL.Path)
			}
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.RawQuery, "tagging"):
			if r.Method == http.MethodPut && r.Header.Get("Seaweed-Owner") != "sdk" {
				t.Fatalf("Seaweed-Owner = %q", r.Header.Get("Seaweed-Owner"))
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if err := client.Copy(context.Background(), "/src.txt", "/dst.txt", filer.CopyOptions{}); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	if err := client.Move(context.Background(), "/dst.txt", "/moved.txt", filer.MoveOptions{}); err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if err := client.SetTags(context.Background(), "/moved.txt", map[string]string{"Owner": "sdk"}, filer.TagOptions{}); err != nil {
		t.Fatalf("SetTags() error = %v", err)
	}
	if err := client.DeleteTags(context.Background(), "/moved.txt", filer.TagOptions{}, "Owner"); err != nil {
		t.Fatalf("DeleteTags() error = %v", err)
	}
	if len(requests) != 4 {
		t.Fatalf("request count = %d, want 4", len(requests))
	}
}

func TestStatusOnlyMethodsReturnAPIErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		method string
		call   func(*filer.Client) error
	}{
		{
			name:   "mkdir",
			path:   "/docs/",
			method: http.MethodPost,
			call: func(client *filer.Client) error {
				return client.Mkdir(context.Background(), "/docs", filer.MkdirOptions{})
			},
		},
		{
			name:   "copy",
			path:   "/dst.txt",
			method: http.MethodPost,
			call: func(client *filer.Client) error {
				return client.Copy(context.Background(), "/src.txt", "/dst.txt", filer.CopyOptions{})
			},
		},
		{
			name:   "move",
			path:   "/moved.txt",
			method: http.MethodPost,
			call: func(client *filer.Client) error {
				return client.Move(context.Background(), "/dst.txt", "/moved.txt", filer.MoveOptions{})
			},
		},
		{
			name:   "set tags",
			path:   "/moved.txt",
			method: http.MethodPut,
			call: func(client *filer.Client) error {
				return client.SetTags(context.Background(), "/moved.txt", map[string]string{"Owner": "sdk"}, filer.TagOptions{})
			},
		},
		{
			name:   "delete tags",
			path:   "/moved.txt",
			method: http.MethodDelete,
			call: func(client *filer.Client) error {
				return client.DeleteTags(context.Background(), "/moved.txt", filer.TagOptions{}, "Owner")
			},
		},
		{
			name:   "delete",
			path:   "/docs/report.txt",
			method: http.MethodDelete,
			call: func(client *filer.Client) error {
				return client.Delete(context.Background(), "/docs/report.txt", filer.DeleteOptions{})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method {
					t.Fatalf("method = %s, want %s", r.Method, tt.method)
				}
				if r.URL.Path != tt.path {
					t.Fatalf("path = %q, want %q", r.URL.Path, tt.path)
				}
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": tt.name + " failed",
				})
			}))
			defer server.Close()

			client := newTestClient(t, server)
			err := tt.call(client)
			if err == nil {
				t.Fatal("status method error = nil, want API error")
			}
			assertAPIError(t, err, tt.name+" failed")
		})
	}
}

func TestValidationAndHTTPErrorResponses(t *testing.T) {
	t.Parallel()

	if _, err := filer.New(filer.Config{}); err == nil {
		t.Fatal("filer.New() error = nil, want base urls error")
	}
	if _, err := filer.New(filer.Config{BaseURLs: []string{"relative"}}); err == nil {
		t.Fatal("filer.New() error = nil, want invalid base url error")
	}
	if _, err := filer.New(filer.Config{
		BaseURLs: []string{"http://example.test"},
		EndpointPolicy: filer.EndpointPolicy{
			Mode: "random",
		},
	}); err == nil {
		t.Fatal("filer.New() error = nil, want invalid endpoint policy error")
	}

	clientWithBaseURL, err := filer.New(filer.Config{
		BaseURLs:   []string{"http://example.test"},
		HTTPClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatalf("filer.New() error = %v", err)
	}
	resp, err := clientWithBaseURL.Get(context.Background(), "", filer.GetOptions{})
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("Get() error = nil, want path error")
	}
	if _, err := clientWithBaseURL.ListPage(context.Background(), "", filer.ListOptions{}); err == nil {
		t.Fatal("List() error = nil, want path error")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	resp, err = client.Get(context.Background(), "/missing.txt", filer.GetOptions{})
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("Get() error = nil, want status error")
	}
	assertNotFound(t, err)
	header, err := client.Head(context.Background(), "/missing.txt", filer.HeadOptions{})
	if err == nil {
		t.Fatalf("Head() = %v, nil, want status error", header)
	}
	assertNotFound(t, err)
	if _, err := client.Tags(context.Background(), "/missing.txt", filer.HeadOptions{}); err == nil {
		t.Fatal("Tags() error = nil, want status error")
	} else {
		assertNotFound(t, err)
	}
	if err := client.Delete(context.Background(), "/missing.txt", filer.DeleteOptions{}); err == nil {
		t.Fatal("Delete() error = nil, want status error")
	} else {
		assertNotFound(t, err)
	}
}

func TestPathValidationForMutatingMethods(t *testing.T) {
	t.Parallel()

	client, err := filer.New(filer.Config{
		BaseURLs:   []string{"http://example.test"},
		HTTPClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatalf("filer.New() error = %v", err)
	}

	if _, err := client.Put(context.Background(), "", strings.NewReader("x"), filer.WriteOptions{}); err == nil {
		t.Fatal("Put() error = nil, want path error")
	}
	if _, err := client.Append(context.Background(), "", strings.NewReader("x"), filer.AppendOptions{}); err == nil {
		t.Fatal("Append() error = nil, want path error")
	}
	if _, err := client.UploadMultipart(context.Background(), "", strings.NewReader("x"), filer.MultipartUploadOptions{Filename: "file.txt"}); err == nil {
		t.Fatal("UploadMultipart() error = nil, want path error")
	}
	if _, err := client.UploadMultipart(context.Background(), "/uploads/", strings.NewReader("x"), filer.MultipartUploadOptions{}); err == nil {
		t.Fatal("UploadMultipart() error = nil, want filename error")
	}
	if _, err := client.UploadMultipart(context.Background(), "/uploads/file.txt", nil, filer.MultipartUploadOptions{}); err == nil {
		t.Fatal("UploadMultipart() error = nil, want body error")
	}
	if err := client.Copy(context.Background(), "/src", "", filer.CopyOptions{}); err == nil {
		t.Fatal("Copy() error = nil, want destination path error")
	}
	if err := client.Move(context.Background(), "/src", "", filer.MoveOptions{}); err == nil {
		t.Fatal("Move() error = nil, want destination path error")
	}
	if err := client.SetTags(context.Background(), "", map[string]string{"Owner": "sdk"}, filer.TagOptions{}); err == nil {
		t.Fatal("SetTags() error = nil, want path error")
	}
	if err := client.DeleteTags(context.Background(), "", filer.TagOptions{}, "Owner"); err == nil {
		t.Fatal("DeleteTags() error = nil, want path error")
	}
	if _, err := client.Stat(context.Background(), "", filer.StatOptions{}); err == nil {
		t.Fatal("Stat() error = nil, want path error")
	}
	if err := client.Delete(context.Background(), "", filer.DeleteOptions{}); err == nil {
		t.Fatal("Delete() error = nil, want path error")
	}
}

func TestClientClose(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client := newTestClient(t, server)
	client.Close()
	client.Close()
}

func writeResultResponse(name string, size int64) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": name,
			"size": size,
		})
	}
}

func listPageResponse(w http.ResponseWriter) {
	if err := json.NewEncoder(w).Encode(map[string][]map[string]string{
		"Entries": {},
	}); err != nil {
		panic(err)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *filer.Client {
	t.Helper()
	client, err := filer.New(filer.Config{
		BaseURLs:   []string{server.URL},
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("filer.New() error = %v", err)
	}
	return client
}

func assertQuery(t *testing.T, got string, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("query value = %q, want %q", got, want)
	}
}

func assertNotFound(t *testing.T, err error) {
	t.Helper()
	var httpErr *httpx.Error
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want *httpx.Error", err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", httpErr.StatusCode, http.StatusNotFound)
	}
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
