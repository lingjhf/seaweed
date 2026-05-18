package master_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lingjhf/seaweed/internal/httpx"
	"github.com/lingjhf/seaweed/master"
)

func TestClientAssignBuildsRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dir/assign" {
			t.Fatalf("path = %q, want /dir/assign", r.URL.Path)
		}
		query := r.URL.Query()
		assertQuery(t, query.Get("count"), "2")
		assertQuery(t, query.Get("collection"), "photos")
		assertQuery(t, query.Get("replication"), "001")
		assertQuery(t, query.Get("ttl"), "3d")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count":     2,
			"fid":       "3,abc",
			"url":       "127.0.0.1:8080",
			"publicUrl": "localhost:8080",
		})
	}))
	defer server.Close()

	client := master.New(master.Config{
		BaseURL: server.URL,
		HTTP: httpx.NewClient(httpx.Config{
			HTTPClient: server.Client(),
		}),
	})

	resp, err := client.Assign(context.Background(), master.AssignOptions{
		Count:       2,
		Collection:  "photos",
		Replication: "001",
		TTL:         "3d",
	})
	if err != nil {
		t.Fatalf("Assign() error = %v", err)
	}
	if resp.FID != "3,abc" {
		t.Fatalf("FID = %q, want 3,abc", resp.FID)
	}
}

func TestClientLookupBuildsRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dir/lookup" {
			t.Fatalf("path = %q, want /dir/lookup", r.URL.Path)
		}
		query := r.URL.Query()
		assertQuery(t, query.Get("volumeId"), "3")
		assertQuery(t, query.Get("collection"), "photos")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"locations": []map[string]string{
				{
					"url":       "127.0.0.1:8080",
					"publicUrl": "localhost:8080",
				},
			},
		})
	}))
	defer server.Close()

	client := master.New(master.Config{
		BaseURL: server.URL,
		HTTP: httpx.NewClient(httpx.Config{
			HTTPClient: server.Client(),
		}),
	})

	resp, err := client.Lookup(context.Background(), "3", master.LookupOptions{
		Collection: "photos",
	})
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if len(resp.Locations) != 1 {
		t.Fatalf("locations len = %d, want 1", len(resp.Locations))
	}
}

func assertQuery(t *testing.T, got string, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("query value = %q, want %q", got, want)
	}
}
