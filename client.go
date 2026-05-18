package seaweed

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lingjhf/seaweed/blob"
	"github.com/lingjhf/seaweed/filer"
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
	if config.MasterURL == "" {
		return nil, fmt.Errorf("seaweed: master url is required")
	}
	if config.TUSBasePath == "" {
		config.TUSBasePath = defaultTUSBasePath
	}
	if config.Retry.MaxAttempts == 0 {
		config.Retry = DefaultRetryPolicy()
	}

	masterURL, err := normalizeBaseURL(config.MasterURL)
	if err != nil {
		return nil, fmt.Errorf("seaweed: invalid master url: %w", err)
	}
	config.MasterURL = masterURL
	if config.VolumeURL != "" {
		volumeURL, err := normalizeBaseURL(config.VolumeURL)
		if err != nil {
			return nil, fmt.Errorf("seaweed: invalid volume url: %w", err)
		}
		config.VolumeURL = volumeURL
	}
	if config.FilerURL != "" {
		filerURL, err := normalizeBaseURL(config.FilerURL)
		if err != nil {
			return nil, fmt.Errorf("seaweed: invalid filer url: %w", err)
		}
		config.FilerURL = filerURL
	}
	if config.S3URL != "" {
		s3URL, err := normalizeBaseURL(config.S3URL)
		if err != nil {
			return nil, fmt.Errorf("seaweed: invalid s3 url: %w", err)
		}
		config.S3URL = s3URL
	}
	if config.IAMURL != "" {
		iamURL, err := normalizeBaseURL(config.IAMURL)
		if err != nil {
			return nil, fmt.Errorf("seaweed: invalid iam url: %w", err)
		}
		config.IAMURL = iamURL
	}
	if config.Region == "" {
		config.Region = "us-east-1"
	}

	masterClient, err := master.New(master.Config{
		BaseURL:     config.MasterURL,
		HTTPClient:  applied.httpClient,
		UserAgent:   config.UserAgent,
		BearerToken: config.BearerToken,
		Retry:       config.Retry,
	})
	if err != nil {
		return nil, err
	}
	client := &Client{
		config: config,
		http:   applied.httpClient,
		master: masterClient,
	}
	if config.VolumeURL != "" {
		client.volume, err = volume.New(volume.Config{
			BaseURL:     config.VolumeURL,
			HTTPClient:  applied.httpClient,
			UserAgent:   config.UserAgent,
			BearerToken: config.BearerToken,
			Retry:       config.Retry,
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
	if config.FilerURL != "" {
		client.filer, err = filer.New(filer.Config{
			BaseURL:     config.FilerURL,
			HTTPClient:  applied.httpClient,
			UserAgent:   config.UserAgent,
			BearerToken: config.BearerToken,
			Retry:       config.Retry,
		})
		if err != nil {
			return nil, err
		}
		client.tus, err = tus.New(tus.Config{
			FilerURL:    config.FilerURL,
			BasePath:    config.TUSBasePath,
			HTTPClient:  applied.httpClient,
			UserAgent:   config.UserAgent,
			BearerToken: config.BearerToken,
			Retry:       config.Retry,
			ContentType: "application/offset+octet-stream",
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

func normalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("expected absolute http url")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
