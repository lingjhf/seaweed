package master

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

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
		return nil, errors.New("master: base urls are required")
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
	Count         int    `json:"count"`
	FID           string `json:"fid"`
	URL           string `json:"url"`
	PublicURL     string `json:"publicUrl"`
	Authorization string `json:"-"`
}

// LookupOptions configures a /dir/lookup request.
type LookupOptions struct {
	Collection string
	FileID     string
	Read       bool
}

// LookupResponse is returned by /dir/lookup.
type LookupResponse struct {
	Locations     []Location `json:"locations"`
	Authorization string     `json:"-"`
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

// VacuumOptions configures a /vol/vacuum request.
type VacuumOptions struct {
	GarbageThreshold float64
}

// DeleteCollectionOptions configures a /col/delete request.
type DeleteCollectionOptions struct {
	Collection string
}

// CountResponse is returned by master endpoints that report a count.
type CountResponse struct {
	Count int `json:"count"`
}

// SubmitOptions configures a /submit upload request.
type SubmitOptions struct {
	FieldName       string
	FileContentType string
}

// SubmitResponse is returned by /submit.
type SubmitResponse struct {
	FID      string `json:"fid"`
	FileName string `json:"fileName"`
	FileURL  string `json:"fileUrl"`
	Size     int64  `json:"size"`
}

// ClusterStatus describes master leader and peer state.
type ClusterStatus struct {
	IsLeader bool     `json:"IsLeader"`
	Leader   string   `json:"Leader"`
	Peers    []string `json:"Peers"`
}

// DirStatusResponse describes writable volume status returned by /dir/status.
type DirStatusResponse struct {
	Topology Topology `json:"Topology"`
	Version  string   `json:"Version"`
}

// Topology describes master writable-volume topology.
type Topology struct {
	DataCenters []DataCenter           `json:"DataCenters"`
	Free        int                    `json:"Free"`
	Max         int                    `json:"Max"`
	Layouts     []WritableVolumeLayout `json:"layouts"`
}

// DataCenter describes one SeaweedFS data center in master topology.
type DataCenter struct {
	ID    string `json:"Id"`
	Free  int    `json:"Free"`
	Max   int    `json:"Max"`
	Racks []Rack `json:"Racks"`
}

// Rack describes one rack in master topology.
type Rack struct {
	ID        string     `json:"Id"`
	Free      int        `json:"Free"`
	Max       int        `json:"Max"`
	DataNodes []DataNode `json:"DataNodes"`
}

// DataNode describes one volume server in master topology.
type DataNode struct {
	URL       string `json:"Url"`
	PublicURL string `json:"PublicUrl"`
	Free      int    `json:"Free"`
	Max       int    `json:"Max"`
	Volumes   int    `json:"Volumes"`
}

// WritableVolumeLayout describes writable volume IDs for one collection layout.
type WritableVolumeLayout struct {
	Collection  string `json:"collection"`
	Replication string `json:"replication"`
	Writables   []int  `json:"writables"`
}

// VolumeStatusResponse describes volume placement returned by /vol/status.
type VolumeStatusResponse struct {
	Version string         `json:"Version"`
	Volumes VolumeTopology `json:"Volumes"`
}

// VolumeTopology describes all volumes grouped by data center, rack, and node.
type VolumeTopology struct {
	DataCenters VolumeDataCenters `json:"DataCenters"`
	Free        int               `json:"Free"`
	Max         int               `json:"Max"`
}

// VolumeDataCenters maps data center IDs to racks.
type VolumeDataCenters map[string]VolumeRacks

// VolumeRacks maps rack IDs to volume server nodes.
type VolumeRacks map[string]VolumeDataNodes

// VolumeDataNodes maps volume server addresses to volumes.
type VolumeDataNodes map[string][]VolumeInfo

// VolumeInfo describes one volume reported by /vol/status.
type VolumeInfo struct {
	ID                int                    `json:"Id"`
	Size              int64                  `json:"Size"`
	ReplicaPlacement  VolumeReplicaPlacement `json:"ReplicaPlacement"`
	RepType           string                 `json:"RepType"`
	TTL               VolumeTTL              `json:"Ttl"`
	DiskType          string                 `json:"DiskType"`
	Collection        string                 `json:"Collection"`
	Version           int                    `json:"Version"`
	FileCount         int64                  `json:"FileCount"`
	DeleteCount       int64                  `json:"DeleteCount"`
	DeletedByteCount  int64                  `json:"DeletedByteCount"`
	ReadOnly          bool                   `json:"ReadOnly"`
	CompactRevision   int                    `json:"CompactRevision"`
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
	header, err := c.http.DecodeJSONEndpointWithHeader(ctx, c.endpoints, "/dir/assign", httpx.Request{
		Method: http.MethodGet,
		Query:  query,
	}, &out)
	if header != nil {
		out.Authorization = header.Get("Authorization")
	}
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
	header, err := c.http.DecodeJSONEndpointWithHeader(ctx, c.endpoints, "/dir/lookup", httpx.Request{
		Method: http.MethodGet,
		Query:  query,
	}, &out)
	if header != nil {
		out.Authorization = header.Get("Authorization")
	}
	return &out, err
}

// Submit uploads a file through the master's /submit convenience endpoint.
func (c *Client) Submit(ctx context.Context, filename string, body io.Reader, opts SubmitOptions) (*SubmitResponse, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, errors.New("master: filename is required")
	}
	if body == nil {
		return nil, errors.New("master: body is required")
	}

	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	go writeMultipartBody(writer, multipartWriter, submitFieldName(opts), filename, body, opts.FileContentType)

	var out SubmitResponse
	err := c.http.DecodeJSONEndpoint(ctx, c.endpoints, "/submit", httpx.Request{
		Method: http.MethodPost,
		Header: http.Header{
			"Content-Type": []string{multipartWriter.FormDataContentType()},
		},
		Body:          reader,
		ContentLength: -1,
	}, &out)
	return &out, err
}

