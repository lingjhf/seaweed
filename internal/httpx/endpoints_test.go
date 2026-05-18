package httpx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lingjhf/seaweed/internal/httpx"
)

func TestNormalizeBaseURLs(t *testing.T) {
	t.Parallel()

	urls, err := httpx.NormalizeBaseURLs([]string{
		"http://127.0.0.1:9333/",
		"http://127.0.0.1:9333?q=ignored#fragment",
		"http://127.0.0.1:8888/filer/",
	})
	if err != nil {
		t.Fatalf("NormalizeBaseURLs() error = %v", err)
	}
	want := []string{
		"http://127.0.0.1:9333",
		"http://127.0.0.1:8888/filer",
	}
	if len(urls) != len(want) {
		t.Fatalf("urls len = %d, want %d: %#v", len(urls), len(want), urls)
	}
	for i := range want {
		if urls[i] != want[i] {
			t.Fatalf("urls[%d] = %q, want %q", i, urls[i], want[i])
		}
	}
}

func TestNormalizeBaseURLsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		urls []string
	}{
		{name: "empty"},
		{name: "relative", urls: []string{"127.0.0.1:9333"}},
		{name: "blank", urls: []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := httpx.NormalizeBaseURLs(tt.urls); err == nil {
				t.Fatal("NormalizeBaseURLs() error = nil, want error")
			}
		})
	}
}

func TestEndpointSetURLUsesActiveEndpoint(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSet([]string{"http://example.test/base/"})
	if err != nil {
		t.Fatalf("NewEndpointSet() error = %v", err)
	}
	if got := endpoints.URL("/status"); got != "http://example.test/base/status" {
		t.Fatalf("URL() = %q", got)
	}
}

func TestEndpointSetURLsReturnsCopy(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSet([]string{
		"http://one.example.test",
		"http://two.example.test",
	})
	if err != nil {
		t.Fatalf("NewEndpointSet() error = %v", err)
	}
	urls := endpoints.URLs()
	urls[0] = "http://mutated.example.test"

	got := endpoints.URLs()
	if got[0] != "http://one.example.test" {
		t.Fatalf("URLs()[0] = %q, want original endpoint", got[0])
	}
}

