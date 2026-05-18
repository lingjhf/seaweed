package volume_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lingjhf/seaweed/internal/httpx"
	"github.com/lingjhf/seaweed/volume"
)

func TestPutSendsRawBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/3,abc" {
			t.Fatalf("path = %s, want /3,abc", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "hello" {
			t.Fatalf("body = %q, want hello", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "3,abc",
			"size": 5,
			"eTag": "tag",
		})
	}))
	defer server.Close()

	client := volume.New(volume.Config{
		BaseURL: server.URL,
		HTTP: httpx.NewClient(httpx.Config{
			HTTPClient: server.Client(),
		}),
	})
	resp, err := client.Put(context.Background(), "3,abc", stringsReader("hello"), volume.PutOptions{
		ContentType:   "text/plain",
		ContentLength: 5,
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if resp.Size != 5 {
		t.Fatalf("Size = %d, want 5", resp.Size)
	}
}

func TestGetReturnsStream(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	client := volume.New(volume.Config{
		BaseURL: server.URL,
		HTTP: httpx.NewClient(httpx.Config{
			HTTPClient: server.Client(),
		}),
	})
	resp, err := client.Get(context.Background(), "3,abc", volume.GetOptions{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}
}

func stringsReader(s string) io.Reader {
	return strings.NewReader(s)
}
