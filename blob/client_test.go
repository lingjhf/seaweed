package blob_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lingjhf/seaweed/blob"
	"github.com/lingjhf/seaweed/master"
)

func TestPutAssignsAndUploads(t *testing.T) {
	t.Parallel()

	var volumeURL string
	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/7,abc" {
			t.Fatalf("volume path = %q", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upload: %v", err)
		}
		if string(body) != "hello" {
			t.Fatalf("body = %q, want hello", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "7,abc",
			"size": 5,
		})
	}))
	defer volumeServer.Close()
	volumeURL = strings.TrimPrefix(volumeServer.URL, "http://")

	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dir/assign" {
			t.Fatalf("master path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"fid": "7,abc",
			"url": volumeURL,
		})
	}))
	defer masterServer.Close()

	masterClient, err := master.New(master.Config{
		BaseURLs:   []string{masterServer.URL},
		HTTPClient: masterServer.Client(),
	})
	if err != nil {
		t.Fatalf("master.New() error = %v", err)
	}
	client, err := blob.New(blob.Config{
		Master:     masterClient,
		HTTPClient: masterServer.Client(),
	})
	if err != nil {
		t.Fatalf("blob.New() error = %v", err)
	}

	resp, err := client.Put(context.Background(), strings.NewReader("hello"), blob.PutOptions{
		ContentLength: 5,
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if resp.FileID != "7,abc" {
		t.Fatalf("FileID = %q, want 7,abc", resp.FileID)
	}
}

func TestNewRequiresMasterClient(t *testing.T) {
	t.Parallel()

	if _, err := blob.New(blob.Config{}); err == nil {
		t.Fatal("blob.New() error = nil, want master client error")
	}
}

func TestPutUsesPublicURLAndAssignOptions(t *testing.T) {
	t.Parallel()

	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/9,abc" {
			t.Fatalf("volume path = %q, want /9,abc", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Fatalf("Content-Type = %q, want text/plain", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Content-Disposition") != `inline; filename="file.txt"` {
			t.Fatalf("Content-Disposition = %q", r.Header.Get("Content-Disposition"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "9,abc",
			"size": 5,
			"eTag": "etag",
		})
	}))
	defer volumeServer.Close()

	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dir/assign" {
			t.Fatalf("master path = %q, want /dir/assign", r.URL.Path)
		}
		query := r.URL.Query()
		assertQuery(t, query.Get("collection"), "photos")
		assertQuery(t, query.Get("dataCenter"), "dc1")
		assertQuery(t, query.Get("rack"), "rack1")
		assertQuery(t, query.Get("replication"), "001")
		assertQuery(t, query.Get("ttl"), "3d")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"fid":       "9,abc",
			"url":       "127.0.0.1:1",
			"publicUrl": strings.TrimPrefix(volumeServer.URL, "http://"),
		})
	}))
	defer masterServer.Close()

	client := newTestClient(t, masterServer, true)
	resp, err := client.Put(context.Background(), strings.NewReader("hello"), blob.PutOptions{
		Collection:    "photos",
		DataCenter:    "dc1",
		Rack:          "rack1",
		Replication:   "001",
		TTL:           "3d",
		ContentType:   "text/plain",
		ContentLength: 5,
		Filename:      "file.txt",
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if resp.FileID != "9,abc" || resp.Size != 5 || resp.ETag != "etag" {
		t.Fatalf("Put() = %+v, want file id, size, etag", resp)
	}
}

func TestPutValidatesAssignResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body map[string]any
	}{
		{
			name: "empty fid",
			body: map[string]any{
				"url": "127.0.0.1:8080",
			},
		},
		{
			name: "empty volume url",
			body: map[string]any{
				"fid": "9,abc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.body)
			}))
			defer masterServer.Close()

			client := newTestClient(t, masterServer, false)
			if _, err := client.Put(context.Background(), strings.NewReader("hello"), blob.PutOptions{}); err == nil {
				t.Fatal("Put() error = nil, want error")
			}
		})
	}
}

