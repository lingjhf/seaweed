package master

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/lingjhf/seaweed/internal/httpx"
)

// Config configures a master client.
type Config struct {
	BaseURLs       []string
	HTTPClient     *http.Client
	UserAgent      string
	BearerToken    string
	Retry          RetryPolicy
	EndpointPolicy EndpointPolicy
}

// RetryPolicy controls retry attempts for retryable master requests.
type RetryPolicy = httpx.RetryPolicy

// EndpointPolicy controls how the client chooses among master endpoints.
type EndpointPolicy = httpx.EndpointPolicy

// Client calls SeaweedFS master HTTP APIs.
type Client struct {
	endpoints *httpx.EndpointSet
	http      *httpx.Client
}

// New creates a master client.
func New(config Config) (*Client, error) {
	if len(config.BaseURLs) == 0 {
		return nil, fmt.Errorf("master: base urls are required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	endpoints, err := httpx.NewEndpointSetWithPolicy(config.BaseURLs, config.EndpointPolicy)
	if err != nil {
		return nil, fmt.Errorf("master: invalid base urls: %w", err)
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
	client.endpoints.StartHealthCheck(config.HTTPClient, http.MethodHead, "/cluster/healthz")
	return client, nil
}

// AssignOptions configures a /dir/assign request.
type AssignOptions struct {
	Count               int
	Collection          string
	DataCenter          string
	Rack                string
	DataNode            string
	Replication         string
	TTL                 string
	Preallocate         int64
	MemoryMapMaxSizeMB  int64
	WritableVolumeCount int
	Disk                string
}

// AssignResponse is returned by /dir/assign.
type AssignResponse struct {
	Count     int    `json:"count"`
	FID       string `json:"fid"`
	URL       string `json:"url"`
	PublicURL string `json:"publicUrl"`
	Error     string `json:"error,omitempty"`
}

// LookupOptions configures a /dir/lookup request.
type LookupOptions struct {
	Collection string
	FileID     string
	Read       bool
}

// LookupResponse is returned by /dir/lookup.
type LookupResponse struct {
	Locations []Location `json:"locations"`
	Error     string     `json:"error,omitempty"`
}

// Location describes a volume server returned by master lookup.
type Location struct {
	URL        string `json:"url"`
	PublicURL  string `json:"publicUrl"`
	DataCenter string `json:"dataCenter,omitempty"`
	Rack       string `json:"rack,omitempty"`
}

// GrowOptions configures a /vol/grow request.
type GrowOptions struct {
	Count              int
	Collection         string
	DataCenter         string
	Rack               string
	DataNode           string
	Replication        string
	TTL                string
	Preallocate        int64
	MemoryMapMaxSizeMB int64
	Disk               string
}

// CountResponse is returned by master endpoints that report a count.
type CountResponse struct {
	Count int    `json:"count"`
	Error string `json:"error,omitempty"`
}

// ClusterStatus describes master leader and peer state.
type ClusterStatus struct {
	IsLeader bool     `json:"IsLeader"`
	Leader   string   `json:"Leader"`
	Peers    []string `json:"Peers"`
}

// Assign asks the master to allocate one or more file IDs.
func (c *Client) Assign(ctx context.Context, opts AssignOptions) (*AssignResponse, error) {
	query := url.Values{}
	httpx.AddInt(query, "count", opts.Count)
	httpx.AddString(query, "collection", opts.Collection)
	httpx.AddString(query, "dataCenter", opts.DataCenter)
	httpx.AddString(query, "rack", opts.Rack)
	httpx.AddString(query, "dataNode", opts.DataNode)
	httpx.AddString(query, "replication", opts.Replication)
	httpx.AddString(query, "ttl", opts.TTL)
	httpx.AddInt64(query, "preallocate", opts.Preallocate)
	httpx.AddInt64(query, "memoryMapMaxSizeMb", opts.MemoryMapMaxSizeMB)
	httpx.AddInt(query, "writableVolumeCount", opts.WritableVolumeCount)
	httpx.AddString(query, "disk", opts.Disk)

	var out AssignResponse
	err := c.http.DecodeJSONEndpoint(ctx, c.endpoints, "/dir/assign", httpx.Request{
		Method: http.MethodGet,
		Query:  query,
	}, &out)
	return &out, err
}

// Lookup finds volume locations for a volume ID.
func (c *Client) Lookup(ctx context.Context, volumeID string, opts LookupOptions) (*LookupResponse, error) {
	query := url.Values{}
	httpx.AddString(query, "volumeId", volumeID)
	httpx.AddString(query, "collection", opts.Collection)
	httpx.AddString(query, "fileId", opts.FileID)
	if opts.Read {
		query.Set("read", "yes")
	}

	var out LookupResponse
	err := c.http.DecodeJSONEndpoint(ctx, c.endpoints, "/dir/lookup", httpx.Request{
		Method: http.MethodGet,
		Query:  query,
	}, &out)
	return &out, err
}

// Vacuum triggers master volume vacuuming with the given garbage threshold.
func (c *Client) Vacuum(ctx context.Context, garbageThreshold float64) error {
	query := url.Values{}
	httpx.AddFloat64(query, "garbageThreshold", garbageThreshold)
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, "/vol/vacuum", httpx.Request{
		Method: http.MethodGet,
		Query:  query,
	}, http.StatusOK)
}

// Grow asks the master to grow volumes.
func (c *Client) Grow(ctx context.Context, opts GrowOptions) (*CountResponse, error) {
	query := url.Values{}
	httpx.AddInt(query, "count", opts.Count)
	httpx.AddString(query, "collection", opts.Collection)
	httpx.AddString(query, "dataCenter", opts.DataCenter)
	httpx.AddString(query, "rack", opts.Rack)
	httpx.AddString(query, "dataNode", opts.DataNode)
	httpx.AddString(query, "replication", opts.Replication)
	httpx.AddString(query, "ttl", opts.TTL)
	httpx.AddInt64(query, "preallocate", opts.Preallocate)
	httpx.AddInt64(query, "memoryMapMaxSizeMb", opts.MemoryMapMaxSizeMB)
	httpx.AddString(query, "disk", opts.Disk)

	var out CountResponse
	err := c.http.DecodeJSONEndpoint(ctx, c.endpoints, "/vol/grow", httpx.Request{
		Method: http.MethodGet,
		Query:  query,
	}, &out)
	return &out, err
}

// DeleteCollection deletes a collection through the master.
func (c *Client) DeleteCollection(ctx context.Context, collection string) error {
	query := url.Values{}
	query.Set("collection", collection)
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, "/col/delete", httpx.Request{
		Method: http.MethodGet,
		Query:  query,
	}, http.StatusOK)
}

// ClusterStatus returns master leader and peer state.
func (c *Client) ClusterStatus(ctx context.Context) (*ClusterStatus, error) {
	var out ClusterStatus
	err := c.http.DecodeJSONEndpoint(ctx, c.endpoints, "/cluster/status", httpx.Request{
		Method: http.MethodGet,
	}, &out)
	return &out, err
}

// Health checks the master health endpoint.
func (c *Client) Health(ctx context.Context) error {
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, "/cluster/healthz", httpx.Request{
		Method: http.MethodHead,
	}, http.StatusOK)
}

// DirStatus returns raw directory status data from the master.
func (c *Client) DirStatus(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	err := c.http.DecodeJSONEndpoint(ctx, c.endpoints, "/dir/status", httpx.Request{
		Method: http.MethodGet,
	}, &out)
	return out, err
}

// VolumeStatus returns raw volume status data from the master.
func (c *Client) VolumeStatus(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	err := c.http.DecodeJSONEndpoint(ctx, c.endpoints, "/vol/status", httpx.Request{
		Method: http.MethodGet,
	}, &out)
	return out, err
}

// Close stops background endpoint health checks.
func (c *Client) Close() {
	c.endpoints.Close()
}
