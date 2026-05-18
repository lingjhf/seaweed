package blob

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/lingjhf/seaweed/internal/httpx"
	"github.com/lingjhf/seaweed/master"
	"github.com/lingjhf/seaweed/volume"
)

type Config struct {
	Master        *master.Client
	HTTPClient    *http.Client
	UserAgent     string
	BearerToken   string
	Retry         RetryPolicy
	UsePublicURLs bool
}

type RetryPolicy = httpx.RetryPolicy

type Client struct {
	master        *master.Client
	httpClient    *http.Client
	userAgent     string
	bearerToken   string
	retry         RetryPolicy
	usePublicURLs bool

	mu        sync.RWMutex
	locations map[string]string
}

type PutOptions struct {
	Collection    string
	DataCenter    string
	Rack          string
	Replication   string
	TTL           string
	ContentType   string
	ContentLength int64
	Filename      string
}

type PutResponse struct {
	FileID string
	Size   int64
	ETag   string
}

type GetOptions struct {
	Range string
}

func New(config Config) (*Client, error) {
	if config.Master == nil {
		return nil, fmt.Errorf("blob: master client is required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Client{
		master:        config.Master,
		httpClient:    config.HTTPClient,
		userAgent:     config.UserAgent,
		bearerToken:   config.BearerToken,
		retry:         config.Retry,
		usePublicURLs: config.UsePublicURLs,
		locations:     map[string]string{},
	}, nil
}

func (c *Client) Put(ctx context.Context, body io.Reader, opts PutOptions) (*PutResponse, error) {
	assigned, err := c.master.Assign(ctx, master.AssignOptions{
		Collection:  opts.Collection,
		DataCenter:  opts.DataCenter,
		Rack:        opts.Rack,
		Replication: opts.Replication,
		TTL:         opts.TTL,
	})
	if err != nil {
		return nil, err
	}
	if assigned.FID == "" {
		return nil, fmt.Errorf("blob: master assign returned empty fid")
	}

	baseURL, err := c.assignedVolumeURL(assigned)
	if err != nil {
		return nil, err
	}
	volumeClient, err := c.volumeClient(baseURL)
	if err != nil {
		return nil, err
	}
	put, err := volumeClient.Put(ctx, assigned.FID, body, volume.PutOptions{
		ContentType:   opts.ContentType,
		ContentLength: opts.ContentLength,
		Filename:      opts.Filename,
	})
	if err != nil {
		return nil, err
	}
	c.remember(volumeID(assigned.FID), baseURL)
	return &PutResponse{
		FileID: assigned.FID,
		Size:   put.Size,
		ETag:   put.ETag,
	}, nil
}

func (c *Client) Get(ctx context.Context, fileID string, opts GetOptions) (*http.Response, error) {
	baseURL, err := c.location(ctx, fileID)
	if err != nil {
		return nil, err
	}
	volumeClient, err := c.volumeClient(baseURL)
	if err != nil {
		return nil, err
	}
	resp, err := volumeClient.Get(ctx, fileID, volume.GetOptions{Range: opts.Range})
	if err != nil {
		if httpx.IsHTTPStatus(err, http.StatusNotFound, http.StatusNotFound) || httpx.IsHTTPStatus(err, http.StatusInternalServerError, 599) {
			c.forget(volumeID(fileID))
		}
		return nil, err
	}
	return resp, nil
}

func (c *Client) Head(ctx context.Context, fileID string) (http.Header, error) {
	baseURL, err := c.location(ctx, fileID)
	if err != nil {
		return nil, err
	}
	volumeClient, err := c.volumeClient(baseURL)
	if err != nil {
		return nil, err
	}
	header, err := volumeClient.Head(ctx, fileID)
	if err != nil {
		if httpx.IsHTTPStatus(err, http.StatusNotFound, http.StatusNotFound) || httpx.IsHTTPStatus(err, http.StatusInternalServerError, 599) {
			c.forget(volumeID(fileID))
		}
		return nil, err
	}
	return header, nil
}

func (c *Client) Delete(ctx context.Context, fileID string) error {
	baseURL, err := c.location(ctx, fileID)
	if err != nil {
		return err
	}
	volumeClient, err := c.volumeClient(baseURL)
	if err != nil {
		return err
	}
	err = volumeClient.Delete(ctx, fileID)
	if err != nil {
		if httpx.IsHTTPStatus(err, http.StatusNotFound, http.StatusNotFound) || httpx.IsHTTPStatus(err, http.StatusInternalServerError, 599) {
			c.forget(volumeID(fileID))
		}
		return err
	}
	return nil
}

func (c *Client) volumeClient(baseURL string) (*volume.Client, error) {
	return volume.New(volume.Config{
		BaseURL:     baseURL,
		HTTPClient:  c.httpClient,
		UserAgent:   c.userAgent,
		BearerToken: c.bearerToken,
		Retry:       c.retry,
	})
}

func (c *Client) location(ctx context.Context, fileID string) (string, error) {
	volumeID := volumeID(fileID)
	if volumeID == "" {
		return "", fmt.Errorf("blob: invalid file id %q", fileID)
	}

	c.mu.RLock()
	baseURL := c.locations[volumeID]
	c.mu.RUnlock()
	if baseURL != "" {
		return baseURL, nil
	}

	lookup, err := c.master.Lookup(ctx, volumeID, master.LookupOptions{Read: true})
	if err != nil {
		return "", err
	}
	if len(lookup.Locations) == 0 {
		return "", fmt.Errorf("blob: no locations for volume %s", volumeID)
	}
	baseURL, err = c.lookupVolumeURL(lookup.Locations[0])
	if err != nil {
		return "", err
	}
	c.remember(volumeID, baseURL)
	return baseURL, nil
}

func (c *Client) assignedVolumeURL(resp *master.AssignResponse) (string, error) {
	raw := resp.URL
	if c.usePublicURLs {
		raw = resp.PublicURL
	}
	return normalizeVolumeURL(raw)
}

func (c *Client) lookupVolumeURL(location master.Location) (string, error) {
	raw := location.URL
	if c.usePublicURLs {
		raw = location.PublicURL
	}
	return normalizeVolumeURL(raw)
}

func (c *Client) remember(volumeID string, baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.locations[volumeID] = baseURL
}

func (c *Client) forget(volumeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.locations, volumeID)
}

func normalizeVolumeURL(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("blob: volume url is empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("blob: invalid volume url %q", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func volumeID(fileID string) string {
	id, _, _ := strings.Cut(fileID, ",")
	return id
}
