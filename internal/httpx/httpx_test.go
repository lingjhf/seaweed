package httpx_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lingjhf/seaweed/internal/httpx"
)

func TestDecodeJSONReturnsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := httpx.NewClient(httpx.Config{HTTPClient: server.Client()})
	err := client.DecodeJSON(context.Background(), httpx.Request{
		Method: http.MethodGet,
		URL:    server.URL,
	}, nil)
	if err == nil {
		t.Fatal("DecodeJSON() error = nil, want error")
	}

	var httpErr *httpx.Error
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want *httpx.Error", err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", httpErr.StatusCode)
	}
}

func TestDoSetsHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "seaweed-test" {
			t.Fatalf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := httpx.NewClient(httpx.Config{
		HTTPClient:  server.Client(),
		UserAgent:   "seaweed-test",
		BearerToken: "token",
	})
	resp, err := client.Do(context.Background(), httpx.Request{
		Method: http.MethodGet,
		URL:    server.URL,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}
