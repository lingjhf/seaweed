package master_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

	client := newTestClient(t, server)

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

func TestNewRequiresBaseURLs(t *testing.T) {
	t.Parallel()

	if _, err := master.New(master.Config{}); err == nil {
		t.Fatal("master.New() error = nil, want base urls error")
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

	client := newTestClient(t, server)

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

func TestClientVolumeManagementRequests(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch r.URL.Path {
		case "/vol/vacuum":
			if r.Method != http.MethodGet {
				t.Fatalf("vacuum method = %s, want GET", r.Method)
			}
			assertQuery(t, query.Get("garbageThreshold"), "0.35")
			w.WriteHeader(http.StatusOK)
		case "/vol/grow":
			if r.Method != http.MethodGet {
				t.Fatalf("grow method = %s, want GET", r.Method)
			}
			assertQuery(t, query.Get("count"), "2")
			assertQuery(t, query.Get("collection"), "photos")
			assertQuery(t, query.Get("dataCenter"), "dc1")
			assertQuery(t, query.Get("rack"), "rack1")
			assertQuery(t, query.Get("dataNode"), "node1")
			assertQuery(t, query.Get("replication"), "001")
			assertQuery(t, query.Get("ttl"), "3d")
			assertQuery(t, query.Get("preallocate"), "4096")
			assertQuery(t, query.Get("memoryMapMaxSizeMb"), "128")
			assertQuery(t, query.Get("disk"), "hdd")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"count": 2,
			})
		case "/col/delete":
			if r.Method != http.MethodGet {
				t.Fatalf("delete collection method = %s, want GET", r.Method)
			}
			assertQuery(t, query.Get("collection"), "photos")
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if err := client.Vacuum(context.Background(), 0.35); err != nil {
		t.Fatalf("Vacuum() error = %v", err)
	}
	grow, err := client.Grow(context.Background(), master.GrowOptions{
		Count:              2,
		Collection:         "photos",
		DataCenter:         "dc1",
		Rack:               "rack1",
		DataNode:           "node1",
		Replication:        "001",
		TTL:                "3d",
		Preallocate:        4096,
		MemoryMapMaxSizeMB: 128,
		Disk:               "hdd",
	})
	if err != nil {
		t.Fatalf("Grow() error = %v", err)
	}
	if grow.Count != 2 {
		t.Fatalf("Grow().Count = %d, want 2", grow.Count)
	}
	if err := client.DeleteCollection(context.Background(), "photos"); err != nil {
		t.Fatalf("DeleteCollection() error = %v", err)
	}
}

func TestClientStatusRequests(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cluster/status":
			if r.Method != http.MethodGet {
				t.Fatalf("cluster status method = %s, want GET", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"IsLeader": true,
				"Leader":   "127.0.0.1:9333",
				"Peers":    []string{"127.0.0.1:9333"},
			})
		case "/cluster/healthz":
			if r.Method != http.MethodHead {
				t.Fatalf("health method = %s, want HEAD", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		case "/dir/status":
			if r.Method != http.MethodGet {
				t.Fatalf("dir status method = %s, want GET", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Topology": "ok",
			})
		case "/vol/status":
			if r.Method != http.MethodGet {
				t.Fatalf("volume status method = %s, want GET", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Version": "test",
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	cluster, err := client.ClusterStatus(context.Background())
	if err != nil {
		t.Fatalf("ClusterStatus() error = %v", err)
	}
	if !cluster.IsLeader || cluster.Leader != "127.0.0.1:9333" || len(cluster.Peers) != 1 {
		t.Fatalf("ClusterStatus() = %+v, want decoded status", cluster)
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	dirStatus, err := client.DirStatus(context.Background())
	if err != nil {
		t.Fatalf("DirStatus() error = %v", err)
	}
	if dirStatus["Topology"] != "ok" {
		t.Fatalf("DirStatus()[Topology] = %v, want ok", dirStatus["Topology"])
	}
	volumeStatus, err := client.VolumeStatus(context.Background())
	if err != nil {
		t.Fatalf("VolumeStatus() error = %v", err)
	}
	if volumeStatus["Version"] != "test" {
		t.Fatalf("VolumeStatus()[Version] = %v, want test", volumeStatus["Version"])
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

func newTestClient(t *testing.T, server *httptest.Server) *master.Client {
	t.Helper()
	client, err := master.New(master.Config{
		BaseURLs:   []string{server.URL},
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("master.New() error = %v", err)
	}
	return client
}

func assertQuery(t *testing.T, got string, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("query value = %q, want %q", got, want)
	}
}
