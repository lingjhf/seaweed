package tus

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/lingjhf/seaweed/internal/httpx"
)

// Version is the TUS protocol version sent by this client.
const Version = "1.0.0"

// Config configures a TUS client.
type Config struct {
	FilerURLs      []string
	BasePath       string
	HTTPClient     *http.Client
	UserAgent      string
	BearerToken    string
	Retry          RetryPolicy
	ContentType    string
	EndpointPolicy EndpointPolicy
}

// RetryPolicy controls retry attempts for retryable TUS requests.
type RetryPolicy = httpx.RetryPolicy

// EndpointPolicy controls how the client chooses among filer endpoints.
type EndpointPolicy = httpx.EndpointPolicy

// Client calls SeaweedFS TUS endpoints.
type Client struct {
	endpoints   *httpx.EndpointSet
	basePath    string
	http        *httpx.Client
	contentType string
}

// Options describes TUS server capabilities.
type Options struct {
	Version                    string
	Versions                   string
	VersionList                []string
	Extensions                 string
	ExtensionList              []string
	MaxSize                    int64
	SupportsCreation           bool
	SupportsCreationWithUpload bool
	SupportsTermination        bool
}

// OptionsOptions configures TUS capability discovery.
type OptionsOptions struct {
	Authorization string
}

// CreateOptions configures upload creation.
type CreateOptions struct {
	Size          int64
	Metadata      map[string]string
	Authorization string
}

// Upload describes a TUS upload resource.
type Upload struct {
	Location string
	Offset   int64
	Size     int64
}

// Status describes the current upload offset and total size.
type Status struct {
	Offset int64
	Size   int64
}

// UploadOptions configures Upload.
type UploadOptions struct {
	Size          int64
	ChunkSize     int64
	Metadata      map[string]string
	Authorization string
}

// ResumeOptions configures Resume.
type ResumeOptions struct {
	ChunkSize     int64
	Authorization string
}

// HeadOptions configures an upload status request.
type HeadOptions struct {
	Authorization string
}

// PatchOptions configures one upload patch request.
type PatchOptions struct {
	Authorization string
}

// TerminateOptions configures upload termination.
type TerminateOptions struct {
	Authorization string
}

