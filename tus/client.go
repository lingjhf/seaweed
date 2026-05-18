package tus

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/lingjhf/seaweed/internal/httpx"
)

const Version = "1.0.0"

type Config struct {
	FilerURL    string
	BasePath    string
	HTTPClient  *http.Client
	UserAgent   string
	BearerToken string
	Retry       RetryPolicy
	ContentType string
}

type RetryPolicy = httpx.RetryPolicy

type Client struct {
	filerURL    string
	basePath    string
	http        *httpx.Client
	contentType string
}

type Options struct {
	Version    string
	Versions   string
	Extensions string
	MaxSize    int64
}

type CreateOptions struct {
	Size     int64
	Metadata map[string]string
}

type Upload struct {
	Location string
	Offset   int64
	Size     int64
}

type Status struct {
	Offset int64
	Size   int64
}

type UploadOptions struct {
	Size      int64
	ChunkSize int64
	Metadata  map[string]string
}

type ResumeOptions struct {
	ChunkSize int64
}

func New(config Config) (*Client, error) {
	if config.FilerURL == "" {
		return nil, fmt.Errorf("tus: filer url is required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
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
	return &Client{
		filerURL: strings.TrimRight(config.FilerURL, "/"),
		basePath: basePath,
		http: httpx.NewClient(httpx.Config{
			HTTPClient:  config.HTTPClient,
			UserAgent:   config.UserAgent,
			BearerToken: config.BearerToken,
			Retry:       config.Retry,
		}),
		contentType: contentType,
	}, nil
}

func (c *Client) Options(ctx context.Context) (*Options, error) {
	rawURL, err := c.baseURL("/")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(ctx, httpx.Request{
		Method:        http.MethodOptions,
		URL:           rawURL,
		ContentLength: -1,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, httpx.ResponseError(http.MethodOptions, rawURL, resp)
	}
	maxSize, err := parseIntHeader(resp.Header.Get("Tus-Max-Size"))
	if err != nil {
		return nil, err
	}
	return &Options{
		Version:    resp.Header.Get("Tus-Resumable"),
		Versions:   resp.Header.Get("Tus-Version"),
		Extensions: resp.Header.Get("Tus-Extension"),
		MaxSize:    maxSize,
	}, nil
}

func (c *Client) Create(ctx context.Context, targetPath string, opts CreateOptions) (*Upload, error) {
	rawURL, err := c.baseURL(targetPath)
	if err != nil {
		return nil, err
	}
	header := c.baseHeader()
	header.Set("Upload-Length", strconv.FormatInt(opts.Size, 10))
	addMetadata(header, opts.Metadata)

	resp, err := c.http.Do(ctx, httpx.Request{
		Method:        http.MethodPost,
		URL:           rawURL,
		Header:        header,
		ContentLength: 0,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, httpx.ResponseError(http.MethodPost, rawURL, resp)
	}
	location, err := c.resolveLocation(resp.Header.Get("Location"))
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

func (c *Client) CreateWithUpload(ctx context.Context, targetPath string, body io.Reader, opts CreateOptions) (*Upload, error) {
	rawURL, err := c.baseURL(targetPath)
	if err != nil {
		return nil, err
	}
	header := c.baseHeader()
	header.Set("Upload-Length", strconv.FormatInt(opts.Size, 10))
	header.Set("Content-Type", c.contentType)
	addMetadata(header, opts.Metadata)

	resp, err := c.http.Do(ctx, httpx.Request{
		Method:        http.MethodPost,
		URL:           rawURL,
		Header:        header,
		Body:          body,
		ContentLength: opts.Size,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, httpx.ResponseError(http.MethodPost, rawURL, resp)
	}
	location, err := c.resolveLocation(resp.Header.Get("Location"))
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

func (c *Client) Head(ctx context.Context, location string) (*Status, error) {
	rawURL, err := c.uploadURL(location)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(ctx, httpx.Request{
		Method:        http.MethodHead,
		URL:           rawURL,
		Header:        c.baseHeader(),
		ContentLength: -1,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, httpx.ResponseError(http.MethodHead, rawURL, resp)
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

func (c *Client) Patch(ctx context.Context, location string, offset int64, body io.Reader, length int64) (*Status, error) {
	rawURL, err := c.uploadURL(location)
	if err != nil {
		return nil, err
	}
	header := c.baseHeader()
	header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	header.Set("Content-Type", c.contentType)
	resp, err := c.http.Do(ctx, httpx.Request{
		Method:        http.MethodPatch,
		URL:           rawURL,
		Header:        header,
		Body:          body,
		ContentLength: length,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return nil, httpx.ResponseError(http.MethodPatch, rawURL, resp)
	}
	newOffset, err := parseIntHeader(resp.Header.Get("Upload-Offset"))
	if err != nil {
		return nil, err
	}
	return &Status{Offset: newOffset}, nil
}

func (c *Client) Terminate(ctx context.Context, location string) error {
	rawURL, err := c.uploadURL(location)
	if err != nil {
		return err
	}
	return c.http.CheckStatus(ctx, httpx.Request{
		Method:        http.MethodDelete,
		URL:           rawURL,
		Header:        c.baseHeader(),
		ContentLength: -1,
	}, http.StatusNoContent)
}

func (c *Client) Upload(ctx context.Context, targetPath string, body io.Reader, opts UploadOptions) (*Upload, error) {
	if opts.Size < 0 {
		return nil, fmt.Errorf("tus: size must be non-negative")
	}
	if opts.ChunkSize <= 0 {
		return c.CreateWithUpload(ctx, targetPath, body, CreateOptions{
			Size:     opts.Size,
			Metadata: opts.Metadata,
		})
	}
	upload, err := c.Create(ctx, targetPath, CreateOptions{
		Size:     opts.Size,
		Metadata: opts.Metadata,
	})
	if err != nil {
		return nil, err
	}
	offset, err := c.patchChunks(ctx, upload.Location, body, 0, opts.Size, opts.ChunkSize)
	if err != nil {
		return nil, err
	}
	upload.Offset = offset
	return upload, nil
}

func (c *Client) Resume(ctx context.Context, location string, body io.ReadSeeker, opts ResumeOptions) (*Status, error) {
	status, err := c.Head(ctx, location)
	if err != nil {
		return nil, err
	}
	if _, err := body.Seek(status.Offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("tus: seek to offset %d: %w", status.Offset, err)
	}
	offset, err := c.patchChunks(ctx, location, body, status.Offset, status.Size, opts.ChunkSize)
	if err != nil {
		return nil, err
	}
	return &Status{Offset: offset, Size: status.Size}, nil
}

func (c *Client) patchChunks(ctx context.Context, location string, body io.Reader, offset int64, size int64, chunkSize int64) (int64, error) {
	if chunkSize <= 0 {
		chunkSize = size
	}
	for offset < size {
		length := chunkSize
		if remaining := size - offset; remaining < length {
			length = remaining
		}
		status, err := c.Patch(ctx, location, offset, io.LimitReader(body, length), length)
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

func (c *Client) baseURL(path string) (string, error) {
	if c.filerURL == "" {
		return "", fmt.Errorf("tus: filer url is required")
	}
	escaped, err := escapePath(path)
	if err != nil {
		return "", err
	}
	return c.filerURL + c.basePath + escaped, nil
}

func (c *Client) uploadURL(location string) (string, error) {
	if location == "" {
		return "", fmt.Errorf("tus: location is required")
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}
	return c.resolveLocation(location)
}

func (c *Client) resolveLocation(location string) (string, error) {
	if location == "" {
		return "", fmt.Errorf("tus: location is empty")
	}
	if c.filerURL == "" {
		return "", fmt.Errorf("tus: filer url is required")
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}
	if !strings.HasPrefix(location, "/") {
		return "", fmt.Errorf("tus: relative location must start with /")
	}
	return c.filerURL + location, nil
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
		return 0, fmt.Errorf("tus: missing integer header")
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

func escapePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" || path == "/" {
		return "/", nil
	}
	hasTrailingSlash := strings.HasSuffix(path, "/")
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "/", nil
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
	return builder.String(), nil
}