func TestGetHeadDeleteLookupCacheAndInvalidate(t *testing.T) {
	t.Parallel()

	var statusCode atomic.Int32
	statusCode.Store(http.StatusOK)
	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if code := int(statusCode.Load()); code != http.StatusOK {
			http.Error(w, "missing", code)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if r.Header.Get("Range") != "bytes=0-4" {
				t.Fatalf("Range = %q, want bytes=0-4", r.Header.Get("Range"))
			}
			_, _ = w.Write([]byte("hello"))
		case http.MethodHead:
			w.Header().Set("ETag", `"etag"`)
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer volumeServer.Close()

	var lookups atomic.Int32
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dir/lookup" {
			t.Fatalf("master path = %q, want /dir/lookup", r.URL.Path)
		}
		lookups.Add(1)
		query := r.URL.Query()
		assertQuery(t, query.Get("volumeId"), "9")
		assertQuery(t, query.Get("read"), "yes")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"locations": []map[string]string{
				{
					"url": strings.TrimPrefix(volumeServer.URL, "http://"),
				},
			},
		})
	}))
	defer masterServer.Close()

	client := newTestClient(t, masterServer, false)
	resp, err := client.Get(context.Background(), "9,abc", blob.GetOptions{Range: "bytes=0-4"})
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
	if lookups.Load() != 1 {
		t.Fatalf("lookups = %d, want 1", lookups.Load())
	}

	header, err := client.Head(context.Background(), "9,abc")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if header.Get("ETag") != `"etag"` {
		t.Fatalf("ETag = %q, want etag", header.Get("ETag"))
	}
	if lookups.Load() != 1 {
		t.Fatalf("lookups after Head = %d, want cached 1", lookups.Load())
	}

	statusCode.Store(http.StatusNotFound)
	if _, err := client.Get(context.Background(), "9,abc", blob.GetOptions{Range: "bytes=0-4"}); err == nil {
		t.Fatal("Get() after 404 error = nil, want error")
	}
	statusCode.Store(http.StatusOK)
	if err := client.Delete(context.Background(), "9,abc"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if lookups.Load() != 2 {
		t.Fatalf("lookups after invalidation = %d, want 2", lookups.Load())
	}
}

func TestLookupErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body map[string]any
	}{
		{
			name: "no locations",
			body: map[string]any{
				"locations": []map[string]string{},
			},
		},
		{
			name: "empty lookup url",
			body: map[string]any{
				"locations": []map[string]string{{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.body)
			}))
			defer masterServer.Close()

			client := newTestClient(t, masterServer, false)
			if _, err := client.Get(context.Background(), "9,abc", blob.GetOptions{}); err == nil {
				t.Fatal("Get() error = nil, want error")
			}
		})
	}
}

func TestInvalidFileID(t *testing.T) {
	t.Parallel()

	masterServer := httptest.NewServer(http.NotFoundHandler())
	defer masterServer.Close()

	client := newTestClient(t, masterServer, false)
	if _, err := client.Get(context.Background(), "", blob.GetOptions{}); err == nil {
		t.Fatal("Get() error = nil, want invalid file id error")
	}
	if _, err := client.Head(context.Background(), ""); err == nil {
		t.Fatal("Head() error = nil, want invalid file id error")
	}
	if err := client.Delete(context.Background(), ""); err == nil {
		t.Fatal("Delete() error = nil, want invalid file id error")
	}
}

func newTestClient(t *testing.T, server *httptest.Server, usePublicURLs bool) *blob.Client {
	t.Helper()
	masterClient, err := master.New(master.Config{
		BaseURLs:   []string{server.URL},
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("master.New() error = %v", err)
	}
	client, err := blob.New(blob.Config{
		Master:        masterClient,
		HTTPClient:    server.Client(),
		UsePublicURLs: usePublicURLs,
	})
	if err != nil {
		t.Fatalf("blob.New() error = %v", err)
	}
	return client
}

func assertQuery(t *testing.T, got string, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("query value = %q, want %q", got, want)
	}
}
