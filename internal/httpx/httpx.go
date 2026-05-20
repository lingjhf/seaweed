package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// RetryPolicy controls retry attempts for retryable HTTP methods.
type RetryPolicy struct {
	MaxAttempts int
	Wait        time.Duration
}

// Config configures a shared HTTP API client.
type Config struct {
	HTTPClient  *http.Client
	UserAgent   string
	BearerToken string
	Retry       RetryPolicy
}

// Client wraps an HTTP client with common SeaweedFS request behavior.
type Client struct {
	httpClient  *http.Client
	userAgent   string
	bearerToken string
	retry       RetryPolicy
}

// Request describes one HTTP request.
type Request struct {
	Method        string
	URL           string
	Query         url.Values
	Header        http.Header
	Body          io.Reader
	ContentLength int64
}

// Error describes a non-success HTTP response.
type Error struct {
	Method     string
	URL        string
	StatusCode int
	Header     http.Header
	Body       string
}

// APIError describes an API-level error returned in a successful JSON response.
type APIError struct {
	Method  string
	URL     string
	Message string
}

// Error formats the HTTP response error.
func (e *Error) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s %s: unexpected status %d", e.Method, e.URL, e.StatusCode)
	}
	return fmt.Sprintf("%s %s: unexpected status %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
}

// Error formats the API-level response error.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: api error: %s", e.Method, e.URL, e.Message)
}

// NewClient creates a shared HTTP API client.
func NewClient(config Config) *Client {
	return &Client{
		httpClient:  config.HTTPClient,
		userAgent:   config.UserAgent,
		bearerToken: config.BearerToken,
		retry:       config.Retry,
	}
}

