package filer

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
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/lingjhf/seaweed/internal/httpx"
)

// Config configures a filer client.
type Config struct {
	BaseURLs       []string
	HTTPClient     *http.Client
	UserAgent      string
	BearerToken    string
	Retry          RetryPolicy
	EndpointPolicy EndpointPolicy
}

// RetryPolicy controls retry attempts for retryable filer requests.
type RetryPolicy = httpx.RetryPolicy

// EndpointPolicy controls how the client chooses among filer endpoints.
type EndpointPolicy = httpx.EndpointPolicy

// Client calls SeaweedFS filer HTTP APIs.
type Client struct {
	endpoints *httpx.EndpointSet
	http      *httpx.Client
}

// WriteOptions configures Put requests to the filer.
type WriteOptions struct {
	DataCenter         string
	Rack               string
	DataNode           string
	Collection         string
	Replication        string
	TTL                string
	MaxMB              int
	Mode               string
	Offset             *int64
	Fsync              bool
	SaveInside         bool
	SkipCheckParentDir bool
	ContentType        string
	ContentDisposition string
	SeaweedHeaders     map[string]string
	ContentLength      int64
	Authorization      string
}

// AppendOptions configures Append requests to the filer.
type AppendOptions struct {
	DataCenter         string
	Rack               string
	DataNode           string
	Collection         string
	Replication        string
	TTL                string
	MaxMB              int
	Mode               string
	Fsync              bool
	SaveInside         bool
	SkipCheckParentDir bool
	ContentType        string
	ContentDisposition string
	SeaweedHeaders     map[string]string
	ContentLength      int64
	Authorization      string
}

// MultipartUploadOptions configures UploadMultipart requests to the filer.
type MultipartUploadOptions struct {
	DataCenter         string
	Rack               string
	DataNode           string
	Collection         string
	Replication        string
	TTL                string
	MaxMB              int
	Mode               string
	Fsync              bool
	SaveInside         bool
	SkipCheckParentDir bool
	Filename           string
	FileContentType    string
	FieldName          string
	SeaweedHeaders     map[string]string
	Authorization      string
}

// WriteResult is returned by successful Put, Append, and UploadMultipart requests.
type WriteResult struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	ETag string `json:"eTag"`
}

// HeadResult contains filer response headers and parsed SeaweedFS tags.
type HeadResult struct {
	Header http.Header
	Tags   map[string]string
}

// GetOptions configures a filer Get request.
type GetOptions struct {
	ResponseContentDisposition string
	ResolveManifest            bool
	Authorization              string
}

// HeadOptions configures a filer Head request.
type HeadOptions struct {
	Authorization string
}

// StatOptions configures a filer metadata request.
type StatOptions struct {
	ResolveManifest bool
	Authorization   string
}

// ListOptions configures one filer listing page.
type ListOptions struct {
	Limit              int
	LastFileName       string
	NamePattern        string
	NamePatternExclude string
	Authorization      string
}

// WalkOptions configures paginated directory walking.
type WalkOptions struct {
	Limit              int
	LastFileName       string
	NamePattern        string
	NamePatternExclude string
	Authorization      string
}

// DeleteOptions configures a filer Delete request.
type DeleteOptions struct {
	Recursive            bool
	IgnoreRecursiveError bool
	SkipChunkDeletion    bool
	Authorization        string
}

// CopyOptions configures a filer Copy request.
type CopyOptions struct {
	Authorization string
}

// MoveOptions configures a filer Move request.
type MoveOptions struct {
	Authorization string
}

// TagOptions configures filer tag requests.
type TagOptions struct {
	Authorization string
}

// MkdirOptions configures a filer Mkdir request.
type MkdirOptions struct {
	Authorization string
}

// ListPage is one page returned by a filer directory listing.
type ListPage struct {
	Path                  string  `json:"Path"`
	Entries               []Entry `json:"Entries"`
	Limit                 int     `json:"Limit"`
	LastFileName          string  `json:"LastFileName"`
	ShouldDisplayLoadMore bool    `json:"ShouldDisplayLoadMore"`
}

