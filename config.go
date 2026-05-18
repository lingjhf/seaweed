package seaweed

import (
	"net"
	"net/http"
	"time"

	"github.com/lingjhf/seaweed/internal/httpx"
)

type RetryPolicy = httpx.RetryPolicy
type EndpointPolicy = httpx.EndpointPolicy
type EndpointPolicyMode = httpx.EndpointPolicyMode
type EndpointHealthCheckPolicy = httpx.EndpointHealthCheckPolicy
type EndpointCircuitBreakerPolicy = httpx.EndpointCircuitBreakerPolicy

const (
	EndpointPolicyFailover   = httpx.EndpointPolicyFailover
	EndpointPolicyRoundRobin = httpx.EndpointPolicyRoundRobin
)

type Config struct {
	MasterURLs           []string
	VolumeURLs           []string
	FilerURLs            []string
	TUSBasePath          string
	S3URLs               []string
	IAMURLs              []string
	Region               string
	AccessKeyID          string
	SecretAccessKey      string
	BearerToken          string
	UserAgent            string
	UsePublicURLs        bool
	Retry                RetryPolicy
	BlobLocationCacheTTL time.Duration

	EndpointPolicy       EndpointPolicy
	MasterEndpointPolicy EndpointPolicy
	VolumeEndpointPolicy EndpointPolicy
	BlobEndpointPolicy   EndpointPolicy
	FilerEndpointPolicy  EndpointPolicy
	TUSEndpointPolicy    EndpointPolicy
}

type Option func(*options)

type options struct {
	httpClient *http.Client
}

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

func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		o.httpClient = client
	}
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		Wait:        100 * time.Millisecond,
	}
}

func DefaultEndpointPolicy() EndpointPolicy {
	return EndpointPolicy{
		Mode: EndpointPolicyFailover,
	}
}

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
