package blob

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lingjhf/seaweed/internal/httpx"
	"github.com/lingjhf/seaweed/master"
	"github.com/lingjhf/seaweed/volume"
)

// Config configures a blob client.
type Config struct {
	Master           *master.Client
	HTTPClient       *http.Client
	UserAgent        string
	BearerToken      string
	Retry            RetryPolicy
	EndpointPolicy   EndpointPolicy
	UsePublicURLs    bool
	LocationCacheTTL time.Duration
	// EnableVolumeAuthorization makes Blob use master-issued per-file
	// Authorization headers for volume reads, writes, and deletes.
	EnableVolumeAuthorization bool
}

// RetryPolicy controls retry attempts for retryable blob volume requests.
type RetryPolicy = httpx.RetryPolicy

// EndpointPolicy controls how blob reads choose among volume locations.
type EndpointPolicy = httpx.EndpointPolicy

// Client uploads and reads SeaweedFS blobs by file ID.
type Client struct {
	master           *master.Client
	httpClient       *http.Client
	userAgent        string
	bearerToken      string
	retry            RetryPolicy
	endpointPolicy   EndpointPolicy
	usePublicURLs    bool
	locationCacheTTL time.Duration
	enableVolumeAuth bool

	mu        sync.RWMutex
	locations map[string]locationCacheEntry
	lookups   map[string]*lookupFlight
}

type locationCacheEntry struct {
	baseURLs  []string
	client    *volume.Client
	expiresAt time.Time
}

type lookupFlight struct {
	done   chan struct{}
	client *volume.Client
	err    error
}

// PutOptions configures a blob upload.
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

// PutResponse is returned by a successful blob upload.
type PutResponse struct {
	FileID string
	Size   int64
	ETag   string
}

// GetOptions configures a blob read.
type GetOptions struct {
	Range string
}