// Entry describes a filer file or directory.
type Entry struct {
	FullPath        string            `json:"FullPath"`
	Mtime           time.Time         `json:"Mtime"`
	Crtime          time.Time         `json:"Crtime"`
	Mode            int64             `json:"Mode"`
	Mime            string            `json:"Mime"`
	Replication     string            `json:"Replication"`
	Collection      string            `json:"Collection"`
	TtlSec          int64             `json:"TtlSec"`
	DiskType        string            `json:"DiskType"`
	UserName        string            `json:"UserName"`
	GroupNames      []string          `json:"GroupNames"`
	UID             int64             `json:"Uid"`
	GID             int64             `json:"Gid"`
	SymlinkTarget   string            `json:"SymlinkTarget"`
	MD5             string            `json:"Md5"`
	FileSize        int64             `json:"FileSize"`
	Rdev            int64             `json:"Rdev"`
	Inode           uint64            `json:"Inode"`
	Extended        map[string][]byte `json:"Extended"`
	Content         []byte            `json:"Content"`
	Chunks          []Chunk           `json:"chunks"`
	HardLinkID      string            `json:"HardLinkId"`
	HardLinkCounter int64             `json:"HardLinkCounter"`
	Remote          any               `json:"Remote"`
	Quota           int64             `json:"Quota"`
}

// Chunk describes one chunk in a filer entry.
type Chunk struct {
	FileID       string `json:"file_id"`
	Size         int64  `json:"size"`
	Mtime        int64  `json:"mtime"`
	ETag         string `json:"e_tag"`
	FID          FID    `json:"fid"`
	IsCompressed bool   `json:"is_compressed"`
	IsGzipped    bool   `json:"is_gzipped"`
}

// FID describes the structured file ID attached to a filer chunk.
type FID struct {
	VolumeID int64  `json:"volume_id"`
	FileKey  uint64 `json:"file_key"`
	Cookie   uint32 `json:"cookie"`
}

// New creates a filer client.
func New(config Config) (*Client, error) {
	if len(config.BaseURLs) == 0 {
		return nil, errors.New("filer: base urls are required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	endpoints, err := httpx.NewEndpointSetWithPolicy(config.BaseURLs, config.EndpointPolicy)
	if err != nil {
		return nil, fmt.Errorf("filer: invalid base urls: %w", err)
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
	client.endpoints.StartHealthCheck(config.HTTPClient, http.MethodGet, "/")
	return client, nil
}

// Put writes body to path through the filer.
func (c *Client) Put(ctx context.Context, path string, body io.Reader, opts WriteOptions) (*WriteResult, error) {
	return c.write(ctx, path, body, opts, "")
}

// Append appends body to path through the filer.
func (c *Client) Append(ctx context.Context, path string, body io.Reader, opts AppendOptions) (*WriteResult, error) {
	return c.write(ctx, path, body, writeOptionsFromAppend(opts), "append")
}

// UploadMultipart uploads body to targetPath using a streaming multipart form.
func (c *Client) UploadMultipart(ctx context.Context, targetPath string, body io.Reader, opts MultipartUploadOptions) (*WriteResult, error) {
	if body == nil {
		return nil, errors.New("filer: body is required")
	}
	filename, err := multipartFilename(targetPath, opts.Filename)
	if err != nil {
		return nil, err
	}
	resourcePath, err := c.resourcePath(targetPath)
	if err != nil {
		return nil, err
	}

	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	go writeMultipartBody(writer, multipartWriter, filename, body, opts)

	header := http.Header{}
	header.Set("Content-Type", multipartWriter.FormDataContentType())
	addHeader(header, "Authorization", opts.Authorization)
	addSeaweedHeaders(header, opts.SeaweedHeaders)

	var out WriteResult
	err = c.http.DecodeJSONEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodPost,
		Query:         multipartQuery(opts),
		Header:        header,
		Body:          reader,
		ContentLength: -1,
	}, &out)
	return &out, err
}

func (c *Client) write(ctx context.Context, path string, body io.Reader, opts WriteOptions, op string) (*WriteResult, error) {
	resourcePath, err := c.resourcePath(path)
	if err != nil {
		return nil, err
	}
	query := putQuery(opts)
	httpx.AddString(query, "op", op)

	var out WriteResult
	err = c.http.DecodeJSONEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodPut,
		Query:         query,
		Header:        putHeader(opts),
		Body:          body,
		ContentLength: opts.ContentLength,
	}, &out)
	return &out, err
}

