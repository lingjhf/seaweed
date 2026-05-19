package seaweed

import (
	"net"
	"net/http"
	"time"

	"github.com/lingjhf/seaweed/internal/httpx"
)

// RetryPolicy controls retry attempts for retryable HTTP methods.
type RetryPolicy = httpx.RetryPolicy

// EndpointPolicy controls how clients choose among multiple service endpoints.
type EndpointPolicy = httpx.EndpointPolicy

// EndpointPolicyMode selects the endpoint selection strategy.
type EndpointPolicyMode = httpx.EndpointPolicyMode

// EndpointHealthCheckPolicy configures background endpoint health probes.
type EndpointHealthCheckPolicy = httpx.EndpointHealthCheckPolicy

// EndpointCircuitBreakerPolicy configures endpoint failure isolation.
type EndpointCircuitBreakerPolicy = httpx.EndpointCircuitBreakerPolicy

const (
	// EndpointPolicyFailover keeps using the active endpoint until it fails.
	EndpointPolicyFailover = httpx.EndpointPolicyFailover
	// EndpointPolicyRoundRobin rotates the starting endpoint for retryable requests.
	EndpointPolicyRoundRobin = httpx.EndpointPolicyRoundRobin
)

// Config configures a root SeaweedFS client.
type Config struct {
	// MasterURLs are the SeaweedFS master HTTP endpoints. At least one is required.
	MasterURLs []string
	// VolumeURLs are optional direct volume server HTTP endpoints.
	VolumeURLs []string
	// FilerURLs are optional filer HTTP endpoints used by the filer and TUS clients.
	FilerURLs []string
	// TUSBasePath is the filer TUS base path. The default is "/.tus".
	TUSBasePath string
	// S3URLs are optional SeaweedFS S3 gateway endpoints.
	S3URLs []string
	// IAMURLs are optional SeaweedFS IAM endpoints. When empty, IAM uses S3URLs.
	IAMURLs []string
	// Region is the AWS signing region for S3 and IAM clients. The default is "us-east-1".
	Region string
	// AccessKeyID is the S3/IAM access key.
	AccessKeyID string
	// SecretAccessKey is the S3/IAM secret key.
	SecretAccessKey string
	// BearerToken is sent as an Authorization bearer token on native HTTP APIs.
	BearerToken string
	// UserAgent overrides the User-Agent sent by native HTTP API clients.
	UserAgent string
	// UsePublicURLs makes the blob client prefer public volume URLs returned by master.
	UsePublicURLs bool
	// Retry controls retries for retryable native HTTP API methods.
	Retry RetryPolicy
	// BlobLocationCacheTTL limits how long blob volume lookups are cached.
	BlobLocationCacheTTL time.Duration
	// EnableVolumeAuthorization makes Blob use master-issued per-file
	// Authorization headers for volume reads, writes, and deletes.
	EnableVolumeAuthorization bool

	// EndpointPolicy is the default policy for all endpoint lists.
	EndpointPolicy EndpointPolicy
	// MasterEndpointPolicy overrides EndpointPolicy for the master client.
	MasterEndpointPolicy EndpointPolicy
	// VolumeEndpointPolicy overrides EndpointPolicy for the direct volume client.
	VolumeEndpointPolicy EndpointPolicy
	// BlobEndpointPolicy overrides EndpointPolicy for blob volume reads.
	BlobEndpointPolicy EndpointPolicy
	// FilerEndpointPolicy overrides EndpointPolicy for the filer client.
	FilerEndpointPolicy EndpointPolicy
	// TUSEndpointPolicy overrides EndpointPolicy for the TUS client.
	TUSEndpointPolicy EndpointPolicy
	// S3EndpointPolicy overrides EndpointPolicy for the S3 client.
	S3EndpointPolicy EndpointPolicy
	// IAMEndpointPolicy overrides EndpointPolicy for the IAM client.
	IAMEndpointPolicy EndpointPolicy
}

// Option customizes root client construction.
type Option func(*options)

type options struct {
	httpClient *http.Client
}

// HTTPClientConfig configures the HTTP client created by NewHTTPClient.
type HTTPClientConfig struct {
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	IdleConnTimeout       time.Duration
	DialTimeout           time.Duration
	KeepAlive             time.Duration
	TLSHandshakeTimeout   time.Duration
	ExpectContinueTimeout time.Duration
	ResponseHeaderTimeout time.Duration
	Timeout               time.Duration
}

// WithHTTPClient makes New use client for all native and AWS SDK requests.
func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		o.httpClient = client
	}
}

// DefaultRetryPolicy returns the default retry policy used by New.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		Wait:        100 * time.Millisecond,
	}
}

// DefaultEndpointPolicy returns the default failover endpoint policy.
func DefaultEndpointPolicy() EndpointPolicy {
	return EndpointPolicy{
		Mode: EndpointPolicyFailover,
	}
}

// DefaultHTTPClientConfig returns the default transport settings for NewHTTPClient.
func DefaultHTTPClientConfig() HTTPClientConfig {
	return HTTPClientConfig{
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   128,
		IdleConnTimeout:       90 * time.Second,
		DialTimeout:           30 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// NewHTTPClient builds an HTTP client tuned for SeaweedFS SDK workloads.
func NewHTTPClient(config HTTPClientConfig) *http.Client {
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 256
	}
	if config.MaxIdleConnsPerHost == 0 {
		config.MaxIdleConnsPerHost = 128
	}
	if config.IdleConnTimeout == 0 {
		config.IdleConnTimeout = 90 * time.Second
	}
	if config.DialTimeout == 0 {
		config.DialTimeout = 30 * time.Second
	}
	if config.KeepAlive == 0 {
		config.KeepAlive = 30 * time.Second
	}
	if config.TLSHandshakeTimeout == 0 {
		config.TLSHandshakeTimeout = 10 * time.Second
	}
	if config.ExpectContinueTimeout == 0 {
		config.ExpectContinueTimeout = time.Second
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: config.DialTimeout, KeepAlive: config.KeepAlive}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ExpectContinueTimeout: config.ExpectContinueTimeout,
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}
}
