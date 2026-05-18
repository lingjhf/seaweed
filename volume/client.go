package volume

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/lingjhf/seaweed/internal/httpx"
)

// Config configures a volume client.
type Config struct {
	BaseURLs       []string
	HTTPClient     *http.Client
	UserAgent      string
	BearerToken    string
	Retry          RetryPolicy
	EndpointPolicy EndpointPolicy
}

// RetryPolicy controls retry attempts for retryable volume requests.
type RetryPolicy = httpx.RetryPolicy

// EndpointPolicy controls how the client chooses among volume endpoints.
type EndpointPolicy = httpx.EndpointPolicy

// Client calls SeaweedFS volume server HTTP APIs.
type Client struct {
	endpoints *httpx.EndpointSet
	http      *httpx.Client
}

// PutOptions configures a file upload to a volume server.
type PutOptions struct {
	ContentType     string
	ContentEncoding string
	ContentMD5      string
	Filename        string
	ContentLength   int64
}

// PutResponse is returned by a successful volume upload.
type PutResponse struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	ETag string `json:"eTag"`
}

// GetOptions configures a file download from a volume server.
type GetOptions struct {
	Range string
}

// StatusResponse describes volume server status returned by /status.
type StatusResponse struct {
	DiskStatuses []DiskStatus `json:"DiskStatuses"`
	Version      string       `json:"Version"`
	Volumes      []VolumeInfo `json:"Volumes"`
}

// DiskStatus describes one local disk used by a volume server.
type DiskStatus struct {
	Dir         string  `json:"dir"`
	All         uint64  `json:"all"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	PercentFree float64 `json:"percent_free"`
	PercentUsed float64 `json:"percent_used"`
	DiskType    string  `json:"disk_type"`
}

// VolumeInfo describes one local volume on a volume server.
type VolumeInfo struct {
	ID                int                    `json:"Id"`
	Size              uint64                 `json:"Size"`
	ReplicaPlacement  VolumeReplicaPlacement `json:"ReplicaPlacement"`
	TTL               VolumeTTL              `json:"Ttl"`
	DiskType          string                 `json:"DiskType"`
	DiskID            int                    `json:"DiskId"`
	Collection        string                 `json:"Collection"`
	Version           int                    `json:"Version"`
	FileCount         int                    `json:"FileCount"`
	DeleteCount       int                    `json:"DeleteCount"`
	DeletedByteCount  uint64                 `json:"DeletedByteCount"`
	ReadOnly          bool                   `json:"ReadOnly"`
	CompactRevision   uint32                 `json:"CompactRevision"`
	ModifiedAtSecond  int64                  `json:"ModifiedAtSecond"`
	RemoteStorageName string                 `json:"RemoteStorageName"`
	RemoteStorageKey  string                 `json:"RemoteStorageKey"`
}

// VolumeReplicaPlacement describes one volume's replication strategy.
type VolumeReplicaPlacement struct {
	SameRackCount       int `json:"SameRackCount"`
	DiffRackCount       int `json:"DiffRackCount"`
	DiffDataCenterCount int `json:"DiffDataCenterCount"`
}

// VolumeTTL describes one volume's time-to-live policy.
type VolumeTTL struct {
	Count int `json:"Count"`
	Unit  int `json:"Unit"`
}

// New creates a volume client.
func New(config Config) (*Client, error) {
	if len(config.BaseURLs) == 0 {
		return nil, fmt.Errorf("volume: base urls are required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	endpoints, err := httpx.NewEndpointSetWithPolicy(config.BaseURLs, config.EndpointPolicy)
	if err != nil {
		return nil, fmt.Errorf("volume: invalid base urls: %w", err)
	}
	client := &Client{
		endpoints: endpoints,
		http: httpx.NewClient(httpx.Config{
			HTTPClient:  config.HTTPClient,
			UserAgent:   config.UserAgent,
			BearerToken: config.BearerToken,
			Retry:       config.Retry,
		}),
	}
	client.endpoints.StartHealthCheck(config.HTTPClient, http.MethodGet, "/healthz")
	return client, nil
}

// Put writes body to fileID on a volume server.
func (c *Client) Put(ctx context.Context, fileID string, body io.Reader, opts PutOptions) (*PutResponse, error) {
	path, err := c.filePath(fileID)
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	addHeader(header, "Content-Type", opts.ContentType)
	addHeader(header, "Content-Encoding", opts.ContentEncoding)
	addHeader(header, "Content-MD5", opts.ContentMD5)
	addHeader(header, "Content-Disposition", contentDisposition(opts.Filename))

	var out PutResponse
	err = c.http.DecodeJSONEndpoint(ctx, c.endpoints, path, httpx.Request{
		Method:        http.MethodPut,
		Header:        header,
		Body:          body,
		ContentLength: opts.ContentLength,
	}, &out)
	return &out, err
}

// Get returns the file content response for fileID.
func (c *Client) Get(ctx context.Context, fileID string, opts GetOptions) (*http.Response, error) {
	path, err := c.filePath(fileID)
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	addHeader(header, "Range", opts.Range)

	resp, err := c.http.DoEndpoint(ctx, c.endpoints, path, httpx.Request{
		Method:        http.MethodGet,
		Header:        header,
		ContentLength: -1,
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer resp.Body.Close()
		return nil, httpx.ResponseError(http.MethodGet, resp.Request.URL.String(), resp)
	}
	return resp, nil
}

// Head returns response headers for fileID.
func (c *Client) Head(ctx context.Context, fileID string) (http.Header, error) {
	path, err := c.filePath(fileID)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.DoEndpoint(ctx, c.endpoints, path, httpx.Request{
		Method:        http.MethodHead,
		ContentLength: -1,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, httpx.ResponseError(http.MethodHead, resp.Request.URL.String(), resp)
	}
	return resp.Header.Clone(), nil
}

// Delete removes fileID from a volume server.
func (c *Client) Delete(ctx context.Context, fileID string) error {
	path, err := c.filePath(fileID)
	if err != nil {
		return err
	}
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, path, httpx.Request{
		Method:        http.MethodDelete,
		ContentLength: -1,
	}, http.StatusOK, http.StatusAccepted, http.StatusNoContent)
}

// Status returns volume server status data.
func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	var out StatusResponse
	err := c.http.DecodeJSONEndpoint(ctx, c.endpoints, "/status", httpx.Request{
		Method:        http.MethodGet,
		ContentLength: -1,
	}, &out)
	return &out, err
}

// Health checks the volume server health endpoint.
func (c *Client) Health(ctx context.Context) error {
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, "/healthz", httpx.Request{
		Method:        http.MethodGet,
		ContentLength: -1,
	}, http.StatusOK)
}

func (c *Client) filePath(fileID string) (string, error) {
	fileID = strings.TrimLeft(fileID, "/")
	if fileID == "" {
		return "", fmt.Errorf("volume: file id is required")
	}
	return "/" + fileID, nil
}

func addHeader(header http.Header, key string, value string) {
	if value != "" {
		header.Set(key, value)
	}
}

func contentDisposition(filename string) string {
	if filename == "" {
		return ""
	}
	return `inline; filename="` + strings.ReplaceAll(filename, `"`, `\"`) + `"`
}

// Close stops background endpoint health checks.
func (c *Client) Close() {
	c.endpoints.Close()
}