// Copy copies srcPath to dstPath through the filer.
func (c *Client) Copy(ctx context.Context, srcPath string, dstPath string, opts CopyOptions) error {
	resourcePath, err := c.resourcePath(dstPath)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("cp.from", srcPath)
	header := http.Header{}
	addHeader(header, "Authorization", opts.Authorization)
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodPost,
		Query:         query,
		Header:        header,
		ContentLength: 0,
	}, http.StatusOK, http.StatusCreated, http.StatusNoContent)
}

// Move moves srcPath to dstPath through the filer.
func (c *Client) Move(ctx context.Context, srcPath string, dstPath string, opts MoveOptions) error {
	resourcePath, err := c.resourcePath(dstPath)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("mv.from", srcPath)
	header := http.Header{}
	addHeader(header, "Authorization", opts.Authorization)
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodPost,
		Query:         query,
		Header:        header,
		ContentLength: 0,
	}, http.StatusOK, http.StatusCreated, http.StatusNoContent)
}

// SetTags writes SeaweedFS filer tags for path.
func (c *Client) SetTags(ctx context.Context, path string, tags map[string]string, opts TagOptions) error {
	resourcePath, err := c.resourcePath(path)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("tagging", "")
	header := http.Header{}
	addHeader(header, "Authorization", opts.Authorization)
	addSeaweedHeaders(header, tags)
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodPut,
		Query:         query,
		Header:        header,
		ContentLength: 0,
	}, http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent)
}

// DeleteTags deletes SeaweedFS filer tag keys from path.
func (c *Client) DeleteTags(ctx context.Context, path string, opts TagOptions, keys ...string) error {
	resourcePath, err := c.resourcePath(path)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("tagging", strings.Join(keys, ","))
	header := http.Header{}
	addHeader(header, "Authorization", opts.Authorization)
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodDelete,
		Query:         query,
		Header:        header,
		ContentLength: -1,
	}, http.StatusOK, http.StatusAccepted, http.StatusNoContent)
}

// Mkdir creates a directory path through the filer.
func (c *Client) Mkdir(ctx context.Context, path string, opts MkdirOptions) error {
	resourcePath, err := c.resourcePath(ensureTrailingSlash(path))
	if err != nil {
		return err
	}
	header := http.Header{}
	addHeader(header, "Authorization", opts.Authorization)
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodPost,
		Header:        header,
		ContentLength: 0,
	}, http.StatusOK, http.StatusCreated, http.StatusNoContent)
}

// Get returns the file content response for path.
func (c *Client) Get(ctx context.Context, path string, opts GetOptions) (*http.Response, error) {
	resourcePath, err := c.resourcePath(path)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	httpx.AddString(query, "response-content-disposition", opts.ResponseContentDisposition)
	addBool(query, "resolveManifest", opts.ResolveManifest)
	header := http.Header{}
	addHeader(header, "Authorization", opts.Authorization)

	resp, err := c.http.DoEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodGet,
		Query:         query,
		Header:        header,
		ContentLength: -1,
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, httpx.ResponseError(http.MethodGet, resp.Request.URL.String(), resp)
	}
	return resp, nil
}

// Head returns headers and parsed SeaweedFS tags for path.
func (c *Client) Head(ctx context.Context, path string, opts HeadOptions) (*HeadResult, error) {
	resourcePath, err := c.resourcePath(path)
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	addHeader(header, "Authorization", opts.Authorization)
	resp, err := c.http.DoEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodHead,
		Header:        header,
		ContentLength: -1,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, httpx.ResponseError(http.MethodHead, resp.Request.URL.String(), resp)
	}
	resultHeader := resp.Header.Clone()
	return &HeadResult{
		Header: resultHeader,
		Tags:   seaweedTags(resultHeader),
	}, nil
}

