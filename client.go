package seaweed

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/lingjhf/seaweed/blob"
	"github.com/lingjhf/seaweed/internal/httpx"
	"github.com/lingjhf/seaweed/master"
	"github.com/lingjhf/seaweed/volume"
)

const defaultTusBasePath = "/.tus"

type Client struct {
	config Config

	master *master.Client
	volume *volume.Client
	blob   *blob.Client
}

func New(config Config, opts ...Option) (*Client, error) {
	applied := options{
		httpClient: http.DefaultClient,
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
	if config.TusBasePath == "" {
		config.TusBasePath = defaultTusBasePath
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

	transport := httpx.NewClient(httpx.Config{
		HTTPClient:  applied.httpClient,
		UserAgent:   config.UserAgent,
		BearerToken: config.BearerToken,
		Retry:       config.Retry,
	})

	client := &Client{
		config: config,
		master: master.New(master.Config{
			BaseURL: config.MasterURL,
			HTTP:    transport,
		}),
	}
	client.volume = volume.New(volume.Config{
		BaseURL: config.VolumeURL,
		HTTP:    transport,
	})
	client.blob = blob.New(blob.Config{
		Master:        client.master,
		HTTP:          transport,
		UsePublicURLs: config.UsePublicURLs,
	})
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
