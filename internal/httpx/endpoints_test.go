package httpx_test

import (
	"testing"

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