// Tags returns parsed SeaweedFS tags for path.
func (c *Client) Tags(ctx context.Context, path string, opts HeadOptions) (map[string]string, error) {
	head, err := c.Head(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	return head.Tags, nil
}

// Stat returns filer metadata for path.
func (c *Client) Stat(ctx context.Context, path string, opts StatOptions) (*Entry, error) {
	resourcePath, err := c.resourcePath(path)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("metadata", "true")
	addBool(query, "resolveManifest", opts.ResolveManifest)
	header := http.Header{}
	addHeader(header, "Authorization", opts.Authorization)

	var out Entry
	err = c.http.DecodeJSONEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodGet,
		Query:         query,
		Header:        header,
		ContentLength: -1,
	}, &out)
	return &out, err
}

// Walk calls fn for entries under path, following filer pagination until done.
func (c *Client) Walk(ctx context.Context, path string, opts WalkOptions, fn func(Entry) error) error {
	if fn == nil {
		return errors.New("filer: walk callback is required")
	}
	lastFileName := opts.LastFileName
	for {
		page, err := c.ListPage(ctx, path, ListOptions{
			Limit:              opts.Limit,
			LastFileName:       lastFileName,
			NamePattern:        opts.NamePattern,
			NamePatternExclude: opts.NamePatternExclude,
			Authorization:      opts.Authorization,
		})
		if err != nil {
			return err
		}
		for _, entry := range page.Entries {
			if err := fn(entry); err != nil {
				return err
			}
		}
		if !page.ShouldDisplayLoadMore || len(page.Entries) == 0 {
			return nil
		}
		if page.LastFileName == "" {
			return errors.New("filer: list page missing last file name")
		}
		if page.LastFileName == lastFileName {
			return fmt.Errorf("filer: list page repeated last file name %q", page.LastFileName)
		}
		lastFileName = page.LastFileName
	}
}

// ListPage returns one filer listing page for path.
func (c *Client) ListPage(ctx context.Context, path string, opts ListOptions) (*ListPage, error) {
	resourcePath, err := c.resourcePath(ensureTrailingSlash(path))
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	httpx.AddInt(query, "limit", opts.Limit)
	httpx.AddString(query, "lastFileName", opts.LastFileName)
	httpx.AddString(query, "namePattern", opts.NamePattern)
	httpx.AddString(query, "namePatternExclude", opts.NamePatternExclude)
	header := http.Header{
		"Accept": []string{"application/json"},
	}
	addHeader(header, "Authorization", opts.Authorization)

	var out ListPage
	err = c.http.DecodeJSONEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodGet,
		Query:         query,
		Header:        header,
		ContentLength: -1,
	}, &out)
	return &out, err
}

// Delete removes path from the filer.
func (c *Client) Delete(ctx context.Context, path string, opts DeleteOptions) error {
	resourcePath, err := c.resourcePath(path)
	if err != nil {
		return err
	}
	query := url.Values{}
	addBool(query, "recursive", opts.Recursive)
	addBool(query, "ignoreRecursiveError", opts.IgnoreRecursiveError)
	addBool(query, "skipChunkDeletion", opts.SkipChunkDeletion)
	header := http.Header{}
	addHeader(header, "Authorization", opts.Authorization)
	return c.http.CheckStatusEndpoint(ctx, c.endpoints, resourcePath, httpx.Request{
		Method:        http.MethodDelete,
		Query:         query,
		Header:        header,
		ContentLength: -1,
	}, http.StatusOK, http.StatusAccepted, http.StatusNoContent)
}

func (c *Client) resourcePath(path string) (string, error) {
	escapedPath, err := escapePath(path)
	if err != nil {
		return "", err
	}
	return escapedPath, nil
}

