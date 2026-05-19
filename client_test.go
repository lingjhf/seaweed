package seaweed_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lingjhf/seaweed"
)

func TestNewRequiresMasterURLs(t *testing.T) {
	t.Parallel()

	_, err := seaweed.New(seaweed.Config{})
	if err == nil {
		t.Fatal("New() error = nil, want error")
	}
}

func TestNewNormalizesMasterURLs(t *testing.T) {
	t.Parallel()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{"http://127.0.0.1:9333/"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := client.Config().MasterURLs; len(got) != 1 || got[0] != "http://127.0.0.1:9333" {
		t.Fatalf("MasterURLs = %#v", got)
	}
	if client.Config().TUSBasePath != "/.tus" {
		t.Fatalf("TUSBasePath = %q", client.Config().TUSBasePath)
	}
}

func TestNewHTTPClientUsesTunedTransport(t *testing.T) {
	t.Parallel()

	client := seaweed.NewHTTPClient(seaweed.DefaultHTTPClientConfig())
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", client.Transport)
	}
	if transport.MaxIdleConns != 256 {
		t.Fatalf("MaxIdleConns = %d, want 256", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 128 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 128", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Fatalf("IdleConnTimeout = %s, want 90s", transport.IdleConnTimeout)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 = false, want true")
	}
}

func TestNewHTTPClientZeroConfigUsesDefaults(t *testing.T) {
	t.Parallel()

	client := seaweed.NewHTTPClient(seaweed.HTTPClientConfig{})
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", client.Transport)
	}
	if transport.MaxIdleConns != 256 {
		t.Fatalf("MaxIdleConns = %d, want 256", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 128 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 128", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Fatalf("IdleConnTimeout = %s, want 90s", transport.IdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != 10*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %s, want 10s", transport.TLSHandshakeTimeout)
	}
	if transport.ExpectContinueTimeout != time.Second {
		t.Fatalf("ExpectContinueTimeout = %s, want 1s", transport.ExpectContinueTimeout)
	}
	if client.Timeout != 0 {
		t.Fatalf("Timeout = %s, want 0", client.Timeout)
	}
}