func TestEndpointSetCandidatesFailoverUsesActiveEndpoint(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSet([]string{
		"http://one.example.test",
		"http://two.example.test",
	})
	if err != nil {
		t.Fatalf("NewEndpointSet() error = %v", err)
	}
	endpoints.MarkSuccess(1)

	candidates := endpoints.Candidates("/status")
	got := []string{candidates[0].URL, candidates[1].URL}
	want := []string{
		"http://two.example.test/status",
		"http://one.example.test/status",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidates[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEndpointSetCandidatesRoundRobin(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSetWithPolicy([]string{
		"http://one.example.test",
		"http://two.example.test",
		"http://three.example.test",
	}, httpx.EndpointPolicy{Mode: httpx.EndpointPolicyRoundRobin})
	if err != nil {
		t.Fatalf("NewEndpointSetWithPolicy() error = %v", err)
	}

	got := []string{
		endpoints.Candidates("/status")[0].URL,
		endpoints.Candidates("/status")[0].URL,
		endpoints.Candidates("/status")[0].URL,
		endpoints.Candidates("/status")[0].URL,
	}
	want := []string{
		"http://one.example.test/status",
		"http://two.example.test/status",
		"http://three.example.test/status",
		"http://one.example.test/status",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("round robin candidate %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNormalizeEndpointPolicy(t *testing.T) {
	t.Parallel()

	policy, err := httpx.NormalizeEndpointPolicy(httpx.EndpointPolicy{})
	if err != nil {
		t.Fatalf("NormalizeEndpointPolicy() error = %v", err)
	}
	if policy.Mode != httpx.EndpointPolicyFailover {
		t.Fatalf("Mode = %q, want failover", policy.Mode)
	}

	if _, err := httpx.NormalizeEndpointPolicy(httpx.EndpointPolicy{Mode: "random"}); err == nil {
		t.Fatal("NormalizeEndpointPolicy() error = nil, want unsupported mode error")
	}
}

func TestNormalizeEndpointPolicyDefaultsHealthAndCircuitBreaker(t *testing.T) {
	t.Parallel()

	policy, err := httpx.NormalizeEndpointPolicy(httpx.EndpointPolicy{
		HealthCheck: httpx.EndpointHealthCheckPolicy{
			Enabled: true,
		},
		CircuitBreaker: httpx.EndpointCircuitBreakerPolicy{
			Enabled: true,
		},
	})
	if err != nil {
		t.Fatalf("NormalizeEndpointPolicy() error = %v", err)
	}
	if policy.HealthCheck.Interval == 0 || policy.HealthCheck.Timeout == 0 {
		t.Fatalf("health check defaults were not applied: %+v", policy.HealthCheck)
	}
	if policy.HealthCheck.FailureThreshold != 1 || policy.HealthCheck.SuccessThreshold != 1 {
		t.Fatalf("health thresholds = %+v", policy.HealthCheck)
	}
	if policy.CircuitBreaker.FailureThreshold != 3 || policy.CircuitBreaker.OpenTimeout == 0 || policy.CircuitBreaker.HalfOpenMaxRequests != 1 {
		t.Fatalf("circuit breaker defaults were not applied: %+v", policy.CircuitBreaker)
	}
}

func TestEndpointSetHealthCheckMarksAndRestoresEndpoint(t *testing.T) {
	t.Parallel()

	var status atomic.Int32
	status.Store(http.StatusServiceUnavailable)
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(status.Load()))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer second.Close()

	endpoints, err := httpx.NewEndpointSetWithPolicy([]string{first.URL, second.URL}, httpx.EndpointPolicy{
		HealthCheck: httpx.EndpointHealthCheckPolicy{
			Enabled:          true,
			Interval:         time.Millisecond,
			Timeout:          100 * time.Millisecond,
			FailureThreshold: 1,
			SuccessThreshold: 1,
		},
	})
	if err != nil {
		t.Fatalf("NewEndpointSetWithPolicy() error = %v", err)
	}
	endpoints.StartHealthCheck(first.Client(), http.MethodGet, "/health")
	defer endpoints.Close()

	waitFor(t, func() bool {
		candidates := endpoints.Candidates("/status")
		return len(candidates) == 1 && candidates[0].URL == second.URL+"/status"
	})
	status.Store(http.StatusNoContent)
	waitFor(t, func() bool {
		candidates := endpoints.Candidates("/status")
		if len(candidates) != 2 {
			return false
		}
		for _, candidate := range candidates {
			if candidate.URL == first.URL+"/status" {
				return true
			}
		}
		return false
	})
}

func TestEndpointSetCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSetWithPolicy([]string{"http://example.test"}, httpx.EndpointPolicy{
		HealthCheck: httpx.EndpointHealthCheckPolicy{
			Enabled:  true,
			Interval: time.Hour,
			Timeout:  time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("NewEndpointSetWithPolicy() error = %v", err)
	}
	endpoints.StartHealthCheck(http.DefaultClient, http.MethodGet, "/")
	endpoints.Close()
	endpoints.Close()
}

func TestEndpointSetCircuitBreakerLimitsHalfOpenRequests(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(release) })

	endpoints, err := httpx.NewEndpointSetWithPolicy([]string{server.URL}, httpx.EndpointPolicy{
		CircuitBreaker: httpx.EndpointCircuitBreakerPolicy{
			Enabled:             true,
			FailureThreshold:    1,
			OpenTimeout:         time.Millisecond,
			HalfOpenMaxRequests: 1,
		},
	})
	if err != nil {
		t.Fatalf("NewEndpointSetWithPolicy() error = %v", err)
	}
	endpoints.RecordFailure(0)
	time.Sleep(5 * time.Millisecond)

	client := httpx.NewClient(httpx.Config{HTTPClient: server.Client()})
	firstErr := make(chan error, 1)
	go func() {
		resp, err := client.DoEndpoint(context.Background(), endpoints, "/health", httpx.Request{
			Method: http.MethodGet,
		})
		if resp != nil {
			resp.Body.Close()
		}
		firstErr <- err
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first half-open request did not reach server")
	}

	resp, err := client.DoEndpoint(context.Background(), endpoints, "/health", httpx.Request{
		Method: http.MethodGet,
	})
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "no available endpoints") {
		t.Fatalf("second DoEndpoint() error = %v, want no available endpoints", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("server calls = %d, want only one half-open request", calls.Load())
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-firstErr:
		if err != nil {
			t.Fatalf("first DoEndpoint() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first half-open request did not finish")
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before deadline")
}
