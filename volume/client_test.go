package volume_test

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

	client := newTestClient(t, server)
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

func TestPutSendsOptionalHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		assertQuery(t, query.Get("fsync"), "true")
		assertQuery(t, query.Get("type"), "replicate")
		assertQuery(t, query.Get("ts"), "1716181200")
		assertQuery(t, query.Get("cm"), "true")
		if r.Header.Get("Content-Encoding") != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", r.Header.Get("Content-Encoding"))
		}
		if r.Header.Get("Content-MD5") != "md5" {
			t.Fatalf("Content-MD5 = %q, want md5", r.Header.Get("Content-MD5"))
		}
		if r.Header.Get("Content-Disposition") != `inline; filename="a\"b.txt"` {
			t.Fatalf("Content-Disposition = %q", r.Header.Get("Content-Disposition"))
		}
		if r.Header.Get("Seaweed-Owner") != "sdk" {
			t.Fatalf("Seaweed-Owner = %q, want sdk", r.Header.Get("Seaweed-Owner"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "3,abc",
			"size": 5,
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.Put(context.Background(), "/3,abc", stringsReader("hello"), volume.PutOptions{
		ContentEncoding:  "gzip",
		ContentMD5:       "md5",
		Filename:         `a"b.txt`,
		ContentLength:    5,
		Fsync:            true,
		Replicate:        true,
		ModifiedAtSecond: 1716181200,
		ChunkManifest:    true,
		SeaweedHeaders: map[string]string{
			"Owner": "sdk",
		},
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
}

func TestPutReturnsJSONAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "write failed",
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.Put(context.Background(), "3,abc", stringsReader("hello"), volume.PutOptions{
		ContentLength: 5,
	})
	if err == nil {
		t.Fatal("Put() error = nil, want API error")
	}
	assertAPIError(t, err, "write failed")
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

	client := newTestClient(t, server)
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

func TestGetSendsRange(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=1-3" {
			t.Fatalf("Range = %q, want bytes=1-3", r.Header.Get("Range"))
		}
		_, _ = w.Write([]byte("ell"))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	resp, err := client.Get(context.Background(), "3,abc", volume.GetOptions{Range: "bytes=1-3"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "ell" {
		t.Fatalf("body = %q, want ell", body)
	}
}

func TestGetAndHeadSendReadOptions(t *testing.T) {
	t.Parallel()

	cropX1 := 0
	cropY1 := 1
	cropX2 := 20
	cropY2 := 21
	chunkManifest := false
	modifiedSince := time.Date(2026, 5, 19, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	wantModifiedSince := "Tue, 19 May 2026 01:30:00 GMT"

	tests := []struct {
		name   string
		method string
		call   func(*volume.Client) error
	}{
		{
			name:   "get",
			method: http.MethodGet,
			call: func(client *volume.Client) error {
				resp, err := client.Get(context.Background(), "3,abc", volume.GetOptions{
					Range:           "bytes=1-3",
					ReadDeleted:     true,
					Width:           100,
					Height:          80,
					Mode:            "fit",
					CropX1:          &cropX1,
					CropY1:          &cropY1,
					CropX2:          &cropX2,
					CropY2:          &cropY2,
					ChunkManifest:   &chunkManifest,
					IfModifiedSince: modifiedSince,
					IfNoneMatch:     `"tag"`,
					AcceptEncoding:  "gzip",
				})
				if err != nil {
					return err
				}
				resp.Body.Close()
				return nil
			},
		},
		{
			name:   "head",
			method: http.MethodHead,
			call: func(client *volume.Client) error {
				_, err := client.Head(context.Background(), "3,abc", volume.HeadOptions{
					Range:           "bytes=1-3",
					ReadDeleted:     true,
					Width:           100,
					Height:          80,
					Mode:            "fit",
					CropX1:          &cropX1,
					CropY1:          &cropY1,
					CropX2:          &cropX2,
					CropY2:          &cropY2,
					ChunkManifest:   &chunkManifest,
					IfModifiedSince: modifiedSince,
					IfNoneMatch:     `"tag"`,
					AcceptEncoding:  "gzip",
				})
				return err
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
				query := r.URL.Query()
				assertQuery(t, query.Get("readDeleted"), "true")
				assertQuery(t, query.Get("width"), "100")
				assertQuery(t, query.Get("height"), "80")
				assertQuery(t, query.Get("mode"), "fit")
				assertQuery(t, query.Get("crop_x1"), "0")
				assertQuery(t, query.Get("crop_y1"), "1")
				assertQuery(t, query.Get("crop_x2"), "20")
				assertQuery(t, query.Get("crop_y2"), "21")
				assertQuery(t, query.Get("cm"), "false")
				if r.Header.Get("Range") != "bytes=1-3" {
					t.Fatalf("Range = %q, want bytes=1-3", r.Header.Get("Range"))
				}
				if r.Header.Get("If-Modified-Since") != wantModifiedSince {
					t.Fatalf("If-Modified-Since = %q, want %q", r.Header.Get("If-Modified-Since"), wantModifiedSince)
				}
				if r.Header.Get("If-None-Match") != `"tag"` {
					t.Fatalf("If-None-Match = %q, want tag", r.Header.Get("If-None-Match"))
				}
				if r.Header.Get("Accept-Encoding") != "gzip" {
					t.Fatalf("Accept-Encoding = %q, want gzip", r.Header.Get("Accept-Encoding"))
				}
				_, _ = w.Write([]byte("ok"))
			}))
			defer server.Close()

			client := newTestClient(t, server)
			if err := tt.call(client); err != nil {
				t.Fatalf("call error = %v", err)
			}
		})
	}
}

func TestHeadDeleteStatusAndHealth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/3,abc":
			w.Header().Set("ETag", `"tag"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && r.URL.Path == "/3,abc":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Version": "test",
				"DiskStatuses": []map[string]any{
					{
						"dir":          "/data",
						"all":          1000,
						"used":         400,
						"free":         600,
						"percent_free": 60.5,
						"percent_used": 39.5,
						"disk_type":    "ssd",
					},
				},
				"Volumes": []map[string]any{
					{
						"Id":   3,
						"Size": 2048,
						"ReplicaPlacement": map[string]any{
							"SameRackCount":       1,
							"DiffRackCount":       2,
							"DiffDataCenterCount": 3,
						},
						"Ttl": map[string]any{
							"Count": 7,
							"Unit":  1,
						},
						"DiskType":          "ssd",
						"DiskId":            2,
						"Collection":        "photos",
						"Version":           3,
						"FileCount":         4,
						"DeleteCount":       1,
						"DeletedByteCount":  8,
						"ReadOnly":          true,
						"CompactRevision":   5,
						"ModifiedAtSecond":  1612388794,
						"RemoteStorageName": "remote",
						"RemoteStorageKey":  "key",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	header, err := client.Head(context.Background(), "3,abc", volume.HeadOptions{})
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if header.Get("ETag") != `"tag"` {
		t.Fatalf("ETag = %q, want tag", header.Get("ETag"))
	}
	if err := client.Delete(context.Background(), "3,abc"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Version != "test" {
		t.Fatalf("Status().Version = %q, want test", status.Version)
	}
	if len(status.DiskStatuses) != 1 {
		t.Fatalf("Status().DiskStatuses len = %d, want 1", len(status.DiskStatuses))
	}
	disk := status.DiskStatuses[0]
	if disk.Dir != "/data" || disk.All != 1000 || disk.Used != 400 || disk.Free != 600 || disk.DiskType != "ssd" {
		t.Fatalf("Status().DiskStatuses[0] = %+v, want decoded disk status", disk)
	}
	if disk.PercentFree != 60.5 || disk.PercentUsed != 39.5 {
		t.Fatalf("Status().DiskStatuses[0] percentages = %f/%f, want 60.5/39.5", disk.PercentFree, disk.PercentUsed)
	}
	if len(status.Volumes) != 1 {
		t.Fatalf("Status().Volumes len = %d, want 1", len(status.Volumes))
	}
	vol := status.Volumes[0]
	if vol.ID != 3 || vol.Size != 2048 || vol.DiskID != 2 || vol.Collection != "photos" || vol.Version != 3 || !vol.ReadOnly {
		t.Fatalf("Status().Volumes[0] = %+v, want decoded volume", vol)
	}
	if vol.ReplicaPlacement.SameRackCount != 1 || vol.ReplicaPlacement.DiffRackCount != 2 || vol.ReplicaPlacement.DiffDataCenterCount != 3 {
		t.Fatalf("Status().Volumes[0].ReplicaPlacement = %+v, want decoded placement", vol.ReplicaPlacement)
	}
	if vol.TTL.Count != 7 || vol.TTL.Unit != 1 {
		t.Fatalf("Status().Volumes[0].TTL = %+v, want decoded ttl", vol.TTL)
	}
	if vol.FileCount != 4 || vol.DeleteCount != 1 || vol.DeletedByteCount != 8 || vol.CompactRevision != 5 || vol.ModifiedAtSecond != 1612388794 {
		t.Fatalf("Status().Volumes[0] counters = %+v, want decoded counters", vol)
	}
	if vol.RemoteStorageName != "remote" || vol.RemoteStorageKey != "key" {
		t.Fatalf("Status().Volumes[0] remote storage = %q/%q, want remote/key", vol.RemoteStorageName, vol.RemoteStorageKey)
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestHTTPErrorResponses(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	resp, err := client.Get(context.Background(), "3,abc", volume.GetOptions{})
	if err == nil {
		if resp != nil {
			resp.Body.Close()
		}
		t.Fatal("Get() error = nil, want error")
	}
	assertHTTPStatus(t, err, http.StatusNotFound)
	header, err := client.Head(context.Background(), "3,abc", volume.HeadOptions{})
	if err == nil {
		t.Fatalf("Head() = %v, nil, want error", header)
	}
	assertHTTPStatus(t, err, http.StatusNotFound)
	if err := client.Delete(context.Background(), "3,abc"); err == nil {
		t.Fatal("Delete() error = nil, want error")
	} else {
		assertHTTPStatus(t, err, http.StatusNotFound)
	}
}

func TestDeleteReturnsStatusAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/3,abc" {
			t.Fatalf("path = %q, want /3,abc", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "delete failed",
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	err := client.Delete(context.Background(), "3,abc")
	if err == nil {
		t.Fatal("Delete() error = nil, want API error")
	}
	assertAPIError(t, err, "delete failed")
}

func TestBaseURLsAndFileIDValidation(t *testing.T) {
	t.Parallel()

	if _, err := volume.New(volume.Config{}); err == nil {
		t.Fatal("volume.New() error = nil, want base urls error")
	}
	if _, err := volume.New(volume.Config{BaseURLs: []string{"relative"}}); err == nil {
		t.Fatal("volume.New() error = nil, want invalid base url error")
	}
	if _, err := volume.New(volume.Config{
		BaseURLs: []string{"http://example.test"},
		EndpointPolicy: volume.EndpointPolicy{
			Mode: "random",
		},
	}); err == nil {
		t.Fatal("volume.New() error = nil, want invalid endpoint policy error")
	}

	clientWithBaseURL, err := volume.New(volume.Config{
		BaseURLs:   []string{"http://example.test"},
		HTTPClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatalf("volume.New() error = %v", err)
	}
	if _, err := clientWithBaseURL.Get(context.Background(), "", volume.GetOptions{}); err == nil {
		t.Fatal("Get() error = nil, want file id error")
	}
	if _, err := clientWithBaseURL.Put(context.Background(), "", stringsReader("body"), volume.PutOptions{}); err == nil {
		t.Fatal("Put() error = nil, want file id error")
	}
	if _, err := clientWithBaseURL.Head(context.Background(), "", volume.HeadOptions{}); err == nil {
		t.Fatal("Head() error = nil, want file id error")
	}
	if err := clientWithBaseURL.Delete(context.Background(), ""); err == nil {
		t.Fatal("Delete() error = nil, want file id error")
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

func stringsReader(s string) io.Reader {
	return strings.NewReader(s)
}

func assertQuery(t *testing.T, got string, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("query value = %q, want %q", got, want)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *volume.Client {
	t.Helper()
	client, err := volume.New(volume.Config{
		BaseURLs:   []string{server.URL},
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("volume.New() error = %v", err)
	}
	return client
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