// Vacuum triggers master volume vacuuming.
func (c *Client) Vacuum(ctx context.Context, opts VacuumOptions) error {
	query := url.Values{}
	httpx.AddFloat64(query, "garbageThreshold", opts.GarbageThreshold)
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, "/vol/vacuum", httpx.Request{
		Method: http.MethodGet,
		Query:  query,
	}, http.StatusOK)
}

// Grow asks the master to grow volumes.
func (c *Client) Grow(ctx context.Context, opts GrowOptions) (*CountResponse, error) {
	if opts.Count <= 0 {
		return nil, errors.New("master: grow count is required")
	}
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
func (c *Client) DeleteCollection(ctx context.Context, opts DeleteCollectionOptions) error {
	if strings.TrimSpace(opts.Collection) == "" {
		return errors.New("master: collection is required")
	}
	query := url.Values{}
	query.Set("collection", opts.Collection)
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

// DirStatus returns writable volume topology from the master.
func (c *Client) DirStatus(ctx context.Context) (*DirStatusResponse, error) {
	var out DirStatusResponse
	err := c.http.DecodeJSONEndpoint(ctx, c.endpoints, "/dir/status", httpx.Request{
		Method: http.MethodGet,
	}, &out)
	return &out, err
}

// VolumeStatus returns volume placement status from the master.
func (c *Client) VolumeStatus(ctx context.Context) (*VolumeStatusResponse, error) {
	var out VolumeStatusResponse
	err := c.http.DecodeJSONEndpoint(ctx, c.endpoints, "/vol/status", httpx.Request{
		Method: http.MethodGet,
	}, &out)
	return &out, err
}

// Close stops background endpoint health checks.
func (c *Client) Close() {
	c.endpoints.Close()
}

func submitFieldName(opts SubmitOptions) string {
	if strings.TrimSpace(opts.FieldName) == "" {
		return "file"
	}
	return opts.FieldName
}

func writeMultipartBody(pipeWriter *io.PipeWriter, multipartWriter *multipart.Writer, fieldName string, filename string, body io.Reader, contentType string) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name":     fieldName,
		"filename": filename,
	}))
	header.Set("Content-Type", contentType)

	part, err := multipartWriter.CreatePart(header)
	if err == nil {
		_, err = io.Copy(part, body)
	}
	if closeErr := multipartWriter.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = pipeWriter.CloseWithError(err)
		return
	}
	_ = pipeWriter.Close()
}
