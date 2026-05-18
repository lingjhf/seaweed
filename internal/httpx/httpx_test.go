package httpx_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func TestErrorString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *httpx.Error
		want string
	}{
		{
			name: "empty body",
			err: &httpx.Error{
				Method:     http.MethodGet,
				URL:        "http://example.test/file",
				StatusCode: http.StatusNotFound,
			},
			want: "GET http://example.test/file: unexpected status 404",
		},
		{
			name: "with body",
			err: &httpx.Error{
				Method:     http.MethodPost,
				URL:        "http://example.test/file",
				StatusCode: http.StatusConflict,
				Body:       "already exists",
			},
			want: "POST http://example.test/file: unexpected status 409: already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDoValidation(t *testing.T) {
	t.Parallel()

	client := httpx.NewClient(httpx.Config{HTTPClient: http.DefaultClient})
	tests := []struct {
		name    string
		request httpx.Request
		want    string
	}{
		{
			name:    "missing method",
			request: httpx.Request{URL: "http://example.test"},
			want:    "httpx: method is required",
		},
		{
			name:    "missing url",
			request: httpx.Request{Method: http.MethodGet},
			want:    "httpx: url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Do(context.Background(), tt.request)
			if err == nil {
				t.Fatal("Do() error = nil, want error")
			}
			if err.Error() != tt.want {
				t.Fatalf("Do() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestDoAddsQueryAndRequestHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "existing=true&limit=10" {
			t.Fatalf("RawQuery = %q, want existing=true&limit=10", r.URL.RawQuery)
		}
		if r.Header.Get("X-Test") != "value" {
			t.Fatalf("X-Test = %q, want value", r.Header.Get("X-Test"))
		}
		if r.ContentLength != 4 {
			t.Fatalf("ContentLength = %d, want 4", r.ContentLength)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "body" {
			t.Fatalf("body = %q, want body", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := httpx.NewClient(httpx.Config{HTTPClient: server.Client()})
	resp, err := client.Do(context.Background(), httpx.Request{
		Method: http.MethodPut,
		URL:    server.URL + "?existing=true",
		Query: url.Values{
			"limit": []string{"10"},
		},
		Header: http.Header{
			"X-Test": []string{"value"},
		},
		Body:          strings.NewReader("body"),
		ContentLength: 4,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestDoRetriesRetryableResponses(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := httpx.NewClient(httpx.Config{
		HTTPClient: server.Client(),
		Retry: httpx.RetryPolicy{
			MaxAttempts: 2,
			Wait:        time.Nanosecond,
		},
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
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestDoDoesNotRetryNonRetryableMethod(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := httpx.NewClient(httpx.Config{
		HTTPClient: server.Client(),
		Retry: httpx.RetryPolicy{
			MaxAttempts: 3,
			Wait:        time.Nanosecond,
		},
	})
	resp, err := client.Do(context.Background(), httpx.Request{
		Method: http.MethodPost,
		URL:    server.URL,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		out     any
		wantErr bool
	}{
		{
			name: "nil output ignores body",
			body: "",
			out:  nil,
		},
		{
			name:    "invalid json",
			body:    "{",
			out:     &map[string]any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := httpx.NewClient(httpx.Config{HTTPClient: server.Client()})
			err := client.DecodeJSON(context.Background(), httpx.Request{
				Method: http.MethodGet,
				URL:    server.URL,
			}, tt.out)
			if tt.wantErr && err == nil {
				t.Fatal("DecodeJSON() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("DecodeJSON() error = %v", err)
			}
		})
	}
}

func TestCheckStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		expected   []int
		wantErr    bool
	}{
		{
			name:       "expected status",
			statusCode: http.StatusAccepted,
			expected:   []int{http.StatusOK, http.StatusAccepted},
		},
		{
			name:       "unexpected status",
			statusCode: http.StatusConflict,
			expected:   []int{http.StatusOK},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "status body", tt.statusCode)
			}))
			defer server.Close()

			client := httpx.NewClient(httpx.Config{HTTPClient: server.Client()})
			err := client.CheckStatus(context.Background(), httpx.Request{
				Method: http.MethodDelete,
				URL:    server.URL,
			}, tt.expected...)
			if tt.wantErr {
				var httpErr *httpx.Error
				if !errors.As(err, &httpErr) {
					t.Fatalf("CheckStatus() error = %T, want *httpx.Error", err)
				}
				if httpErr.Body != "status body" {
					t.Fatalf("Body = %q, want status body", httpErr.Body)
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckStatus() error = %v", err)
			}
		})
	}
}

func TestIsHTTPStatus(t *testing.T) {
	t.Parallel()

	err := &httpx.Error{StatusCode: http.StatusNotFound}
	if !httpx.IsHTTPStatus(err, 400, 499) {
		t.Fatal("IsHTTPStatus() = false, want true")
	}
	if httpx.IsHTTPStatus(err, 500, 599) {
		t.Fatal("IsHTTPStatus() = true, want false")
	}
	if httpx.IsHTTPStatus(errors.New("plain error"), 400, 499) {
		t.Fatal("IsHTTPStatus() with plain error = true, want false")
	}
}

func TestAddQueryValues(t *testing.T) {
	t.Parallel()

	query := url.Values{}
	httpx.AddInt(query, "int", 3)
	httpx.AddInt(query, "zero-int", 0)
	httpx.AddInt64(query, "int64", 4)
	httpx.AddInt64(query, "zero-int64", 0)
	httpx.AddFloat64(query, "float64", 1.25)
	httpx.AddFloat64(query, "zero-float64", 0)
	httpx.AddString(query, "string", "value")
	httpx.AddString(query, "empty-string", "")

	want := "float64=1.25&int=3&int64=4&string=value"
	if got := query.Encode(); got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}