// New creates a blob client.
func New(config Config) (*Client, error) {
	if config.Master == nil {
		return nil, fmt.Errorf("blob: master client is required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	endpointPolicy, err := httpx.NormalizeEndpointPolicy(config.EndpointPolicy)
	if err != nil {
		return nil, fmt.Errorf("blob: invalid endpoint policy: %w", err)
	}
	return &Client{
		master:           config.Master,
		httpClient:       config.HTTPClient,
		userAgent:        config.UserAgent,
		bearerToken:      config.BearerToken,
		retry:            config.Retry,
		endpointPolicy:   endpointPolicy,
		usePublicURLs:    config.UsePublicURLs,
		locationCacheTTL: config.LocationCacheTTL,
		enableVolumeAuth: config.EnableVolumeAuthorization,
		locations:        map[string]locationCacheEntry{},
		lookups:          map[string]*lookupFlight{},
	}, nil
}

// Put assigns a file ID through master and writes body to the assigned volume.
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
	volumeClient, err := c.volumeClient([]string{baseURL})
	if err != nil {
		return nil, err
	}
	defer volumeClient.Close()
	putOptions := volume.PutOptions{
		ContentType:   opts.ContentType,
		ContentLength: opts.ContentLength,
		Filename:      opts.Filename,
	}
	if c.enableVolumeAuth {
		putOptions.Authorization = assigned.Authorization
	}
	put, err := volumeClient.Put(ctx, assigned.FID, body, putOptions)
	if err != nil {
		return nil, err
	}
	if _, err := c.remember(volumeID(assigned.FID), []string{baseURL}); err != nil {
		return nil, err
	}
	return &PutResponse{
		FileID: assigned.FID,
		Size:   put.Size,
		ETag:   put.ETag,
	}, nil
}

// Get reads fileID from its volume server.
func (c *Client) Get(ctx context.Context, fileID string, opts GetOptions) (*http.Response, error) {
	if c.enableVolumeAuth {
		return c.getWithAuthorization(ctx, fileID, opts)
	}
	volumeClient, err := c.volumeClientFor(ctx, fileID)
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

// Head returns headers for fileID from its volume server.
func (c *Client) Head(ctx context.Context, fileID string) (http.Header, error) {
	if c.enableVolumeAuth {
		return c.headWithAuthorization(ctx, fileID)
	}
	volumeClient, err := c.volumeClientFor(ctx, fileID)
	if err != nil {
		return nil, err
	}
	header, err := volumeClient.Head(ctx, fileID, volume.HeadOptions{})
	if err != nil {
		if httpx.IsHTTPStatus(err, http.StatusNotFound, http.StatusNotFound) || httpx.IsHTTPStatus(err, http.StatusInternalServerError, 599) {
			c.forget(volumeID(fileID))
		}
		return nil, err
	}
	return header, nil
}

// Delete removes fileID from its volume server.
func (c *Client) Delete(ctx context.Context, fileID string) error {
	if c.enableVolumeAuth {
		return c.deleteWithAuthorization(ctx, fileID)
	}
	volumeClient, err := c.volumeClientFor(ctx, fileID)
	if err != nil {
		return err
	}
	err = volumeClient.Delete(ctx, fileID, volume.DeleteOptions{})
	if err != nil {
		if httpx.IsHTTPStatus(err, http.StatusNotFound, http.StatusNotFound) || httpx.IsHTTPStatus(err, http.StatusInternalServerError, 599) {
			c.forget(volumeID(fileID))
		}
		return err
	}
	return nil
}

func (c *Client) getWithAuthorization(ctx context.Context, fileID string, opts GetOptions) (*http.Response, error) {
	volumeClient, authorization, err := c.authorizedVolumeClientFor(ctx, fileID, true)
	if err != nil {
		return nil, err
	}
	defer volumeClient.Close()
	return volumeClient.Get(ctx, fileID, volume.GetOptions{
		Range:         opts.Range,
		Authorization: authorization,
	})
}

func (c *Client) headWithAuthorization(ctx context.Context, fileID string) (http.Header, error) {
	volumeClient, authorization, err := c.authorizedVolumeClientFor(ctx, fileID, true)
	if err != nil {
		return nil, err
	}
	defer volumeClient.Close()
	return volumeClient.Head(ctx, fileID, volume.HeadOptions{Authorization: authorization})
}

func (c *Client) deleteWithAuthorization(ctx context.Context, fileID string) error {
	volumeClient, authorization, err := c.authorizedVolumeClientFor(ctx, fileID, false)
	if err != nil {
		return err
	}
	defer volumeClient.Close()
	return volumeClient.Delete(ctx, fileID, volume.DeleteOptions{Authorization: authorization})
}

func (c *Client) authorizedVolumeClientFor(ctx context.Context, fileID string, read bool) (*volume.Client, string, error) {
	volumeID := volumeID(fileID)
	if volumeID == "" {
		return nil, "", fmt.Errorf("blob: invalid file id %q", fileID)
	}
	lookup, err := c.master.Lookup(ctx, volumeID, master.LookupOptions{
		FileID: fileID,
		Read:   read,
	})
	if err != nil {
		return nil, "", err
	}
	if len(lookup.Locations) == 0 {
		return nil, "", fmt.Errorf("blob: no locations for volume %s", volumeID)
	}
	baseURLs, err := c.lookupVolumeURLs(lookup.Locations)
	if err != nil {
		return nil, "", err
	}
	volumeClient, err := c.volumeClient(baseURLs)
	if err != nil {
		return nil, "", err
	}
	return volumeClient, lookup.Authorization, nil
}

func (c *Client) volumeClient(baseURLs []string) (*volume.Client, error) {
	return volume.New(volume.Config{
		BaseURLs:       baseURLs,
		HTTPClient:     c.httpClient,
		UserAgent:      c.userAgent,
		BearerToken:    c.bearerToken,
		Retry:          c.retry,
		EndpointPolicy: c.endpointPolicy,
	})
}

func (c *Client) volumeClientFor(ctx context.Context, fileID string) (*volume.Client, error) {
	volumeID := volumeID(fileID)
	if volumeID == "" {
		return nil, fmt.Errorf("blob: invalid file id %q", fileID)
	}

	if volumeClient, ok := c.cachedVolumeClient(volumeID); ok {
		return volumeClient, nil
	}
	flight, leader, cached := c.beginLookup(volumeID)
	if cached != nil {
		return cached, nil
	}
	if !leader {
		return waitLookup(ctx, flight)
	}

	volumeClient, err := c.lookupVolumeClient(ctx, volumeID)
	c.finishLookup(volumeID, flight, volumeClient, err)
	return volumeClient, err
}

func (c *Client) lookupVolumeClient(ctx context.Context, volumeID string) (*volume.Client, error) {
	lookup, err := c.master.Lookup(ctx, volumeID, master.LookupOptions{Read: true})
	if err != nil {
		return nil, err
	}
	if len(lookup.Locations) == 0 {
		return nil, fmt.Errorf("blob: no locations for volume %s", volumeID)
	}
	baseURLs, err := c.lookupVolumeURLs(lookup.Locations)
	if err != nil {
		return nil, err
	}
	return c.remember(volumeID, baseURLs)
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

func (c *Client) lookupVolumeURLs(locations []master.Location) ([]string, error) {
	baseURLs := make([]string, 0, len(locations))
	seen := map[string]struct{}{}
	for _, location := range locations {
		baseURL, err := c.lookupVolumeURL(location)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[baseURL]; ok {
			continue
		}
		seen[baseURL] = struct{}{}
		baseURLs = append(baseURLs, baseURL)
	}
	if len(baseURLs) == 0 {
		return nil, fmt.Errorf("blob: no locations for volume")
	}
	return baseURLs, nil
}

func (c *Client) cachedVolumeClient(volumeID string) (*volume.Client, bool) {
	now := time.Now()
	c.mu.RLock()
	entry, ok := c.locations[volumeID]
	if ok && entry.valid(now) {
		c.mu.RUnlock()
		return entry.client, true
	}
	c.mu.RUnlock()

	if ok {
		c.forgetExpired(volumeID, now)
	}
	return nil, false
}

func (c *Client) beginLookup(volumeID string) (*lookupFlight, bool, *volume.Client) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.locations[volumeID]; ok && entry.valid(now) {
		return nil, false, entry.client
	}
	if flight, ok := c.lookups[volumeID]; ok {
		return flight, false, nil
	}
	flight := &lookupFlight{
		done: make(chan struct{}),
	}
	c.lookups[volumeID] = flight
	return flight, true, nil
}

func (c *Client) finishLookup(volumeID string, flight *lookupFlight, volumeClient *volume.Client, err error) {
	flight.client = volumeClient
	flight.err = err

	c.mu.Lock()
	if c.lookups[volumeID] == flight {
		delete(c.lookups, volumeID)
	}
	c.mu.Unlock()
	close(flight.done)
}

func waitLookup(ctx context.Context, flight *lookupFlight) (*volume.Client, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-flight.done:
		return flight.client, flight.err
	}
}

func (entry locationCacheEntry) valid(now time.Time) bool {
	return entry.client != nil && len(entry.baseURLs) > 0 && (entry.expiresAt.IsZero() || now.Before(entry.expiresAt))
}

func (c *Client) remember(volumeID string, baseURLs []string) (*volume.Client, error) {
	volumeClient, err := c.volumeClient(baseURLs)
	if err != nil {
		return nil, err
	}
	entry := locationCacheEntry{
		baseURLs:  cloneStrings(baseURLs),
		client:    volumeClient,
		expiresAt: c.locationExpiresAt(),
	}

	var oldClient *volume.Client
	c.mu.Lock()
	if oldEntry, ok := c.locations[volumeID]; ok {
		oldClient = oldEntry.client
	}
	c.locations[volumeID] = entry
	c.mu.Unlock()
	if oldClient != nil {
		oldClient.Close()
	}
	return volumeClient, nil
}

func (c *Client) forget(volumeID string) {
	var oldClient *volume.Client
	c.mu.Lock()
	if oldEntry, ok := c.locations[volumeID]; ok {
		oldClient = oldEntry.client
		delete(c.locations, volumeID)
	}
	c.mu.Unlock()
	if oldClient != nil {
		oldClient.Close()
	}
}

func (c *Client) forgetExpired(volumeID string, now time.Time) {
	var oldClient *volume.Client
	c.mu.Lock()
	if oldEntry, ok := c.locations[volumeID]; ok && !oldEntry.valid(now) {
		oldClient = oldEntry.client
		delete(c.locations, volumeID)
	}
	c.mu.Unlock()
	if oldClient != nil {
		oldClient.Close()
	}
}

func (c *Client) locationExpiresAt() time.Time {
	if c.locationCacheTTL <= 0 {
		return time.Time{}
	}
	return time.Now().Add(c.locationCacheTTL)
}

// Close releases cached volume clients.
func (c *Client) Close() {
	c.mu.Lock()
	locations := c.locations
	c.locations = map[string]locationCacheEntry{}
	c.mu.Unlock()

	for _, entry := range locations {
		if entry.client != nil {
			entry.client.Close()
		}
	}
}

func cloneStrings(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
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
