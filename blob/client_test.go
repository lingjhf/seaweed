package blob_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

	masterServer := httptest.NewServer(http.NotFoundHandler())
	defer masterServer.Close()
	masterClient, err := master.New(master.Config{
		BaseURLs:   []string{masterServer.URL},
		HTTPClient: masterServer.Client(),
	})
	if err != nil {
		t.Fatalf("master.New() error = %v", err)
	}
	if _, err := blob.New(blob.Config{
		Master: masterClient,
		EndpointPolicy: blob.EndpointPolicy{
			Mode: "random",
		},
	}); err == nil {
		t.Fatal("blob.New() error = nil, want invalid endpoint policy error")
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

func TestPutUsesAssignAuthorizationWhenEnabled(t *testing.T) {
	t.Parallel()

	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer assign-token" {
			t.Fatalf("Authorization = %q, want Bearer assign-token", r.Header.Get("Authorization"))
		}
		_, _ = io.Copy(io.Discard, r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "9,abc",
			"size": 5,
		})
	}))
	defer volumeServer.Close()

	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Authorization", "Bearer assign-token")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"fid": "9,abc",
			"url": strings.TrimPrefix(volumeServer.URL, "http://"),
		})
	}))
	defer masterServer.Close()

	client := newTestClientWithConfig(t, masterServer, blob.Config{
		EnableVolumeAuthorization: true,
	})
	if _, err := client.Put(context.Background(), strings.NewReader("hello"), blob.PutOptions{ContentLength: 5}); err != nil {
		t.Fatalf("Put() error = %v", err)
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

func TestPutReturnsAssignAndUploadErrors(t *testing.T) {
	t.Parallel()

	t.Run("assign error", func(t *testing.T) {
		masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "master busy", http.StatusServiceUnavailable)
		}))
		defer masterServer.Close()

		client := newTestClient(t, masterServer, false)
		if _, err := client.Put(context.Background(), strings.NewReader("hello"), blob.PutOptions{}); err == nil {
			t.Fatal("Put() error = nil, want assign error")
		}
	})

	t.Run("assign api error", func(t *testing.T) {
		masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "no writable volumes",
			})
		}))
		defer masterServer.Close()

		client := newTestClient(t, masterServer, false)
		_, err := client.Put(context.Background(), strings.NewReader("hello"), blob.PutOptions{})
		if err == nil {
			t.Fatal("Put() error = nil, want assign API error")
		}
		assertAPIError(t, err, "no writable volumes")
	})

	t.Run("upload error", func(t *testing.T) {
		volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "volume busy", http.StatusServiceUnavailable)
		}))
		defer volumeServer.Close()
		masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"fid": "9,abc",
				"url": strings.TrimPrefix(volumeServer.URL, "http://"),
			})
		}))
		defer masterServer.Close()

		client := newTestClient(t, masterServer, false)
		if _, err := client.Put(context.Background(), strings.NewReader("hello"), blob.PutOptions{}); err == nil {
			t.Fatal("Put() error = nil, want upload error")
		}
	})

	t.Run("upload api error", func(t *testing.T) {
		volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "write failed",
			})
		}))
		defer volumeServer.Close()
		masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"fid": "9,abc",
				"url": strings.TrimPrefix(volumeServer.URL, "http://"),
			})
		}))
		defer masterServer.Close()

		client := newTestClient(t, masterServer, false)
		_, err := client.Put(context.Background(), strings.NewReader("hello"), blob.PutOptions{})
		if err == nil {
			t.Fatal("Put() error = nil, want upload API error")
		}
		assertAPIError(t, err, "write failed")
	})
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

