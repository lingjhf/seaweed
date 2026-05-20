package seaweed

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/lingjhf/seaweed/internal/httpx"
)

func TestS3EndpointResolverAddsPathStyleBucketToBasePath(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSet([]string{"http://example.test/base%2Fpath"})
	if err != nil {
		t.Fatalf("NewEndpointSet() error = %v", err)
	}
	resolved, err := s3EndpointResolver{endpoints: endpoints}.ResolveEndpoint(context.Background(), s3.EndpointParameters{
		Bucket: aws.String("bucket.with.dots"),
	})
	if err != nil {
		t.Fatalf("ResolveEndpoint() error = %v", err)
	}
	if resolved.URI.Path != "/base/path/bucket.with.dots" {
		t.Fatalf("Path = %q, want /base/path/bucket.with.dots", resolved.URI.Path)
	}
	if resolved.URI.RawPath != "/base%2Fpath/bucket.with.dots" {
		t.Fatalf("RawPath = %q, want /base%%2Fpath/bucket.with.dots", resolved.URI.RawPath)
	}
}

func TestS3EndpointResolverReturnsLeaseError(t *testing.T) {
	t.Parallel()

	if _, err := (s3EndpointResolver{}).ResolveEndpoint(context.Background(), s3.EndpointParameters{}); err == nil {
		t.Fatal("ResolveEndpoint() error = nil, want endpoint error")
	}
}

func TestResolveAWSEndpointErrors(t *testing.T) {
	t.Parallel()

	if _, err := resolveAWSEndpoint(nil); err == nil {
		t.Fatal("resolveAWSEndpoint(nil) error = nil, want error")
	}

	endpoints, err := httpx.NewEndpointSetWithPolicy([]string{"http://example.test"}, httpx.EndpointPolicy{
		CircuitBreaker: httpx.EndpointCircuitBreakerPolicy{
			Enabled:          true,
			FailureThreshold: 1,
			OpenTimeout:      time.Hour,
		},
	})
	if err != nil {
		t.Fatalf("NewEndpointSetWithPolicy() error = %v", err)
	}
	endpoints.RecordFailure(0)
	if _, err := resolveAWSEndpoint(endpoints); err == nil || !strings.Contains(err.Error(), "no available endpoints") {
		t.Fatalf("resolveAWSEndpoint() error = %v, want no available endpoints", err)
	}
}

func TestAWSEndpointAttemptFinishNilSafe(t *testing.T) {
	t.Parallel()

	var nilAttempt *awsEndpointAttempt
	nilAttempt.finish(true)
	(&awsEndpointAttempt{}).finish(true)
}

func TestEndpointIndexForRequestMatchesBasePathBoundary(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSet([]string{"http://example.test/api"})
	if err != nil {
		t.Fatalf("NewEndpointSet() error = %v", err)
	}
	matchedURL, err := url.Parse("http://example.test/api/bucket/key")
	if err != nil {
		t.Fatalf("Parse matched URL: %v", err)
	}
	index, ok := endpointIndexForRequest(endpoints, matchedURL)
	if !ok || index != 0 {
		t.Fatalf("endpointIndexForRequest() = %d, %v; want 0, true", index, ok)
	}

	nearMissURL, err := url.Parse("http://example.test/apiary/bucket")
	if err != nil {
		t.Fatalf("Parse near-miss URL: %v", err)
	}
	if _, ok := endpointIndexForRequest(endpoints, nearMissURL); ok {
		t.Fatal("endpointIndexForRequest() matched path prefix without boundary")
	}
	if _, ok := endpointIndexForRequest(nil, matchedURL); ok {
		t.Fatal("endpointIndexForRequest(nil) matched")
	}
	if _, ok := endpointIndexForRequest(endpoints, nil); ok {
		t.Fatal("endpointIndexForRequest(nil URL) matched")
	}
	differentHostURL, err := url.Parse("http://other.example.test/api/bucket")
	if err != nil {
		t.Fatalf("Parse different host URL: %v", err)
	}
	if _, ok := endpointIndexForRequest(endpoints, differentHostURL); ok {
		t.Fatal("endpointIndexForRequest() matched different host")
	}
}