func TestNewHTTPClientUsesOverrides(t *testing.T) {
	t.Parallel()

	client := seaweed.NewHTTPClient(seaweed.HTTPClientConfig{
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       11 * time.Second,
		DialTimeout:           12 * time.Second,
		KeepAlive:             13 * time.Second,
		TLSHandshakeTimeout:   14 * time.Second,
		ExpectContinueTimeout: 15 * time.Second,
		ResponseHeaderTimeout: 16 * time.Second,
		Timeout:               17 * time.Second,
	})
	transport := client.Transport.(*http.Transport)
	if transport.MaxIdleConns != 10 || transport.MaxIdleConnsPerHost != 8 {
		t.Fatalf("idle conn settings = %d/%d, want 10/8", transport.MaxIdleConns, transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 11*time.Second {
		t.Fatalf("IdleConnTimeout = %s, want 11s", transport.IdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != 14*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %s, want 14s", transport.TLSHandshakeTimeout)
	}
	if transport.ExpectContinueTimeout != 15*time.Second {
		t.Fatalf("ExpectContinueTimeout = %s, want 15s", transport.ExpectContinueTimeout)
	}
	if transport.ResponseHeaderTimeout != 16*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %s, want 16s", transport.ResponseHeaderTimeout)
	}
	if client.Timeout != 17*time.Second {
		t.Fatalf("Timeout = %s, want 17s", client.Timeout)
	}
}

func TestNewNormalizesConfiguredURLsAndAccessors(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{}
	client, err := seaweed.New(seaweed.Config{
		MasterURLs:                []string{"http://127.0.0.1:9333/master/?q=1#fragment"},
		VolumeURLs:                []string{"http://127.0.0.1:8080/volume/"},
		FilerURLs:                 []string{"http://127.0.0.1:8888/filer/"},
		TUSBasePath:               "uploads",
		S3URLs:                    []string{"http://127.0.0.1:8333/s3/"},
		IAMURLs:                   []string{"http://127.0.0.1:8333/iam/"},
		AccessKeyID:               "access",
		SecretAccessKey:           "secret",
		UserAgent:                 "seaweed-test",
		BearerToken:               "token",
		UsePublicURLs:             true,
		BlobLocationCacheTTL:      10 * time.Second,
		EnableVolumeAuthorization: true,
		BlobEndpointPolicy: seaweed.EndpointPolicy{
			Mode: seaweed.EndpointPolicyRoundRobin,
		},
		S3EndpointPolicy: seaweed.EndpointPolicy{
			Mode: seaweed.EndpointPolicyRoundRobin,
		},
		IAMEndpointPolicy: seaweed.EndpointPolicy{
			Mode: seaweed.EndpointPolicyRoundRobin,
		},
	}, seaweed.WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	config := client.Config()
	if len(config.MasterURLs) != 1 || config.MasterURLs[0] != "http://127.0.0.1:9333/master" {
		t.Fatalf("MasterURLs = %#v", config.MasterURLs)
	}
	if len(config.VolumeURLs) != 1 || config.VolumeURLs[0] != "http://127.0.0.1:8080/volume" {
		t.Fatalf("VolumeURLs = %#v", config.VolumeURLs)
	}
	if len(config.FilerURLs) != 1 || config.FilerURLs[0] != "http://127.0.0.1:8888/filer" {
		t.Fatalf("FilerURLs = %#v", config.FilerURLs)
	}
	if len(config.S3URLs) != 1 || config.S3URLs[0] != "http://127.0.0.1:8333/s3" {
		t.Fatalf("S3URLs = %#v", config.S3URLs)
	}
	if len(config.IAMURLs) != 1 || config.IAMURLs[0] != "http://127.0.0.1:8333/iam" {
		t.Fatalf("IAMURLs = %#v", config.IAMURLs)
	}
	if config.Region != "us-east-1" {
		t.Fatalf("Region = %q, want us-east-1", config.Region)
	}
	if config.Retry.MaxAttempts != 3 {
		t.Fatalf("Retry.MaxAttempts = %d, want 3", config.Retry.MaxAttempts)
	}
	if config.BlobLocationCacheTTL != 10*time.Second {
		t.Fatalf("BlobLocationCacheTTL = %s, want 10s", config.BlobLocationCacheTTL)
	}
	if !config.EnableVolumeAuthorization {
		t.Fatal("EnableVolumeAuthorization = false, want true")
	}
	if config.BlobEndpointPolicy.Mode != seaweed.EndpointPolicyRoundRobin {
		t.Fatalf("BlobEndpointPolicy.Mode = %q, want round-robin", config.BlobEndpointPolicy.Mode)
	}
	if config.S3EndpointPolicy.Mode != seaweed.EndpointPolicyRoundRobin {
		t.Fatalf("S3EndpointPolicy.Mode = %q, want round-robin", config.S3EndpointPolicy.Mode)
	}
	if config.IAMEndpointPolicy.Mode != seaweed.EndpointPolicyRoundRobin {
		t.Fatalf("IAMEndpointPolicy.Mode = %q, want round-robin", config.IAMEndpointPolicy.Mode)
	}
	if client.Master() == nil || client.Volume() == nil || client.Blob() == nil || client.Filer() == nil || client.TUS() == nil {
		t.Fatal("client accessors returned nil")
	}
	if s3Client, err := client.S3(context.Background()); err != nil || s3Client == nil {
		t.Fatalf("S3() = %v, %v; want client", s3Client, err)
	}
	if iamClient, err := client.IAM(context.Background()); err != nil || iamClient == nil {
		t.Fatalf("IAM() = %v, %v; want client", iamClient, err)
	}
}

func TestNewPropagatesRoundRobinEndpointPolicy(t *testing.T) {
	t.Parallel()

	var firstCalls int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&firstCalls, 1)
		if r.Method != http.MethodHead || r.URL.Path != "/cluster/healthz" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer first.Close()
	var secondCalls int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&secondCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer second.Close()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{first.URL, second.URL},
		EndpointPolicy: seaweed.EndpointPolicy{
			Mode: seaweed.EndpointPolicyRoundRobin,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for range 4 {
		if err := client.Master().Health(context.Background()); err != nil {
			t.Fatalf("Health() error = %v", err)
		}
	}
	if firstCalls != 2 || secondCalls != 2 {
		t.Fatalf("master health calls = %d/%d, want 2/2", firstCalls, secondCalls)
	}
}

func TestNewUsesServiceEndpointPolicyOverride(t *testing.T) {
	t.Parallel()

	var firstCalls int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&firstCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer first.Close()
	var secondCalls int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&secondCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer second.Close()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs:     []string{first.URL, second.URL},
		EndpointPolicy: seaweed.DefaultEndpointPolicy(),
		MasterEndpointPolicy: seaweed.EndpointPolicy{
			Mode: seaweed.EndpointPolicyRoundRobin,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for range 2 {
		if err := client.Master().Health(context.Background()); err != nil {
			t.Fatalf("Health() error = %v", err)
		}
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("master calls = %d/%d, want service override round robin", firstCalls, secondCalls)
	}
}

func TestClientClose(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs:      []string{server.URL},
		VolumeURLs:      []string{server.URL},
		FilerURLs:       []string{server.URL},
		S3URLs:          []string{server.URL},
		IAMURLs:         []string{server.URL},
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	client.Close()
	client.Close()
}

func TestNewRejectsNilHTTPClient(t *testing.T) {
	t.Parallel()

	_, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{"http://127.0.0.1:9333"},
	}, seaweed.WithHTTPClient(nil))
	if err == nil {
		t.Fatal("New() error = nil, want nil http client error")
	}
}

func TestNewRejectsInvalidURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config seaweed.Config
	}{
		{
			name: "master",
			config: seaweed.Config{
				MasterURLs: []string{"127.0.0.1:9333"},
			},
		},
		{
			name: "volume",
			config: seaweed.Config{
				MasterURLs: []string{"http://127.0.0.1:9333"},
				VolumeURLs: []string{"127.0.0.1:8080"},
			},
		},
		{
			name: "filer",
			config: seaweed.Config{
				MasterURLs: []string{"http://127.0.0.1:9333"},
				FilerURLs:  []string{"127.0.0.1:8888"},
			},
		},
		{
			name: "s3",
			config: seaweed.Config{
				MasterURLs: []string{"http://127.0.0.1:9333"},
				S3URLs:     []string{"127.0.0.1:8333"},
			},
		},
		{
			name: "iam",
			config: seaweed.Config{
				MasterURLs: []string{"http://127.0.0.1:9333"},
				IAMURLs:    []string{"127.0.0.1:8333"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := seaweed.New(tt.config); err == nil {
				t.Fatal("New() error = nil, want invalid url error")
			}
		})
	}
}

func TestNewRejectsInvalidEndpointPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config seaweed.Config
	}{
		{
			name: "global",
			config: seaweed.Config{
				MasterURLs: []string{"http://127.0.0.1:9333"},
				EndpointPolicy: seaweed.EndpointPolicy{
					Mode: "random",
				},
			},
		},
		{
			name: "blob",
			config: seaweed.Config{
				MasterURLs: []string{"http://127.0.0.1:9333"},
				BlobEndpointPolicy: seaweed.EndpointPolicy{
					Mode: "random",
				},
			},
		},
		{
			name: "s3",
			config: seaweed.Config{
				MasterURLs: []string{"http://127.0.0.1:9333"},
				S3EndpointPolicy: seaweed.EndpointPolicy{
					Mode: "random",
				},
			},
		},
		{
			name: "iam",
			config: seaweed.Config{
				MasterURLs: []string{"http://127.0.0.1:9333"},
				IAMEndpointPolicy: seaweed.EndpointPolicy{
					Mode: "random",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := seaweed.New(tt.config); err == nil {
				t.Fatal("New() error = nil, want invalid endpoint policy error")
			}
		})
	}
}

func TestIAMFallsBackToS3Endpoint(t *testing.T) {
	t.Parallel()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs:      []string{"http://127.0.0.1:9333"},
		S3URLs:          []string{"http://127.0.0.1:8333"},
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	iamClient, err := client.IAM(context.Background())
	if err != nil {
		t.Fatalf("IAM() error = %v", err)
	}
	if iamClient == nil {
		t.Fatal("IAM() = nil, want client")
	}
}