func TestGetHeadDeleteUseLookupAuthorizationWhenEnabled(t *testing.T) {
	t.Parallel()

	wantAuthorizationByMethod := map[string]string{
		http.MethodGet:    "Bearer read-get-token",
		http.MethodHead:   "Bearer read-head-token",
		http.MethodDelete: "Bearer delete-token",
	}
	var volumeCalls atomic.Int32
	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		volumeCalls.Add(1)
		wantAuthorization := wantAuthorizationByMethod[r.Method]
		if r.Header.Get("Authorization") != wantAuthorization {
			t.Fatalf("%s Authorization = %q, want %q", r.Method, r.Header.Get("Authorization"), wantAuthorization)
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
		call := lookups.Add(1)
		query := r.URL.Query()
		assertQuery(t, query.Get("volumeId"), "9")
		assertQuery(t, query.Get("fileId"), "9,abc")
		switch call {
		case 1:
			assertQuery(t, query.Get("read"), "yes")
			w.Header().Set("Authorization", "Bearer read-get-token")
		case 2:
			assertQuery(t, query.Get("read"), "yes")
			w.Header().Set("Authorization", "Bearer read-head-token")
		case 3:
			assertQuery(t, query.Get("read"), "")
			w.Header().Set("Authorization", "Bearer delete-token")
		default:
			t.Fatalf("unexpected lookup call %d", call)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"locations": []map[string]string{
				{
					"url": strings.TrimPrefix(volumeServer.URL, "http://"),
				},
			},
		})
	}))
	defer masterServer.Close()

	client := newTestClientWithConfig(t, masterServer, blob.Config{
		EnableVolumeAuthorization: true,
	})
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
	header, err := client.Head(context.Background(), "9,abc")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if header.Get("ETag") != `"etag"` {
		t.Fatalf("ETag = %q, want etag", header.Get("ETag"))
	}
	if err := client.Delete(context.Background(), "9,abc"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if lookups.Load() != 3 {
		t.Fatalf("lookups = %d, want 3", lookups.Load())
	}
	if volumeCalls.Load() != 3 {
		t.Fatalf("volume calls = %d, want 3", volumeCalls.Load())
	}
}

func TestGetFailsOverAcrossLookupLocations(t *testing.T) {
	t.Parallel()

	var firstCalls atomic.Int32
	firstVolume := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls.Add(1)
		http.Error(w, "volume unavailable", http.StatusInternalServerError)
	}))
	defer firstVolume.Close()

	var secondCalls atomic.Int32
	secondVolume := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls.Add(1)
		if r.URL.Path != "/9,abc" {
			t.Fatalf("volume path = %q, want /9,abc", r.URL.Path)
		}
		_, _ = w.Write([]byte("replica"))
	}))
	defer secondVolume.Close()

	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"locations": []map[string]string{
				{
					"url": strings.TrimPrefix(firstVolume.URL, "http://"),
				},
				{
					"url": strings.TrimPrefix(secondVolume.URL, "http://"),
				},
			},
		})
	}))
	defer masterServer.Close()

	client := newTestClient(t, masterServer, false)
	resp, err := client.Get(context.Background(), "9,abc", blob.GetOptions{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "replica" {
		t.Fatalf("body = %q, want replica", body)
	}
	if firstCalls.Load() != 1 || secondCalls.Load() != 1 {
		t.Fatalf("volume calls = %d/%d, want 1/1", firstCalls.Load(), secondCalls.Load())
	}
}

func TestLookupUsesPublicURLAndDeduplicatesLocations(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("public"))
	}))
	defer volumeServer.Close()

	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		location := strings.TrimPrefix(volumeServer.URL, "http://")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"locations": []map[string]string{
				{
					"url":       "127.0.0.1:1",
					"publicUrl": location,
				},
				{
					"url":       "127.0.0.1:2",
					"publicUrl": location,
				},
			},
		})
	}))
	defer masterServer.Close()

	client := newTestClient(t, masterServer, true)
	assertGetBody(t, client, "9,abc", "public")
	if calls.Load() != 1 {
		t.Fatalf("volume calls = %d, want one deduplicated endpoint", calls.Load())
	}
}

func TestBlobLocationCacheTTLRefreshesLookup(t *testing.T) {
	t.Parallel()

	firstVolume := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first"))
	}))
	defer firstVolume.Close()
	secondVolume := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("second"))
	}))
	defer secondVolume.Close()

	var lookups atomic.Int32
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		location := strings.TrimPrefix(secondVolume.URL, "http://")
		if lookups.Add(1) == 1 {
			location = strings.TrimPrefix(firstVolume.URL, "http://")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"locations": []map[string]string{
				{
					"url": location,
				},
			},
		})
	}))
	defer masterServer.Close()

	client := newTestClientWithConfig(t, masterServer, blob.Config{
		LocationCacheTTL: 5 * time.Millisecond,
	})
	assertGetBody(t, client, "9,abc", "first")
	time.Sleep(20 * time.Millisecond)
	assertGetBody(t, client, "9,abc", "second")
	if lookups.Load() != 2 {
		t.Fatalf("lookups = %d, want 2", lookups.Load())
	}
}

