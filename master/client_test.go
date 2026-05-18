package master_test

import (
	"context"
	"encoding/json"
	"errors"
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

func TestClientReturnsJSONAPIErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "no writable volumes",
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.Assign(context.Background(), master.AssignOptions{})
	if err == nil {
		t.Fatal("Assign() error = nil, want API error")
	}
	assertAPIError(t, err, "no writable volumes")
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
				"Version": "test",
				"Topology": map[string]any{
					"Free": 3,
					"Max":  7,
					"DataCenters": []map[string]any{
						{
							"Id":   "dc1",
							"Free": 3,
							"Max":  7,
							"Racks": []map[string]any{
								{
									"Id":   "rack1",
									"Free": 3,
									"Max":  7,
									"DataNodes": []map[string]any{
										{
											"Url":       "127.0.0.1:8080",
											"PublicUrl": "localhost:8080",
											"Free":      3,
											"Max":       7,
											"Volumes":   4,
										},
									},
								},
							},
						},
					},
					"layouts": []map[string]any{
						{
							"collection":  "photos",
							"replication": "001",
							"writables":   []int{1, 2},
						},
					},
				},
			})
		case "/vol/status":
			if r.Method != http.MethodGet {
				t.Fatalf("volume status method = %s, want GET", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Version": "test",
				"Volumes": map[string]any{
					"Free": 5,
					"Max":  9,
					"DataCenters": map[string]any{
						"DefaultDataCenter": map[string]any{
							"DefaultRack": map[string]any{
								"127.0.0.1:8080": []map[string]any{
									{
										"Id":   1,
										"Size": 313888,
										"ReplicaPlacement": map[string]any{
											"SameRackCount":       2,
											"DiffRackCount":       1,
											"DiffDataCenterCount": 0,
										},
										"Ttl": map[string]any{
											"Count": 3,
											"Unit":  1,
										},
										"DiskType":          "ssd1",
										"Collection":        "photos",
										"Version":           3,
										"FileCount":         4,
										"DeleteCount":       1,
										"DeletedByteCount":  8,
										"ReadOnly":          true,
										"CompactRevision":   2,
										"ModifiedAtSecond":  1612388794,
										"RemoteStorageName": "remote",
										"RemoteStorageKey":  "key",
									},
								},
							},
						},
					},
				},
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
	if dirStatus.Version != "test" {
		t.Fatalf("DirStatus().Version = %q, want test", dirStatus.Version)
	}
	if dirStatus.Topology.Free != 3 || dirStatus.Topology.Max != 7 {
		t.Fatalf("DirStatus().Topology capacity = %d/%d, want 3/7", dirStatus.Topology.Free, dirStatus.Topology.Max)
	}
	if len(dirStatus.Topology.DataCenters) != 1 {
		t.Fatalf("DirStatus().Topology.DataCenters len = %d, want 1", len(dirStatus.Topology.DataCenters))
	}
	dataCenter := dirStatus.Topology.DataCenters[0]
	if dataCenter.ID != "dc1" || len(dataCenter.Racks) != 1 {
		t.Fatalf("DirStatus().Topology.DataCenters[0] = %+v, want dc1 with one rack", dataCenter)
	}
	rack := dataCenter.Racks[0]
	if rack.ID != "rack1" || len(rack.DataNodes) != 1 {
		t.Fatalf("DirStatus().Topology.DataCenters[0].Racks[0] = %+v, want rack1 with one data node", rack)
	}
	node := rack.DataNodes[0]
	if node.URL != "127.0.0.1:8080" || node.PublicURL != "localhost:8080" || node.Volumes != 4 {
		t.Fatalf("DirStatus() data node = %+v, want decoded node", node)
	}
	if len(dirStatus.Topology.Layouts) != 1 {
		t.Fatalf("DirStatus().Topology.Layouts len = %d, want 1", len(dirStatus.Topology.Layouts))
	}
	layout := dirStatus.Topology.Layouts[0]
	if layout.Collection != "photos" || layout.Replication != "001" || len(layout.Writables) != 2 || layout.Writables[0] != 1 || layout.Writables[1] != 2 {
		t.Fatalf("DirStatus().Topology.Layouts[0] = %+v, want decoded layout", layout)
	}
	volumeStatus, err := client.VolumeStatus(context.Background())
	if err != nil {
		t.Fatalf("VolumeStatus() error = %v", err)
	}
	if volumeStatus.Version != "test" {
		t.Fatalf("VolumeStatus().Version = %q, want test", volumeStatus.Version)
	}
	if volumeStatus.Volumes.Free != 5 || volumeStatus.Volumes.Max != 9 {
		t.Fatalf("VolumeStatus().Volumes capacity = %d/%d, want 5/9", volumeStatus.Volumes.Free, volumeStatus.Volumes.Max)
	}
	volumes := volumeStatus.Volumes.DataCenters["DefaultDataCenter"]["DefaultRack"]["127.0.0.1:8080"]
	if len(volumes) != 1 {
		t.Fatalf("VolumeStatus() volumes len = %d, want 1", len(volumes))
	}
	volume := volumes[0]
	if volume.ID != 1 || volume.Size != 313888 || volume.Collection != "photos" || !volume.ReadOnly {
		t.Fatalf("VolumeStatus() volume = %+v, want decoded volume", volume)
	}
	if volume.ReplicaPlacement.SameRackCount != 2 || volume.ReplicaPlacement.DiffRackCount != 1 || volume.ReplicaPlacement.DiffDataCenterCount != 0 {
		t.Fatalf("VolumeStatus() replica placement = %+v, want decoded replica placement", volume.ReplicaPlacement)
	}
	if volume.TTL.Count != 3 || volume.TTL.Unit != 1 {
		t.Fatalf("VolumeStatus() ttl = %+v, want decoded ttl", volume.TTL)
	}
	if volume.FileCount != 4 || volume.DeleteCount != 1 || volume.DeletedByteCount != 8 || volume.CompactRevision != 2 || volume.ModifiedAtSecond != 1612388794 {
		t.Fatalf("VolumeStatus() counters = %+v, want decoded counters", volume)
	}
	if volume.RemoteStorageName != "remote" || volume.RemoteStorageKey != "key" {
		t.Fatalf("VolumeStatus() remote storage = %q/%q, want remote/key", volume.RemoteStorageName, volume.RemoteStorageKey)
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
