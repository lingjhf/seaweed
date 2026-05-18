package seaweed

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lingjhf/seaweed/blob"
	"github.com/lingjhf/seaweed/filer"
	"github.com/lingjhf/seaweed/internal/httpx"
	"github.com/lingjhf/seaweed/master"
	"github.com/lingjhf/seaweed/tus"
	"github.com/lingjhf/seaweed/volume"
)

const defaultTUSBasePath = "/.tus"

type Client struct {
	config Config
	http   *http.Client

	master *master.Client
	volume *volume.Client
	blob   *blob.Client
	filer  *filer.Client
	tus    *tus.Client
}

func New(config Config, opts ...Option) (*Client, error) {
	applied := options{
		httpClient: NewHTTPClient(DefaultHTTPClientConfig()),
	}
	for _, opt := range opts {
		opt(&applied)
	}
	if applied.httpClient == nil {
		return nil, fmt.Errorf("seaweed: http client is nil")
	}
	if len(config.MasterURLs) == 0 {
		return nil, fmt.Errorf("seaweed: master urls are required")
	}
	if config.TUSBasePath == "" {
		config.TUSBasePath = defaultTUSBasePath
	}
	if config.Retry.MaxAttempts == 0 {
		config.Retry = DefaultRetryPolicy()
	}
	endpointPolicy, err := httpx.NormalizeEndpointPolicy(config.EndpointPolicy)
	if err != nil {
		return nil, fmt.Errorf("seaweed: invalid endpoint policy: %w", err)
	}
	config.EndpointPolicy = endpointPolicy
	if _, err := httpx.NormalizeEndpointPolicy(endpointPolicyOrDefault(config.MasterEndpointPolicy, config.EndpointPolicy)); err != nil {
		return nil, fmt.Errorf("seaweed: invalid master endpoint policy: %w", err)
	}
	if _, err := httpx.NormalizeEndpointPolicy(endpointPolicyOrDefault(config.VolumeEndpointPolicy, config.EndpointPolicy)); err != nil {
		return nil, fmt.Errorf("seaweed: invalid volume endpoint policy: %w", err)
	}
	if _, err := httpx.NormalizeEndpointPolicy(endpointPolicyOrDefault(config.FilerEndpointPolicy, config.EndpointPolicy)); err != nil {
		return nil, fmt.Errorf("seaweed: invalid filer endpoint policy: %w", err)
	}
	if _, err := httpx.NormalizeEndpointPolicy(endpointPolicyOrDefault(config.TUSEndpointPolicy, config.EndpointPolicy)); err != nil {
		return nil, fmt.Errorf("seaweed: invalid tus endpoint policy: %w", err)
	}

	masterURLs, err := httpx.NormalizeBaseURLs(config.MasterURLs)
	if err != nil {
		return nil, fmt.Errorf("seaweed: invalid master urls: %w", err)
	}
	config.MasterURLs = masterURLs
	if len(config.VolumeURLs) > 0 {
		volumeURLs, err := httpx.NormalizeBaseURLs(config.VolumeURLs)
		if err != nil {
			return nil, fmt.Errorf("seaweed: invalid volume urls: %w", err)
		}
		config.VolumeURLs = volumeURLs
	}
	if len(config.FilerURLs) > 0 {
		filerURLs, err := httpx.NormalizeBaseURLs(config.FilerURLs)
		if err != nil {
			return nil, fmt.Errorf("seaweed: invalid filer urls: %w", err)
		}
		config.FilerURLs = filerURLs
	}
	if config.S3URL != "" {
		s3URL, err := httpx.NormalizeBaseURL(config.S3URL)
		if err != nil {
			return nil, fmt.Errorf("seaweed: invalid s3 url: %w", err)
		}
		config.S3URL = s3URL
	}
	if config.IAMURL != "" {
		iamURL, err := httpx.NormalizeBaseURL(config.IAMURL)
		if err != nil {
			return nil, fmt.Errorf("seaweed: invalid iam url: %w", err)
		}
		config.IAMURL = iamURL
	}
	if config.Region == "" {
		config.Region = "us-east-1"
	}

	masterClient, err := master.New(master.Config{
		BaseURLs:       config.MasterURLs,
		HTTPClient:     applied.httpClient,
		UserAgent:      config.UserAgent,
		BearerToken:    config.BearerToken,
		Retry:          config.Retry,
		EndpointPolicy: endpointPolicyOrDefault(config.MasterEndpointPolicy, config.EndpointPolicy),
	})
	if err != nil {
		return nil, err
	}
	client := &Client{
		config: config,
		http:   applied.httpClient,
		master: masterClient,
	}
	if len(config.VolumeURLs) > 0 {
		client.volume, err = volume.New(volume.Config{
			BaseURLs:       config.VolumeURLs,
			HTTPClient:     applied.httpClient,
			UserAgent:      config.UserAgent,
			BearerToken:    config.BearerToken,
			Retry:          config.Retry,
			EndpointPolicy: endpointPolicyOrDefault(config.VolumeEndpointPolicy, config.EndpointPolicy),
		})
		if err != nil {
			return nil, err
		}
	}
	client.blob, err = blob.New(blob.Config{
		Master:        client.master,
		HTTPClient:    applied.httpClient,
		UserAgent:     config.UserAgent,
		BearerToken:   config.BearerToken,
		Retry:         config.Retry,
		UsePublicURLs: config.UsePublicURLs,
	})
	if err != nil {
		return nil, err
	}
	if len(config.FilerURLs) > 0 {
		client.filer, err = filer.New(filer.Config{
			BaseURLs:       config.FilerURLs,
			HTTPClient:     applied.httpClient,
			UserAgent:      config.UserAgent,
			BearerToken:    config.BearerToken,
			Retry:          config.Retry,
			EndpointPolicy: endpointPolicyOrDefault(config.FilerEndpointPolicy, config.EndpointPolicy),
		})
		if err != nil {
			return nil, err
		}
		client.tus, err = tus.New(tus.Config{
			FilerURLs:      config.FilerURLs,
			BasePath:       config.TUSBasePath,
			HTTPClient:     applied.httpClient,
			UserAgent:      config.UserAgent,
			BearerToken:    config.BearerToken,
			Retry:          config.Retry,
			ContentType:    "application/offset+octet-stream",
			EndpointPolicy: endpointPolicyOrDefault(config.TUSEndpointPolicy, config.EndpointPolicy),
		})
		if err != nil {
			return nil, err
		}
	}
	return client, nil
}

