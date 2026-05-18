package filer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/lingjhf/seaweed/internal/httpx"
)

type Config struct {
	BaseURL     string
	HTTPClient  *http.Client
	UserAgent   string
	BearerToken string
	Retry       RetryPolicy
}

type RetryPolicy = httpx.RetryPolicy

type Client struct {
	baseURL string
	http    *httpx.Client
}

type PutOptions struct {
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
}

type WriteResponse struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	ETag  string `json:"eTag"`
	Error string `json:"error,omitempty"`
}

type GetOptions struct {
	ResponseContentDisposition string
	ResolveManifest            bool
}

type StatOptions struct {
	ResolveManifest bool
}

type ListOptions struct {
	Limit              int
	LastFileName       string
	NamePattern        string
	NamePatternExclude string
}

type DeleteOptions struct {
	Recursive            bool
	IgnoreRecursiveError bool
	SkipChunkDeletion    bool
}

type ListResponse struct {
	Path                  string  `json:"Path"`
	Entries               []Entry `json:"Entries"`
	Limit                 int     `json:"Limit"`
	LastFileName          string  `json:"LastFileName"`
	ShouldDisplayLoadMore bool    `json:"ShouldDisplayLoadMore"`
}

type Entry struct {
	FullPath        string            `json:"FullPath"`
	Mode            int64             `json:"Mode"`
	Mime            string            `json:"Mime"`
	Replication     string            `json:"Replication"`
	Collection      string            `json:"Collection"`
	TtlSec          int64             `json:"TtlSec"`
	UserName        string            `json:"UserName"`
	GroupNames      []string          `json:"GroupNames"`
	SymlinkTarget   string            `json:"SymlinkTarget"`
	FileSize        int64             `json:"FileSize"`
	Extended        map[string][]byte `json:"Extended"`
	Content         []byte            `json:"Content"`
	Chunks          []Chunk           `json:"chunks"`
	HardLinkID      string            `json:"HardLinkId"`
	HardLinkCounter int64             `json:"HardLinkCounter"`
}

type Chunk struct {
	FileID       string `json:"file_id"`
	Size         int64  `json:"size"`
	Mtime        int64  `json:"mtime"`
	ETag         string `json:"e_tag"`
	IsCompressed bool   `json:"is_compressed"`
	IsGzipped    bool   `json:"is_gzipped"`
}

func New(config Config) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("filer: base url is required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Client{
		baseURL: config.BaseURL,
		http: httpx.NewClient(httpx.Config{
			HTTPClient:  config.HTTPClient,
			UserAgent:   config.UserAgent,
			BearerToken: config.BearerToken,
			Retry:       config.Retry,
		}),
	}, nil
}

func (c *Client) Put(ctx context.Context, path string, body io.Reader, opts PutOptions) (*WriteResponse, error) {
	return c.write(ctx, path, body, opts, "")
}

func (c *Client) Append(ctx context.Context, path string, body io.Reader, opts PutOptions) (*WriteResponse, error) {
	if opts.Offset != nil {
		return nil, fmt.Errorf("filer: append is incompatible with offset")
	}
	return c.write(ctx, path, body, opts, "append")
}

func (c *Client) write(ctx context.Context, path string, body io.Reader, opts PutOptions, op string) (*WriteResponse, error) {
	rawURL, err := c.resourceURL(path)
	if err != nil {
		return nil, err
	}
	query := putQuery(opts)
	httpx.AddString(query, "op", op)

	var out WriteResponse
	err = c.http.DecodeJSON(ctx, httpx.Request{
		Method:        http.MethodPut,
		URL:           rawURL,
		Query:         query,
		Header:        putHeader(opts),
		Body:          body,
		ContentLength: opts.ContentLength,
	}, &out)
	return &out, err
}

func (c *Client) Copy(ctx context.Context, srcPath string, dstPath string) error {
	rawURL, err := c.resourceURL(dstPath)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("cp.from", srcPath)
	return c.http.CheckStatus(ctx, httpx.Request{
		Method:        http.MethodPost,
		URL:           rawURL,
		Query:         query,
		ContentLength: 0,
	}, http.StatusOK, http.StatusCreated, http.StatusNoContent)
}

func (c *Client) Move(ctx context.Context, srcPath string, dstPath string) error {
	rawURL, err := c.resourceURL(dstPath)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("mv.from", srcPath)
	return c.http.CheckStatus(ctx, httpx.Request{
		Method:        http.MethodPost,
		URL:           rawURL,
		Query:         query,
		ContentLength: 0,
	}, http.StatusOK, http.StatusCreated, http.StatusNoContent)
}

func (c *Client) SetTags(ctx context.Context, path string, tags map[string]string) error {
	rawURL, err := c.resourceURL(path)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("tagging", "")
	header := http.Header{}
	for key, value := range tags {
		header.Set("Seaweed-"+strings.TrimPrefix(key, "Seaweed-"), value)
	}
	return c.http.CheckStatus(ctx, httpx.Request{
		Method:        http.MethodPut,
		URL:           rawURL,
		Query:         query,
		Header:        header,
		ContentLength: 0,
	}, http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent)
}

