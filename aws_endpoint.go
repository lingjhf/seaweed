package seaweed

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
	smithyauth "github.com/aws/smithy-go/auth"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/lingjhf/seaweed/internal/httpx"
)

type s3EndpointResolver struct {
	endpoints *httpx.EndpointSet
}

func (r s3EndpointResolver) ResolveEndpoint(ctx context.Context, params s3.EndpointParameters) (smithyendpoints.Endpoint, error) {
	endpoint, err := resolveAWSEndpoint(r.endpoints)
	if err != nil {
		return smithyendpoints.Endpoint{}, err
	}
	if params.Bucket != nil && *params.Bucket != "" {
		bucket := *params.Bucket
		endpoint.URI.Path = smithyhttp.JoinPath(endpoint.URI.Path, bucket)
		if endpoint.URI.RawPath != "" {
			endpoint.URI.RawPath = smithyhttp.JoinPath(endpoint.URI.RawPath, url.PathEscape(bucket))
		}
	}
	return endpoint, nil
}

type iamEndpointResolver struct {
	endpoints *httpx.EndpointSet
}

func (r iamEndpointResolver) ResolveEndpoint(ctx context.Context, params iam.EndpointParameters) (smithyendpoints.Endpoint, error) {
	return resolveAWSEndpoint(r.endpoints)
}

func resolveAWSEndpoint(endpoints *httpx.EndpointSet) (smithyendpoints.Endpoint, error) {
	if endpoints == nil {
		return smithyendpoints.Endpoint{}, fmt.Errorf("seaweed: endpoints are required")
	}
	lease, err := endpoints.Lease("")
	if err != nil {
		return smithyendpoints.Endpoint{}, err
	}
	parsed, err := url.Parse(lease.URL)
	if err != nil {
		lease.Finish(false)
		return smithyendpoints.Endpoint{}, err
	}
	return smithyendpoints.Endpoint{URI: *parsed}, nil
}

type s3AuthSchemeResolver struct{}

func (s3AuthSchemeResolver) ResolveAuthSchemes(ctx context.Context, params *s3.AuthResolverParameters) ([]*smithyauth.Option, error) {
	return []*smithyauth.Option{sigV4AuthOption("s3", params.Region)}, nil
}

type iamAuthSchemeResolver struct{}

func (iamAuthSchemeResolver) ResolveAuthSchemes(ctx context.Context, params *iam.AuthResolverParameters) ([]*smithyauth.Option, error) {
	return []*smithyauth.Option{sigV4AuthOption("iam", params.Region)}, nil
}

func sigV4AuthOption(signingName string, signingRegion string) *smithyauth.Option {
	var props smithy.Properties
	smithyhttp.SetSigV4SigningName(&props, signingName)
	smithyhttp.SetSigV4SigningRegion(&props, signingRegion)
	return &smithyauth.Option{
		SchemeID:         smithyauth.SchemeIDSigV4,
		SignerProperties: props,
	}
}

func awsEndpointPolicyMiddleware(endpoints *httpx.EndpointSet) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		if err := stack.Finalize.Insert(awsEndpointAttemptMiddleware{endpoints: endpoints}, "ResolveEndpointV2", middleware.After); err != nil {
			return err
		}
		return stack.Deserialize.Add(awsEndpointResultMiddleware{}, middleware.After)
	}
}

type awsEndpointAttemptKey struct{}

type awsEndpointAttempt struct {
	once      sync.Once
	endpoints *httpx.EndpointSet
	index     int
}

func (a *awsEndpointAttempt) finish(success bool) {
	if a == nil || a.endpoints == nil {
		return
	}
	a.once.Do(func() {
		a.endpoints.FinishCandidate(a.index, success)
	})
}

type awsEndpointAttemptMiddleware struct {
	endpoints *httpx.EndpointSet
}

func (m awsEndpointAttemptMiddleware) ID() string {
	return "SeaweedEndpointAttempt"
}

func (m awsEndpointAttemptMiddleware) HandleFinalize(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (
	out middleware.FinalizeOutput, metadata middleware.Metadata, err error,
) {
	request, ok := in.Request.(*smithyhttp.Request)
	if !ok || request.URL == nil {
		return next.HandleFinalize(ctx, in)
	}
	index, ok := endpointIndexForRequest(m.endpoints, request.URL)
	if !ok {
		return next.HandleFinalize(ctx, in)
	}
	attempt := &awsEndpointAttempt{
		endpoints: m.endpoints,
		index:     index,
	}
	ctx = context.WithValue(ctx, awsEndpointAttemptKey{}, attempt)
	out, metadata, err = next.HandleFinalize(ctx, in)
	if err != nil {
		attempt.finish(false)
	}
	return out, metadata, err
}

type awsEndpointResultMiddleware struct{}

func (awsEndpointResultMiddleware) ID() string {
	return "SeaweedEndpointResult"
}

func (awsEndpointResultMiddleware) HandleDeserialize(ctx context.Context, in middleware.DeserializeInput, next middleware.DeserializeHandler) (
	out middleware.DeserializeOutput, metadata middleware.Metadata, err error,
) {
	out, metadata, err = next.HandleDeserialize(ctx, in)
	attempt, _ := ctx.Value(awsEndpointAttemptKey{}).(*awsEndpointAttempt)
	if attempt == nil {
		return out, metadata, err
	}
	success := err == nil
	if response, ok := out.RawResponse.(*smithyhttp.Response); ok && response.Response != nil {
		status := response.StatusCode
		success = status != http.StatusTooManyRequests && status < http.StatusInternalServerError
	}
	attempt.finish(success)
	return out, metadata, err
}

func endpointIndexForRequest(endpoints *httpx.EndpointSet, requestURL *url.URL) (int, bool) {
	if endpoints == nil || requestURL == nil {
		return 0, false
	}
	for index, rawEndpoint := range endpoints.URLs() {
		endpointURL, err := url.Parse(rawEndpoint)
		if err != nil {
			continue
		}
		if requestURL.Scheme != endpointURL.Scheme || requestURL.Host != endpointURL.Host {
			continue
		}
		endpointPath := strings.TrimRight(endpointURL.EscapedPath(), "/")
		requestPath := strings.TrimRight(requestURL.EscapedPath(), "/")
		if endpointPath == "" || requestPath == endpointPath || strings.HasPrefix(requestPath, endpointPath+"/") {
			return index, true
		}
	}
	return 0, false
}