func TestAWSEndpointMiddlewareBranches(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSet([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("NewEndpointSet() error = %v", err)
	}
	attemptMiddleware := awsEndpointAttemptMiddleware{endpoints: endpoints}
	if attemptMiddleware.ID() != "SeaweedEndpointAttempt" {
		t.Fatalf("attempt middleware ID = %q", attemptMiddleware.ID())
	}
	resultMiddleware := awsEndpointResultMiddleware{}
	if resultMiddleware.ID() != "SeaweedEndpointResult" {
		t.Fatalf("result middleware ID = %q", resultMiddleware.ID())
	}

	finalizeCalls := 0
	_, _, err = attemptMiddleware.HandleFinalize(context.Background(), middleware.FinalizeInput{
		Request: "not-http",
	}, finalizeHandlerFunc(func(ctx context.Context, in middleware.FinalizeInput) (middleware.FinalizeOutput, middleware.Metadata, error) {
		finalizeCalls++
		return middleware.FinalizeOutput{}, middleware.Metadata{}, nil
	}))
	if err != nil {
		t.Fatalf("HandleFinalize(non-http) error = %v", err)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1", finalizeCalls)
	}

	req, ok := smithyhttp.NewStackRequest().(*smithyhttp.Request)
	if !ok {
		t.Fatal("NewStackRequest() did not return *smithyhttp.Request")
	}
	req.URL = mustParseURL(t, "http://other.example.test/")
	_, _, err = attemptMiddleware.HandleFinalize(context.Background(), middleware.FinalizeInput{
		Request: req,
	}, finalizeHandlerFunc(func(ctx context.Context, in middleware.FinalizeInput) (middleware.FinalizeOutput, middleware.Metadata, error) {
		finalizeCalls++
		return middleware.FinalizeOutput{}, middleware.Metadata{}, nil
	}))
	if err != nil {
		t.Fatalf("HandleFinalize(no match) error = %v", err)
	}
	if finalizeCalls != 2 {
		t.Fatalf("finalize calls = %d, want 2", finalizeCalls)
	}

	_, _, err = resultMiddleware.HandleDeserialize(context.Background(), middleware.DeserializeInput{}, deserializeHandlerFunc(func(ctx context.Context, in middleware.DeserializeInput) (middleware.DeserializeOutput, middleware.Metadata, error) {
		return middleware.DeserializeOutput{}, middleware.Metadata{}, errors.New("transport")
	}))
	if err == nil {
		t.Fatal("HandleDeserialize() error = nil, want transport error")
	}
}

func TestAWSEndpointPolicyMiddlewareRequiresResolverMiddleware(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSet([]string{"http://example.test"})
	if err != nil {
		t.Fatalf("NewEndpointSet() error = %v", err)
	}
	stack := middleware.NewStack("test", smithyhttp.NewStackRequest)
	if err := awsEndpointPolicyMiddleware(endpoints)(stack); err == nil {
		t.Fatal("awsEndpointPolicyMiddleware() error = nil, want missing ResolveEndpointV2 error")
	}
}

func TestAWSEndpointAttemptMiddlewareRecordsFinalizeError(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSetWithPolicy([]string{"http://example.test"}, httpx.EndpointPolicy{
		CircuitBreaker: httpx.EndpointCircuitBreakerPolicy{
			Enabled:          true,
			FailureThreshold: 1,
			OpenTimeout:      time.Hour,
		},
	})
	if err != nil {
		t.Fatalf("NewEndpointSetWithPolicy() error = %v", err)
	}
	req, ok := smithyhttp.NewStackRequest().(*smithyhttp.Request)
	if !ok {
		t.Fatal("NewStackRequest() did not return *smithyhttp.Request")
	}
	req.URL = mustParseURL(t, "http://example.test/")
	_, _, err = (awsEndpointAttemptMiddleware{endpoints: endpoints}).HandleFinalize(context.Background(), middleware.FinalizeInput{
		Request: req,
	}, finalizeHandlerFunc(func(ctx context.Context, in middleware.FinalizeInput) (middleware.FinalizeOutput, middleware.Metadata, error) {
		return middleware.FinalizeOutput{}, middleware.Metadata{}, errors.New("signing")
	}))
	if err == nil {
		t.Fatal("HandleFinalize() error = nil, want signing error")
	}
	if _, err := endpoints.Lease("/"); err == nil || !strings.Contains(err.Error(), "no available endpoints") {
		t.Fatalf("Lease() error = %v, want open circuit", err)
	}
}

func TestAWSEndpointResultMiddlewareOpensCircuitOnThrottle(t *testing.T) {
	t.Parallel()

	endpoints, err := httpx.NewEndpointSetWithPolicy([]string{"http://example.test"}, httpx.EndpointPolicy{
		CircuitBreaker: httpx.EndpointCircuitBreakerPolicy{
			Enabled:          true,
			FailureThreshold: 1,
			OpenTimeout:      time.Hour,
		},
	})
	if err != nil {
		t.Fatalf("NewEndpointSetWithPolicy() error = %v", err)
	}
	attempt := &awsEndpointAttempt{endpoints: endpoints, index: 0}
	ctx := context.WithValue(context.Background(), awsEndpointAttemptKey{}, attempt)
	_, _, err = awsEndpointResultMiddleware{}.HandleDeserialize(ctx, middleware.DeserializeInput{}, deserializeHandlerFunc(func(ctx context.Context, in middleware.DeserializeInput) (middleware.DeserializeOutput, middleware.Metadata, error) {
		return middleware.DeserializeOutput{
			RawResponse: &smithyhttp.Response{
				Response: &http.Response{StatusCode: http.StatusTooManyRequests},
			},
		}, middleware.Metadata{}, nil
	}))
	if err != nil {
		t.Fatalf("HandleDeserialize() error = %v", err)
	}
	if _, err := endpoints.Lease("/"); err == nil || !strings.Contains(err.Error(), "no available endpoints") {
		t.Fatalf("Lease() error = %v, want open circuit", err)
	}
	attempt.finish(true)
	if _, err := endpoints.Lease("/"); err == nil || !strings.Contains(err.Error(), "no available endpoints") {
		t.Fatalf("second finish changed circuit state: %v", err)
	}
}

type finalizeHandlerFunc func(context.Context, middleware.FinalizeInput) (middleware.FinalizeOutput, middleware.Metadata, error)

func (f finalizeHandlerFunc) HandleFinalize(ctx context.Context, in middleware.FinalizeInput) (middleware.FinalizeOutput, middleware.Metadata, error) {
	return f(ctx, in)
}

type deserializeHandlerFunc func(context.Context, middleware.DeserializeInput) (middleware.DeserializeOutput, middleware.Metadata, error)

func (f deserializeHandlerFunc) HandleDeserialize(ctx context.Context, in middleware.DeserializeInput) (middleware.DeserializeOutput, middleware.Metadata, error) {
	return f(ctx, in)
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("Parse URL: %v", err)
	}
	return parsed
}