func (c *Client) Config() Config {
	return c.config
}

func (c *Client) Master() *master.Client {
	return c.master
}

func (c *Client) Volume() *volume.Client {
	return c.volume
}

func (c *Client) Blob() *blob.Client {
	return c.blob
}

func (c *Client) Filer() *filer.Client {
	return c.filer
}

func (c *Client) TUS() *tus.Client {
	return c.tus
}

func (c *Client) Close() {
	if c.master != nil {
		c.master.Close()
	}
	if c.volume != nil {
		c.volume.Close()
	}
	if c.filer != nil {
		c.filer.Close()
	}
	if c.tus != nil {
		c.tus.Close()
	}
}

func (c *Client) S3(ctx context.Context) (*s3.Client, error) {
	if c.config.S3URL == "" {
		return nil, fmt.Errorf("seaweed: s3 url is required")
	}
	cfg, err := c.awsConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(c.config.S3URL)
		o.UsePathStyle = true
	}), nil
}

func (c *Client) IAM(ctx context.Context) (*iam.Client, error) {
	endpoint := c.config.IAMURL
	if endpoint == "" {
		endpoint = c.config.S3URL
	}
	if endpoint == "" {
		return nil, fmt.Errorf("seaweed: iam url or s3 url is required")
	}
	cfg, err := c.awsConfig(ctx)
	if err != nil {
		return nil, err
	}
	return iam.NewFromConfig(cfg, func(o *iam.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	}), nil
}

func (c *Client) awsConfig(ctx context.Context) (aws.Config, error) {
	if c.config.AccessKeyID == "" || c.config.SecretAccessKey == "" {
		return aws.Config{}, fmt.Errorf("seaweed: access key id and secret access key are required")
	}
	return awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(c.config.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			c.config.AccessKeyID,
			c.config.SecretAccessKey,
			"",
		)),
		awsconfig.WithHTTPClient(c.http),
	)
}

func endpointPolicyOrDefault(policy EndpointPolicy, fallback EndpointPolicy) EndpointPolicy {
	if policy.Mode == "" && !policy.HealthCheck.Enabled && !policy.CircuitBreaker.Enabled {
		return fallback
	}
	return policy
}
