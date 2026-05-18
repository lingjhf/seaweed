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
	resp, err := client.Put(context.Background(), "/docs/report.txt", strings.NewReader("hello"), filer.PutOptions{
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
	resp, err := client.Append(context.Background(), "/docs/report.txt", strings.NewReader("-tail"), filer.PutOptions{ContentLength: 5})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if resp.Size != 10 {
		t.Fatalf("Size = %d, want 10", resp.Size)
	}

	offset := int64(5)
	if _, err := client.Append(context.Background(), "/docs/report.txt", strings.NewReader("-tail"), filer.PutOptions{Offset: &offset}); err == nil {
		t.Fatal("Append() with offset error = nil, want error")
	}
}

func TestListBuildsJSONRequest(t *testing.T) {
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
				{"FullPath": "/docs/report.txt", "FileSize": 5},
			},
			"Limit":        2,
			"LastFileName": "report.txt",
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	resp, err := client.List(context.Background(), "/docs", filer.ListOptions{
		Limit:              2,
		LastFileName:       "a.txt",
		NamePattern:        "*.txt",
		NamePatternExclude: "*.tmp",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("Entries len = %d, want 1", len(resp.Entries))
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
				"FileSize": 5,
				"chunks": []map[string]any{
					{"file_id": "7,abc", "size": 5, "e_tag": "tag"},
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
	if err := client.Mkdir(context.Background(), "/docs"); err != nil {
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
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}
	header, err := client.Head(context.Background(), "/docs/report.txt")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if header.Get("Seaweed-Owner") != "sdk" {
		t.Fatalf("Seaweed-Owner = %q, want sdk", header.Get("Seaweed-Owner"))
	}
	entry, err := client.Stat(context.Background(), "/docs/report.txt", filer.StatOptions{ResolveManifest: true})
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if entry.FullPath != "/docs/report.txt" || entry.FileSize != 5 || len(entry.Chunks) != 1 {
		t.Fatalf("Stat() = %+v, want decoded entry", entry)
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
	if err := client.Copy(context.Background(), "/src.txt", "/dst.txt"); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	if err := client.Move(context.Background(), "/dst.txt", "/moved.txt"); err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if err := client.SetTags(context.Background(), "/moved.txt", map[string]string{"Owner": "sdk"}); err != nil {
		t.Fatalf("SetTags() error = %v", err)
	}
	if err := client.DeleteTags(context.Background(), "/moved.txt", "Owner"); err != nil {
		t.Fatalf("DeleteTags() error = %v", err)
	}
	if len(requests) != 4 {
		t.Fatalf("request count = %d, want 4", len(requests))
	}
}

func TestValidationAndHTTPErrorResponses(t *testing.T) {
	t.Parallel()

	if _, err := filer.New(filer.Config{}); err == nil {
		t.Fatal("filer.New() error = nil, want base urls error")
	}

	clientWithBaseURL, err := filer.New(filer.Config{
		BaseURLs:   []string{"http://example.test"},
		HTTPClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatalf("filer.New() error = %v", err)
	}
	if _, err := clientWithBaseURL.Get(context.Background(), "", filer.GetOptions{}); err == nil {
		t.Fatal("Get() error = nil, want path error")
	}
	if _, err := clientWithBaseURL.List(context.Background(), "", filer.ListOptions{}); err == nil {
		t.Fatal("List() error = nil, want path error")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	resp, err := client.Get(context.Background(), "/missing.txt", filer.GetOptions{})
	if err == nil {
		if resp != nil {
			resp.Body.Close()
		}
		t.Fatal("Get() error = nil, want status error")
	}
	assertHTTPStatus(t, err, http.StatusNotFound)
	header, err := client.Head(context.Background(), "/missing.txt")
	if err == nil {
		t.Fatalf("Head() = %v, nil, want status error", header)
	}
	assertHTTPStatus(t, err, http.StatusNotFound)
	if err := client.Delete(context.Background(), "/missing.txt", filer.DeleteOptions{}); err == nil {
		t.Fatal("Delete() error = nil, want status error")
	} else {
		assertHTTPStatus(t, err, http.StatusNotFound)
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

func assertHTTPStatus(t *testing.T, err error, want int) {
	t.Helper()
	var httpErr *httpx.Error
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want *httpx.Error", err)
	}
	if httpErr.StatusCode != want {
		t.Fatalf("status = %d, want %d", httpErr.StatusCode, want)
	}
}
