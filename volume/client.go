package volume

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

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

type PutOptions struct {
	ContentType     string
	ContentEncoding string
	ContentMD5      string
	Filename        string
	ContentLength   int64
}

type PutResponse struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	ETag  string `json:"eTag"`
	Error string `json:"error,omitempty"`
}

type GetOptions struct {
	Range string
}

func New(config Config) *Client {
	return &Client{
		baseURL: config.BaseURL,
		http:    config.HTTP,
	}
}

func (c *Client) Put(ctx context.Context, fileID string, body io.Reader, opts PutOptions) (*PutResponse, error) {
	rawURL, err := c.fileURL(fileID)
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	addHeader(header, "Content-Type", opts.ContentType)
	addHeader(header, "Content-Encoding", opts.ContentEncoding)
	addHeader(header, "Content-MD5", opts.ContentMD5)
	addHeader(header, "Content-Disposition", contentDisposition(opts.Filename))

	var out PutResponse
	err = c.http.DecodeJSON(ctx, httpx.Request{
		Method:        http.MethodPut,
		URL:           rawURL,
		Header:        header,
		Body:          body,
		ContentLength: opts.ContentLength,
	}, &out)
	return &out, err
}

func (c *Client) Get(ctx context.Context, fileID string, opts GetOptions) (*http.Response, error) {
	rawURL, err := c.fileURL(fileID)
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	addHeader(header, "Range", opts.Range)

	resp, err := c.http.Do(ctx, httpx.Request{
		Method:        http.MethodGet,
		URL:           rawURL,
		Header:        header,
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

func (c *Client) Head(ctx context.Context, fileID string) (http.Header, error) {
	rawURL, err := c.fileURL(fileID)
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

func (c *Client) Delete(ctx context.Context, fileID string) error {
	rawURL, err := c.fileURL(fileID)
	if err != nil {
		return err
	}
	return c.http.CheckStatus(ctx, httpx.Request{
		Method:        http.MethodDelete,
		URL:           rawURL,
		ContentLength: -1,
	}, http.StatusOK, http.StatusAccepted, http.StatusNoContent)
}

func (c *Client) Status(ctx context.Context) (map[string]any, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("volume: base url is required")
	}
	out := map[string]any{}
	err := c.http.DecodeJSON(ctx, httpx.Request{
		Method:        http.MethodGet,
		URL:           c.baseURL + "/status",
		ContentLength: -1,
	}, &out)
	return out, err
}

func (c *Client) Health(ctx context.Context) error {
	if c.baseURL == "" {
		return fmt.Errorf("volume: base url is required")
	}
	return c.http.CheckStatus(ctx, httpx.Request{
		Method:        http.MethodGet,
		URL:           c.baseURL + "/status",
		ContentLength: -1,
	}, http.StatusOK)
}

func (c *Client) fileURL(fileID string) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("volume: base url is required")
	}
	fileID = strings.TrimLeft(fileID, "/")
	if fileID == "" {
		return "", fmt.Errorf("volume: file id is required")
	}
	return c.baseURL + "/" + fileID, nil
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