func TestBlobEndpointPolicyRoundRobinUsesCachedVolumeClient(t *testing.T) {
	t.Parallel()

	firstVolume := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first"))
	}))
	defer firstVolume.Close()
	secondVolume := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("second"))
	}))
	defer secondVolume.Close()

	var lookups atomic.Int32
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"locations": []map[string]string{
				{
					"url": strings.TrimPrefix(firstVolume.URL, "http://"),
				},
				{
					"url": strings.TrimPrefix(secondVolume.URL, "http://"),
				},
			},
		})
	}))
	defer masterServer.Close()

	client := newTestClientWithConfig(t, masterServer, blob.Config{
		EndpointPolicy: blob.EndpointPolicy{
			Mode: "round-robin",
		},
	})
	assertGetBody(t, client, "9,abc", "first")
	assertGetBody(t, client, "9,abc", "second")
	if lookups.Load() != 1 {
		t.Fatalf("lookups = %d, want cached 1", lookups.Load())
	}
}

func TestCloseClearsLocationCache(t *testing.T) {
	t.Parallel()

	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("body"))
	}))
	defer volumeServer.Close()

	var lookups atomic.Int32
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
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
	assertGetBody(t, client, "9,abc", "body")
	client.Close()
	assertGetBody(t, client, "9,abc", "body")
	if lookups.Load() != 2 {
		t.Fatalf("lookups = %d, want cache cleared by Close", lookups.Load())
	}
}

func TestConcurrentGetCoalescesLookup(t *testing.T) {
	t.Parallel()

	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("body"))
	}))
	defer volumeServer.Close()

	var lookups atomic.Int32
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		time.Sleep(50 * time.Millisecond)
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
	start := make(chan struct{})
	errs := make(chan error, 16)
	for range 16 {
		go func() {
			<-start
			resp, err := client.Get(context.Background(), "9,abc", blob.GetOptions{})
			if err != nil {
				errs <- err
				return
			}
			_, err = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			errs <- err
		}()
	}

	close(start)
	for range 16 {
		if err := <-errs; err != nil {
			t.Fatalf("Get() error = %v", err)
		}
	}
	if lookups.Load() != 1 {
		t.Fatalf("lookups = %d, want coalesced 1", lookups.Load())
	}
}

func TestLookupWaitRespectsContextCancellation(t *testing.T) {
	t.Parallel()

	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("body"))
	}))
	defer volumeServer.Close()

	var lookups atomic.Int32
	var startOnce sync.Once
	lookupStarted := make(chan struct{})
	releaseLookup := make(chan struct{})
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		startOnce.Do(func() {
			close(lookupStarted)
		})
		<-releaseLookup
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
	leaderErr := make(chan error, 1)
	go func() {
		resp, err := client.Get(context.Background(), "9,abc", blob.GetOptions{})
		if err != nil {
			leaderErr <- err
			return
		}
		resp.Body.Close()
		leaderErr <- nil
	}()

	<-lookupStarted
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Get(ctx, "9,abc", blob.GetOptions{}); err != context.Canceled {
		t.Fatalf("Get() error = %v, want context.Canceled", err)
	}
	close(releaseLookup)
	if err := <-leaderErr; err != nil {
		t.Fatalf("leader Get() error = %v", err)
	}
	if lookups.Load() != 1 {
		t.Fatalf("lookups = %d, want waiting request to share lookup", lookups.Load())
	}
}

func TestHeadAndDeleteInvalidateLookupCacheOnErrors(t *testing.T) {
	t.Parallel()

	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "volume unavailable", http.StatusInternalServerError)
	}))
	defer volumeServer.Close()

	var lookups atomic.Int32
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
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
	if _, err := client.Head(context.Background(), "9,abc"); err == nil {
		t.Fatal("Head() error = nil, want status error")
	}
	if err := client.Delete(context.Background(), "9,abc"); err == nil {
		t.Fatal("Delete() error = nil, want status error")
	}
	if lookups.Load() != 2 {
		t.Fatalf("lookups = %d, want cache invalidated between Head and Delete", lookups.Load())
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

func assertGetBody(t *testing.T, client *blob.Client, fileID string, want string) {
	t.Helper()
	resp, err := client.Get(context.Background(), fileID, blob.GetOptions{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != want {
		t.Fatalf("body = %q, want %s", body, want)
	}
}

func newTestClient(t *testing.T, server *httptest.Server, usePublicURLs bool) *blob.Client {
	t.Helper()
	return newTestClientWithConfig(t, server, blob.Config{
		UsePublicURLs: usePublicURLs,
	})
}

func newTestClientWithConfig(t *testing.T, server *httptest.Server, config blob.Config) *blob.Client {
	t.Helper()
	masterClient, err := master.New(master.Config{
		BaseURLs:   []string{server.URL},
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("master.New() error = %v", err)
	}
	config.Master = masterClient
	if config.HTTPClient == nil {
		config.HTTPClient = server.Client()
	}
	client, err := blob.New(config)
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