// New creates a TUS client.
func New(config Config) (*Client, error) {
	if len(config.FilerURLs) == 0 {
		return nil, errors.New("tus: filer urls are required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	endpoints, err := httpx.NewEndpointSetWithPolicy(config.FilerURLs, config.EndpointPolicy)
	if err != nil {
		return nil, fmt.Errorf("tus: invalid filer urls: %w", err)
	}
	basePath := strings.TrimRight(config.BasePath, "/")
	if basePath == "" {
		basePath = "/.tus"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	contentType := config.ContentType
	if contentType == "" {
		contentType = "application/offset+octet-stream"
	}
	client := &Client{
		endpoints: endpoints,
		basePath:  basePath,
		http: httpx.NewClient(httpx.Config{
			HTTPClient:  config.HTTPClient,
			UserAgent:   config.UserAgent,
			BearerToken: config.BearerToken,
			Retry:       config.Retry,
		}),
		contentType: contentType,
	}
	client.endpoints.StartHealthCheck(config.HTTPClient, http.MethodOptions, basePath+"/")
	return client, nil
}

// Options returns server TUS capability headers.
func (c *Client) Options(ctx context.Context, opts OptionsOptions) (*Options, error) {
	path := c.baseURL("/")
	header := c.baseHeader()
	addAuthorizationHeader(header, opts.Authorization)
	resp, err := c.http.DoEndpoint(ctx, c.endpoints, path, httpx.Request{
		Method:        http.MethodOptions,
		Header:        header,
		ContentLength: -1,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, httpx.ResponseError(http.MethodOptions, resp.Request.URL.String(), resp)
	}
	maxSize, err := parseIntHeader(resp.Header.Get("Tus-Max-Size"))
	if err != nil {
		return nil, err
	}
	versions := splitHeaderList(resp.Header.Get("Tus-Version"))
	extensions := splitHeaderList(resp.Header.Get("Tus-Extension"))
	return &Options{
		Version:                    resp.Header.Get("Tus-Resumable"),
		Versions:                   resp.Header.Get("Tus-Version"),
		VersionList:                versions,
		Extensions:                 resp.Header.Get("Tus-Extension"),
		ExtensionList:              extensions,
		MaxSize:                    maxSize,
		SupportsCreation:           slices.Contains(extensions, "creation"),
		SupportsCreationWithUpload: slices.Contains(extensions, "creation-with-upload"),
		SupportsTermination:        slices.Contains(extensions, "termination"),
	}, nil
}

// Create creates an upload resource without sending file bytes.
func (c *Client) Create(ctx context.Context, targetPath string, opts CreateOptions) (*Upload, error) {
	path := c.baseURL(targetPath)
	header := c.baseHeader()
	header.Set("Upload-Length", strconv.FormatInt(opts.Size, 10))
	addAuthorizationHeader(header, opts.Authorization)
	addMetadata(header, opts.Metadata)

	resp, err := c.http.DoEndpoint(ctx, c.endpoints, path, httpx.Request{
		Method:        http.MethodPost,
		Header:        header,
		ContentLength: 0,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusCreated {
		return nil, httpx.ResponseError(http.MethodPost, resp.Request.URL.String(), resp)
	}
	location, err := resolveLocation(resp.Header.Get("Location"), resp.Request.URL)
	if err != nil {
		return nil, err
	}
	offset, err := parseOptionalIntHeader(resp.Header.Get("Upload-Offset"))
	if err != nil {
		return nil, err
	}
	return &Upload{
		Location: location,
		Offset:   offset,
		Size:     opts.Size,
	}, nil
}

// CreateWithUpload creates an upload resource and sends the body in the create request.
func (c *Client) CreateWithUpload(ctx context.Context, targetPath string, body io.Reader, opts CreateOptions) (*Upload, error) {
	path := c.baseURL(targetPath)
	header := c.baseHeader()
	header.Set("Upload-Length", strconv.FormatInt(opts.Size, 10))
	header.Set("Content-Type", c.contentType)
	addAuthorizationHeader(header, opts.Authorization)
	addMetadata(header, opts.Metadata)

	resp, err := c.http.DoEndpoint(ctx, c.endpoints, path, httpx.Request{
		Method:        http.MethodPost,
		Header:        header,
		Body:          body,
		ContentLength: opts.Size,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusCreated {
		return nil, httpx.ResponseError(http.MethodPost, resp.Request.URL.String(), resp)
	}
	location, err := resolveLocation(resp.Header.Get("Location"), resp.Request.URL)
	if err != nil {
		return nil, err
	}
	offset, err := parseOptionalIntHeader(resp.Header.Get("Upload-Offset"))
	if err != nil {
		return nil, err
	}
	return &Upload{
		Location: location,
		Offset:   offset,
		Size:     opts.Size,
	}, nil
}

// Head returns current upload status for location.
func (c *Client) Head(ctx context.Context, location string, opts HeadOptions) (*Status, error) {
	target, endpointAware, err := c.uploadURL(location)
	if err != nil {
		return nil, err
	}
	header := c.baseHeader()
	addAuthorizationHeader(header, opts.Authorization)
	request := httpx.Request{
		Method:        http.MethodHead,
		Header:        header,
		ContentLength: -1,
	}
	var resp *http.Response
	if endpointAware {
		resp, err = c.http.DoEndpoint(ctx, c.endpoints, target, request)
	} else {
		request.URL = target
		resp, err = c.http.Do(ctx, request)
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, httpx.ResponseError(http.MethodHead, resp.Request.URL.String(), resp)
	}
	offset, err := parseIntHeader(resp.Header.Get("Upload-Offset"))
	if err != nil {
		return nil, err
	}
	size, err := parseIntHeader(resp.Header.Get("Upload-Length"))
	if err != nil {
		return nil, err
	}
	return &Status{Offset: offset, Size: size}, nil
}

// Patch appends bytes to an upload resource at offset.
func (c *Client) Patch(ctx context.Context, location string, offset int64, body io.Reader, length int64, opts PatchOptions) (*Status, error) {
	target, endpointAware, err := c.uploadURL(location)
	if err != nil {
		return nil, err
	}
	header := c.baseHeader()
	header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	header.Set("Content-Type", c.contentType)
	addAuthorizationHeader(header, opts.Authorization)
	request := httpx.Request{
		Method:        http.MethodPatch,
		Header:        header,
		Body:          body,
		ContentLength: length,
	}
	var resp *http.Response
	if endpointAware {
		resp, err = c.http.DoEndpoint(ctx, c.endpoints, target, request)
	} else {
		request.URL = target
		resp, err = c.http.Do(ctx, request)
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusNoContent {
		return nil, httpx.ResponseError(http.MethodPatch, resp.Request.URL.String(), resp)
	}
	newOffset, err := parseIntHeader(resp.Header.Get("Upload-Offset"))
	if err != nil {
		return nil, err
	}
	return &Status{Offset: newOffset}, nil
}

// Terminate deletes an upload resource.
func (c *Client) Terminate(ctx context.Context, location string, opts TerminateOptions) error {
	target, endpointAware, err := c.uploadURL(location)
	if err != nil {
		return err
	}
	header := c.baseHeader()
	addAuthorizationHeader(header, opts.Authorization)
	request := httpx.Request{
		Method:        http.MethodDelete,
		Header:        header,
		ContentLength: -1,
	}
	if endpointAware {
		return c.http.CheckStatusEndpoint(ctx, c.endpoints, target, request, http.StatusNoContent)
	}
	request.URL = target
	return c.http.CheckStatus(ctx, request, http.StatusNoContent)
}

// Upload uploads body, using creation-with-upload unless ChunkSize is positive.
func (c *Client) Upload(ctx context.Context, targetPath string, body io.Reader, opts UploadOptions) (*Upload, error) {
	if opts.Size < 0 {
		return nil, errors.New("tus: size must be non-negative")
	}
	if opts.ChunkSize <= 0 {
		return c.CreateWithUpload(ctx, targetPath, body, CreateOptions{
			Size:          opts.Size,
			Metadata:      opts.Metadata,
			Authorization: opts.Authorization,
		})
	}
	upload, err := c.Create(ctx, targetPath, CreateOptions{
		Size:          opts.Size,
		Metadata:      opts.Metadata,
		Authorization: opts.Authorization,
	})
	if err != nil {
		return nil, err
	}
	offset, err := c.patchChunks(ctx, upload.Location, body, 0, opts.Size, opts.ChunkSize, opts.Authorization)
	if err != nil {
		return nil, err
	}
	upload.Offset = offset
	return upload, nil
}

// Resume seeks body to the server offset and continues an existing upload.
func (c *Client) Resume(ctx context.Context, location string, body io.ReadSeeker, opts ResumeOptions) (*Status, error) {
	status, err := c.Head(ctx, location, HeadOptions{Authorization: opts.Authorization})
	if err != nil {
		return nil, err
	}
	if _, err := body.Seek(status.Offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("tus: seek to offset %d: %w", status.Offset, err)
	}
	offset, err := c.patchChunks(ctx, location, body, status.Offset, status.Size, opts.ChunkSize, opts.Authorization)
	if err != nil {
		return nil, err
	}
	return &Status{Offset: offset, Size: status.Size}, nil
}

func (c *Client) patchChunks(ctx context.Context, location string, body io.Reader, offset int64, size int64, chunkSize int64, authorization string) (int64, error) {
	if chunkSize <= 0 {
		chunkSize = size
	}
	for offset < size {
		length := chunkSize
		if remaining := size - offset; remaining < length {
			length = remaining
		}
		status, err := c.Patch(ctx, location, offset, io.LimitReader(body, length), length, PatchOptions{Authorization: authorization})
		if err != nil {
			return offset, err
		}
		offset = status.Offset
	}
	return offset, nil
}

func (c *Client) baseHeader() http.Header {
	return http.Header{
		"Tus-Resumable": []string{Version},
	}
}

func addAuthorizationHeader(header http.Header, value string) {
	if value != "" {
		header.Set("Authorization", value)
	}
}

func (c *Client) baseURL(path string) string {
	escaped := escapePath(path)
	return c.basePath + escaped
}

func (c *Client) uploadURL(location string) (string, bool, error) {
	if location == "" {
		return "", false, errors.New("tus: location is required")
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return "", false, err
	}
	if parsed.IsAbs() {
		return parsed.String(), false, nil
	}
	if !strings.HasPrefix(location, "/") {
		return "", false, errors.New("tus: relative location must start with /")
	}
	return location, true, nil
}

func resolveLocation(location string, responseURL *url.URL) (string, error) {
	if location == "" {
		return "", errors.New("tus: location is empty")
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}
	if !strings.HasPrefix(location, "/") {
		return "", errors.New("tus: relative location must start with /")
	}
	if responseURL == nil {
		return "", errors.New("tus: response url is required")
	}
	return responseURL.ResolveReference(parsed).String(), nil
}

func addMetadata(header http.Header, metadata map[string]string) {
	if len(metadata) == 0 {
		return
	}
	var builder strings.Builder
	first := true
	for key, value := range metadata {
		if !first {
			builder.WriteByte(',')
		}
		builder.WriteString(key)
		builder.WriteByte(' ')
		builder.WriteString(base64.StdEncoding.EncodeToString([]byte(value)))
		first = false
	}
	header.Set("Upload-Metadata", builder.String())
}

func parseIntHeader(value string) (int64, error) {
	if value == "" {
		return 0, errors.New("tus: missing integer header")
	}
	out, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("tus: invalid integer header %q: %w", value, err)
	}
	return out, nil
}

func parseOptionalIntHeader(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	out, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("tus: invalid integer header %q: %w", value, err)
	}
	return out, nil
}

func splitHeaderList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func escapePath(path string) string {
	if strings.TrimSpace(path) == "" || path == "/" {
		return "/"
	}
	hasTrailingSlash := strings.HasSuffix(path, "/")
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "/"
	}
	var builder strings.Builder
	builder.Grow(len(path) + 8)
	builder.WriteByte('/')
	wrote := false
	for start := 0; start < len(trimmed); {
		for start < len(trimmed) && trimmed[start] == '/' {
			start++
		}
		end := start
		for end < len(trimmed) && trimmed[end] != '/' {
			end++
		}
		if end > start {
			if wrote {
				builder.WriteByte('/')
			}
			builder.WriteString(url.PathEscape(trimmed[start:end]))
			wrote = true
		}
		start = end + 1
	}
	if hasTrailingSlash && wrote {
		builder.WriteByte('/')
	}
	return builder.String()
}

// Close stops background endpoint health checks.
func (c *Client) Close() {
	c.endpoints.Close()
}