func putQuery(opts WriteOptions) url.Values {
	query := url.Values{}
	httpx.AddString(query, "dataCenter", opts.DataCenter)
	httpx.AddString(query, "rack", opts.Rack)
	httpx.AddString(query, "dataNode", opts.DataNode)
	httpx.AddString(query, "collection", opts.Collection)
	httpx.AddString(query, "replication", opts.Replication)
	httpx.AddString(query, "ttl", opts.TTL)
	httpx.AddInt(query, "maxMB", opts.MaxMB)
	httpx.AddString(query, "mode", opts.Mode)
	if opts.Offset != nil {
		query.Set("offset", strconv.FormatInt(*opts.Offset, 10))
	}
	addBool(query, "fsync", opts.Fsync)
	addBool(query, "saveInside", opts.SaveInside)
	addBool(query, "skipCheckParentDir", opts.SkipCheckParentDir)
	return query
}

func multipartQuery(opts MultipartUploadOptions) url.Values {
	return putQuery(WriteOptions{
		DataCenter:         opts.DataCenter,
		Rack:               opts.Rack,
		DataNode:           opts.DataNode,
		Collection:         opts.Collection,
		Replication:        opts.Replication,
		TTL:                opts.TTL,
		MaxMB:              opts.MaxMB,
		Mode:               opts.Mode,
		Fsync:              opts.Fsync,
		SaveInside:         opts.SaveInside,
		SkipCheckParentDir: opts.SkipCheckParentDir,
	})
}

func putHeader(opts WriteOptions) http.Header {
	header := http.Header{}
	addHeader(header, "Content-Type", opts.ContentType)
	addHeader(header, "Content-Disposition", opts.ContentDisposition)
	addHeader(header, "Authorization", opts.Authorization)
	addSeaweedHeaders(header, opts.SeaweedHeaders)
	return header
}

func addSeaweedHeaders(header http.Header, values map[string]string) {
	for key, value := range values {
		header.Set("Seaweed-"+strings.TrimPrefix(key, "Seaweed-"), value)
	}
}

func writeOptionsFromAppend(opts AppendOptions) WriteOptions {
	return WriteOptions{
		DataCenter:         opts.DataCenter,
		Rack:               opts.Rack,
		DataNode:           opts.DataNode,
		Collection:         opts.Collection,
		Replication:        opts.Replication,
		TTL:                opts.TTL,
		MaxMB:              opts.MaxMB,
		Mode:               opts.Mode,
		Fsync:              opts.Fsync,
		SaveInside:         opts.SaveInside,
		SkipCheckParentDir: opts.SkipCheckParentDir,
		ContentType:        opts.ContentType,
		ContentDisposition: opts.ContentDisposition,
		SeaweedHeaders:     opts.SeaweedHeaders,
		ContentLength:      opts.ContentLength,
		Authorization:      opts.Authorization,
	}
}

func multipartFilename(targetPath string, configured string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		return configured, nil
	}
	if strings.HasSuffix(targetPath, "/") {
		return "", errors.New("filer: filename is required")
	}
	filename := path.Base(strings.TrimRight(targetPath, "/"))
	if strings.TrimSpace(filename) == "" || filename == "." || filename == "/" {
		return "", errors.New("filer: filename is required")
	}
	return filename, nil
}

func writeMultipartBody(pipeWriter *io.PipeWriter, multipartWriter *multipart.Writer, filename string, body io.Reader, opts MultipartUploadOptions) {
	fieldName := opts.FieldName
	if strings.TrimSpace(fieldName) == "" {
		fieldName = "file"
	}
	contentType := opts.FileContentType
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

func seaweedTags(header http.Header) map[string]string {
	tags := map[string]string{}
	for key, values := range header {
		if len(values) == 0 || !strings.HasPrefix(key, "Seaweed-") {
			continue
		}
		tags[strings.TrimPrefix(key, "Seaweed-")] = values[0]
	}
	return tags
}

func addHeader(header http.Header, key string, value string) {
	if value != "" {
		header.Set(key, value)
	}
}

func addBool(query url.Values, key string, value bool) {
	if value {
		query.Set(key, "true")
	}
}

func escapePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("filer: path is required")
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

func ensureTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	}
	return path + "/"
}

// Close stops background endpoint health checks.
func (c *Client) Close() {
	c.endpoints.Close()
}