func (c *Client) DeleteTags(ctx context.Context, path string, keys ...string) error {
	rawURL, err := c.resourceURL(path)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("tagging", strings.Join(keys, ","))
	return c.http.CheckStatus(ctx, httpx.Request{
		Method:        http.MethodDelete,
		URL:           rawURL,
		Query:         query,
		ContentLength: -1,
	}, http.StatusOK, http.StatusAccepted, http.StatusNoContent)
}

func (c *Client) Mkdir(ctx context.Context, path string) error {
	rawURL, err := c.resourceURL(ensureTrailingSlash(path))
	if err != nil {
		return err
	}
	return c.http.CheckStatus(ctx, httpx.Request{
		Method:        http.MethodPost,
		URL:           rawURL,
		ContentLength: 0,
	}, http.StatusOK, http.StatusCreated, http.StatusNoContent)
}

func (c *Client) Get(ctx context.Context, path string, opts GetOptions) (*http.Response, error) {
	rawURL, err := c.resourceURL(path)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	httpx.AddString(query, "response-content-disposition", opts.ResponseContentDisposition)
	addBool(query, "resolveManifest", opts.ResolveManifest)

	resp, err := c.http.Do(ctx, httpx.Request{
		Method:        http.MethodGet,
		URL:           rawURL,
		Query:         query,
		ContentLength: -1,
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer resp.Body.Close()
		return nil, httpx.ResponseError(http.MethodGet, rawURL, resp)
	}
	return resp, nil
}

func (c *Client) Head(ctx context.Context, path string) (http.Header, error) {
	rawURL, err := c.resourceURL(path)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(ctx, httpx.Request{
		Method:        http.MethodHead,
		URL:           rawURL,
		ContentLength: -1,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, httpx.ResponseError(http.MethodHead, rawURL, resp)
	}
	return resp.Header.Clone(), nil
}

func (c *Client) Stat(ctx context.Context, path string, opts StatOptions) (*Entry, error) {
	rawURL, err := c.resourceURL(path)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("metadata", "true")
	addBool(query, "resolveManifest", opts.ResolveManifest)

	var out Entry
	err = c.http.DecodeJSON(ctx, httpx.Request{
		Method:        http.MethodGet,
		URL:           rawURL,
		Query:         query,
		ContentLength: -1,
	}, &out)
	return &out, err
}

func (c *Client) List(ctx context.Context, path string, opts ListOptions) (*ListResponse, error) {
	rawURL, err := c.resourceURL(ensureTrailingSlash(path))
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	httpx.AddInt(query, "limit", opts.Limit)
	httpx.AddString(query, "lastFileName", opts.LastFileName)
	httpx.AddString(query, "namePattern", opts.NamePattern)
	httpx.AddString(query, "namePatternExclude", opts.NamePatternExclude)

	var out ListResponse
	err = c.http.DecodeJSON(ctx, httpx.Request{
		Method: http.MethodGet,
		URL:    rawURL,
		Query:  query,
		Header: http.Header{
			"Accept": []string{"application/json"},
		},
		ContentLength: -1,
	}, &out)
	return &out, err
}

func (c *Client) Delete(ctx context.Context, path string, opts DeleteOptions) error {
	rawURL, err := c.resourceURL(path)
	if err != nil {
		return err
	}
	query := url.Values{}
	addBool(query, "recursive", opts.Recursive)
	addBool(query, "ignoreRecursiveError", opts.IgnoreRecursiveError)
	addBool(query, "skipChunkDeletion", opts.SkipChunkDeletion)
	return c.http.CheckStatus(ctx, httpx.Request{
		Method:        http.MethodDelete,
		URL:           rawURL,
		Query:         query,
		ContentLength: -1,
	}, http.StatusOK, http.StatusAccepted, http.StatusNoContent)
}

func (c *Client) resourceURL(path string) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("filer: base url is required")
	}
	escapedPath, err := escapePath(path)
	if err != nil {
		return "", err
	}
	return c.baseURL + escapedPath, nil
}

func putQuery(opts PutOptions) url.Values {
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

func putHeader(opts PutOptions) http.Header {
	header := http.Header{}
	addHeader(header, "Content-Type", opts.ContentType)
	addHeader(header, "Content-Disposition", opts.ContentDisposition)
	for key, value := range opts.SeaweedHeaders {
		header.Set("Seaweed-"+strings.TrimPrefix(key, "Seaweed-"), value)
	}
	return header
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
		return "", fmt.Errorf("filer: path is required")
	}
	hasTrailingSlash := strings.HasSuffix(path, "/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		escaped = append(escaped, url.PathEscape(part))
	}
	if len(escaped) == 0 {
		return "/", nil
	}
	out := "/" + strings.Join(escaped, "/")
	if hasTrailingSlash {
		out += "/"
	}
	return out, nil
}

func ensureTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	}
	return path + "/"
}