// Do sends request through the configured HTTP client.
func (c *Client) Do(ctx context.Context, request Request) (*http.Response, error) {
	if request.Method == "" {
		return nil, fmt.Errorf("httpx: method is required")
	}
	if request.URL == "" {
		return nil, fmt.Errorf("httpx: url is required")
	}
	attempts := c.retry.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := c.newHTTPRequest(ctx, request)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(req)
		if err == nil && !shouldRetryResponse(req.Method, resp.StatusCode) {
			return resp, nil
		}
		if err != nil {
			lastErr = err
		}
		if attempt == attempts || !isRetryableMethod(request.Method) {
			if err != nil {
				return nil, err
			}
			return resp, nil
		}
		if resp != nil {
			resp.Body.Close()
		}

		timer := time.NewTimer(c.retry.Wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

// DoEndpoint sends request to an endpoint selected from endpoints.
func (c *Client) DoEndpoint(ctx context.Context, endpoints *EndpointSet, path string, request Request) (*http.Response, error) {
	if endpoints == nil {
		return nil, fmt.Errorf("httpx: endpoints are required")
	}
	candidates := endpoints.Candidates(path)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("httpx: no available endpoints")
	}
	if !isRetryableMethod(request.Method) {
		candidates = candidates[:1]
	}

	var lastErr error
	for i, candidate := range candidates {
		available, halfOpen := endpoints.beginCandidate(candidate.Index)
		if !available {
			continue
		}
		request.URL = candidate.URL
		resp, err := c.Do(ctx, request)
		if err == nil && !isEndpointFailureResponse(resp.StatusCode) {
			endpoints.finishCandidate(candidate.Index, halfOpen, true)
			return resp, nil
		}
		endpoints.finishCandidate(candidate.Index, halfOpen, false)
		if err == nil && !shouldRetryResponse(request.Method, resp.StatusCode) {
			return resp, nil
		}
		if i == len(candidates)-1 {
			if err != nil {
				return nil, err
			}
			return resp, nil
		}
		if err != nil {
			lastErr = err
		} else {
			resp.Body.Close()
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if lastErr == nil {
		return nil, fmt.Errorf("httpx: no available endpoints")
	}
	return nil, lastErr
}

// DecodeJSON sends request and decodes a successful JSON response into out.
func (c *Client) DecodeJSON(ctx context.Context, request Request, out any) error {
	resp, err := c.Do(ctx, request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ResponseError(request.Method, responseURL(resp, request.URL), resp)
	}
	return decodeJSONResponse(resp, request, out)
}

// DecodeJSONEndpoint sends request through endpoints and decodes JSON into out.
func (c *Client) DecodeJSONEndpoint(ctx context.Context, endpoints *EndpointSet, path string, request Request, out any) error {
	_, err := c.DecodeJSONEndpointWithResponse(ctx, endpoints, path, request, out)
	return err
}

// DecodeJSONEndpointWithResponse sends request through endpoints, decodes JSON
// into out, and returns the response for callers that need response headers.
func (c *Client) DecodeJSONEndpointWithResponse(ctx context.Context, endpoints *EndpointSet, path string, request Request, out any) (*http.Response, error) {
	resp, err := c.DoEndpoint(ctx, endpoints, path, request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp, ResponseError(request.Method, responseURL(resp, request.URL), resp)
	}
	return resp, decodeJSONResponse(resp, request, out)
}

// CheckStatus sends request and requires one of the expected response statuses.
func (c *Client) CheckStatus(ctx context.Context, request Request, expected ...int) error {
	resp, err := c.Do(ctx, request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if slices.Contains(expected, resp.StatusCode) {
		return checkStatusResponse(resp, request)
	}
	return ResponseError(request.Method, responseURL(resp, request.URL), resp)
}

// CheckStatusEndpoint sends request through endpoints and requires an expected status.
func (c *Client) CheckStatusEndpoint(ctx context.Context, endpoints *EndpointSet, path string, request Request, expected ...int) error {
	resp, err := c.DoEndpoint(ctx, endpoints, path, request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if slices.Contains(expected, resp.StatusCode) {
		return checkStatusResponse(resp, request)
	}
	return ResponseError(request.Method, responseURL(resp, request.URL), resp)
}

func (c *Client) newHTTPRequest(ctx context.Context, request Request) (*http.Request, error) {
	rawURL := request.URL
	if len(request.Query) > 0 {
		separator := "?"
		if strings.Contains(rawURL, "?") {
			separator = "&"
		}
		rawURL += separator + request.Query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, request.Method, rawURL, request.Body)
	if err != nil {
		return nil, err
	}
	if request.ContentLength >= 0 {
		req.ContentLength = request.ContentLength
	}
	for key, values := range request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.bearerToken != "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
	return req, nil
}

func responseURL(resp *http.Response, fallback string) string {
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String()
	}
	return fallback
}

// ResponseError reads resp and returns an Error.
func ResponseError(method, rawURL string, resp *http.Response) error {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if readErr != nil {
		body = []byte("failed to read response body: " + readErr.Error())
	}
	return &Error{
		Method:     method,
		URL:        rawURL,
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       strings.TrimSpace(string(body)),
	}
}

func decodeJSONResponse(resp *http.Response, request Request, out any) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		if out == nil {
			return nil
		}
		return fmt.Errorf("decode response: %w", io.EOF)
	}
	if apiErr := apiErrorFromBody(resp, request, body); apiErr != nil {
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func checkStatusResponse(resp *http.Response, request Request) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if apiErr := apiErrorFromBody(resp, request, bytes.TrimSpace(body)); apiErr != nil {
		return apiErr
	}
	return nil
}

func apiErrorFromBody(resp *http.Response, request Request, body []byte) *APIError {
	if len(body) == 0 {
		return nil
	}
	var apiError struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Error != "" {
		return &APIError{
			Method:  request.Method,
			URL:     responseURL(resp, request.URL),
			Message: apiError.Error,
		}
	}
	return nil
}

// IsHTTPStatus reports whether err is an Error with a status in [min, max].
func IsHTTPStatus(err error, min int, max int) bool {
	httpErr, ok := err.(*Error)
	if !ok {
		return false
	}
	return httpErr.StatusCode >= min && httpErr.StatusCode <= max
}

func isRetryableMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func shouldRetryResponse(method string, status int) bool {
	if !isRetryableMethod(method) {
		return false
	}
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func isEndpointFailureResponse(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// AddInt adds key to query when value is non-zero.
func AddInt(query url.Values, key string, value int) {
	if value != 0 {
		query.Set(key, strconv.Itoa(value))
	}
}

// AddInt64 adds key to query when value is non-zero.
func AddInt64(query url.Values, key string, value int64) {
	if value != 0 {
		query.Set(key, strconv.FormatInt(value, 10))
	}
}

// AddFloat64 adds key to query when value is non-zero.
func AddFloat64(query url.Values, key string, value float64) {
	if value != 0 {
		query.Set(key, strconv.FormatFloat(value, 'f', -1, 64))
	}
}

// AddString adds key to query when value is not empty.
func AddString(query url.Values, key string, value string) {
	if value != "" {
		query.Set(key, value)
	}
}
