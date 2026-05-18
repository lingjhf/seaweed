package master

import (
	"context"
	"net/http"
	"net/url"

	"github.com/lingjhf/seaweed/internal/httpx"
)

type Config struct {
	BaseURL string
	HTTP    *httpx.Client
}

type Client struct {
	baseURL string
	http    *httpx.Client
}

func New(config Config) *Client {
	return &Client{
		baseURL: config.BaseURL,
		http:    config.HTTP,
	}
}

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

type AssignResponse struct {
	Count     int    `json:"count"`
	FID       string `json:"fid"`
	URL       string `json:"url"`
	PublicURL string `json:"publicUrl"`
	Error     string `json:"error,omitempty"`
}

type LookupOptions struct {
	Collection string
	FileID     string
	Read       bool
}

type LookupResponse struct {
	Locations []Location `json:"locations"`
	Error     string     `json:"error,omitempty"`
}

type Location struct {
	URL        string `json:"url"`
	PublicURL  string `json:"publicUrl"`
	DataCenter string `json:"dataCenter,omitempty"`
	Rack       string `json:"rack,omitempty"`
}

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

type CountResponse struct {
	Count int    `json:"count"`
	Error string `json:"error,omitempty"`
}

type ClusterStatus struct {
	IsLeader bool     `json:"IsLeader"`
	Leader   string   `json:"Leader"`
	Peers    []string `json:"Peers"`
}

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
	err := c.http.DecodeJSON(ctx, httpx.Request{
		Method: http.MethodGet,
		URL:    c.baseURL + "/dir/assign",
		Query:  query,
	}, &out)
	return &out, err
}

func (c *Client) Lookup(ctx context.Context, volumeID string, opts LookupOptions) (*LookupResponse, error) {
	query := url.Values{}
	httpx.AddString(query, "volumeId", volumeID)
	httpx.AddString(query, "collection", opts.Collection)
	httpx.AddString(query, "fileId", opts.FileID)
	if opts.Read {
		query.Set("read", "yes")
	}

	var out LookupResponse
	err := c.http.DecodeJSON(ctx, httpx.Request{
		Method: http.MethodGet,
		URL:    c.baseURL + "/dir/lookup",
		Query:  query,
	}, &out)
	return &out, err
}

func (c *Client) Vacuum(ctx context.Context, garbageThreshold float64) error {
	query := url.Values{}
	httpx.AddFloat64(query, "garbageThreshold", garbageThreshold)
	return c.http.CheckStatus(ctx, httpx.Request{
		Method: http.MethodGet,
		URL:    c.baseURL + "/vol/vacuum",
		Query:  query,
	}, http.StatusOK)
}

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
	err := c.http.DecodeJSON(ctx, httpx.Request{
		Method: http.MethodGet,
		URL:    c.baseURL + "/vol/grow",
		Query:  query,
	}, &out)
	return &out, err
}

func (c *Client) DeleteCollection(ctx context.Context, collection string) error {
	query := url.Values{}
	query.Set("collection", collection)
	return c.http.CheckStatus(ctx, httpx.Request{
		Method: http.MethodGet,
		URL:    c.baseURL + "/col/delete",
		Query:  query,
	}, http.StatusOK)
}

func (c *Client) ClusterStatus(ctx context.Context) (*ClusterStatus, error) {
	var out ClusterStatus
	err := c.http.DecodeJSON(ctx, httpx.Request{
		Method: http.MethodGet,
		URL:    c.baseURL + "/cluster/status",
	}, &out)
	return &out, err
}

func (c *Client) Health(ctx context.Context) error {
	return c.http.CheckStatus(ctx, httpx.Request{
		Method: http.MethodHead,
		URL:    c.baseURL + "/cluster/healthz",
	}, http.StatusOK)
}

func (c *Client) DirStatus(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	err := c.http.DecodeJSON(ctx, httpx.Request{
		Method: http.MethodGet,
		URL:    c.baseURL + "/dir/status",
	}, &out)
	return out, err
}

func (c *Client) VolumeStatus(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	err := c.http.DecodeJSON(ctx, httpx.Request{
		Method: http.MethodGet,
		URL:    c.baseURL + "/vol/status",
	}, &out)
	return out, err
}
