package blob_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lingjhf/seaweed/blob"
	"github.com/lingjhf/seaweed/internal/httpx"
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

	httpClient := httpx.NewClient(httpx.Config{HTTPClient: masterServer.Client()})
	client := blob.New(blob.Config{
		Master: master.New(master.Config{
			BaseURL: masterServer.URL,
			HTTP:    httpClient,
		}),
		HTTP: httpClient,
	})

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
