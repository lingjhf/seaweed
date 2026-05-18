package filer_test

import (
	"context"
	"encoding/json"
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
		if r.URL.Query().Get("ttl") != "3d" {
			t.Fatalf("ttl = %q, want 3d", r.URL.Query().Get("ttl"))
		}
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
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

	client := filer.New(filer.Config{
		BaseURL: server.URL,
		HTTP:    httpx.NewClient(httpx.Config{HTTPClient: server.Client()}),
	})
	resp, err := client.Put(context.Background(), "/docs/report.txt", strings.NewReader("hello"), filer.PutOptions{
		TTL:           "3d",
		ContentType:   "text/plain",
		ContentLength: 5,
		SeaweedHeaders: map[string]string{
			"Owner": "sdk",
		},
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if resp.Size != 5 {
		t.Fatalf("Size = %d, want 5", resp.Size)
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
		if r.URL.Query().Get("limit") != "2" {
			t.Fatalf("limit = %q, want 2", r.URL.Query().Get("limit"))
		}
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

	client := filer.New(filer.Config{
		BaseURL: server.URL,
		HTTP:    httpx.NewClient(httpx.Config{HTTPClient: server.Client()}),
	})
	resp, err := client.List(context.Background(), "/docs", filer.ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("Entries len = %d, want 1", len(resp.Entries))
	}
}
