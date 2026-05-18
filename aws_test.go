package seaweed_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lingjhf/seaweed"
)

func TestS3RequiresEndpointAndCredentials(t *testing.T) {
	t.Parallel()

	client, err := seaweed.New(seaweed.Config{MasterURLs: []string{"http://127.0.0.1:9333"}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.S3(context.Background()); err == nil {
		t.Fatal("S3() error = nil, want error")
	}

	client, err = seaweed.New(seaweed.Config{
		MasterURLs: []string{"http://127.0.0.1:9333"},
		S3URLs:     []string{"http://127.0.0.1:8333"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.S3(context.Background()); err == nil {
		t.Fatal("S3() without credentials error = nil, want error")
	}
}

func TestIAMRequiresEndpointOrS3AndCredentials(t *testing.T) {
	t.Parallel()

	client, err := seaweed.New(seaweed.Config{MasterURLs: []string{"http://127.0.0.1:9333"}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.IAM(context.Background()); err == nil {
		t.Fatal("IAM() error = nil, want endpoint error")
	}

	client, err = seaweed.New(seaweed.Config{
		MasterURLs: []string{"http://127.0.0.1:9333"},
		IAMURLs:    []string{"http://127.0.0.1:8333"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.IAM(context.Background()); err == nil {
		t.Fatal("IAM() without credentials error = nil, want error")
	}
}

func TestS3EndpointPolicyRoundRobin(t *testing.T) {
	t.Parallel()

	first, firstCalls := newListBucketsServer(t, http.StatusOK)
	defer first.Close()
	second, secondCalls := newListBucketsServer(t, http.StatusOK)
	defer second.Close()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs:      []string{"http://127.0.0.1:9333"},
		S3URLs:          []string{first.URL, second.URL},
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		S3EndpointPolicy: seaweed.EndpointPolicy{
			Mode: seaweed.EndpointPolicyRoundRobin,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	s3Client, err := client.S3(context.Background())
	if err != nil {
		t.Fatalf("S3() error = %v", err)
	}

	for range 4 {
		if _, err := s3Client.ListBuckets(context.Background(), nil); err != nil {
			t.Fatalf("ListBuckets() error = %v", err)
		}
	}
	if firstCalls.Load() != 2 || secondCalls.Load() != 2 {
		t.Fatalf("s3 calls = %d/%d, want 2/2", firstCalls.Load(), secondCalls.Load())
	}
}

func TestS3EndpointPolicyUsesPathStyleBucketEndpoint(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != "/sdk-test-bucket" {
			t.Errorf("path = %q, want /sdk-test-bucket", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs:      []string{"http://127.0.0.1:9333"},
		S3URLs:          []string{server.URL},
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	s3Client, err := client.S3(context.Background())
	if err != nil {
		t.Fatalf("S3() error = %v", err)
	}

	if _, err := s3Client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String("sdk-test-bucket"),
	}); err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestIAMEndpointPolicyFallsBackToS3URLsRoundRobin(t *testing.T) {
	t.Parallel()

	first, firstCalls := newListUsersServer(t)
	defer first.Close()
	second, secondCalls := newListUsersServer(t)
	defer second.Close()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs:      []string{"http://127.0.0.1:9333"},
		S3URLs:          []string{first.URL, second.URL},
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		IAMEndpointPolicy: seaweed.EndpointPolicy{
			Mode: seaweed.EndpointPolicyRoundRobin,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	iamClient, err := client.IAM(context.Background())
	if err != nil {
		t.Fatalf("IAM() error = %v", err)
	}

	for range 4 {
		if _, err := iamClient.ListUsers(context.Background(), nil); err != nil {
			t.Fatalf("ListUsers() error = %v", err)
		}
	}
	if firstCalls.Load() != 2 || secondCalls.Load() != 2 {
		t.Fatalf("iam calls = %d/%d, want 2/2", firstCalls.Load(), secondCalls.Load())
	}
}

func TestS3EndpointPolicyTreatsClientErrorsAsEndpointSuccess(t *testing.T) {
	t.Parallel()

	first, firstCalls := newListBucketsServer(t, http.StatusForbidden)
	defer first.Close()
	second, secondCalls := newListBucketsServer(t, http.StatusOK)
	defer second.Close()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs:      []string{"http://127.0.0.1:9333"},
		S3URLs:          []string{first.URL, second.URL},
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		S3EndpointPolicy: seaweed.EndpointPolicy{
			CircuitBreaker: seaweed.EndpointCircuitBreakerPolicy{
				Enabled:          true,
				FailureThreshold: 1,
				OpenTimeout:      time.Hour,
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	s3Client, err := client.S3(context.Background())
	if err != nil {
		t.Fatalf("S3() error = %v", err)
	}

	for range 2 {
		if _, err := s3Client.ListBuckets(context.Background(), nil); err == nil {
			t.Fatal("ListBuckets() error = nil, want access denied error")
		}
	}
	if firstCalls.Load() != 2 || secondCalls.Load() != 0 {
		t.Fatalf("s3 calls = %d/%d, want 403 endpoint to stay selected", firstCalls.Load(), secondCalls.Load())
	}
}

func TestS3EndpointPolicyRetriesNextEndpointAfterServerError(t *testing.T) {
	t.Parallel()

	first, firstCalls := newListBucketsServer(t, http.StatusInternalServerError)
	defer first.Close()
	second, secondCalls := newListBucketsServer(t, http.StatusOK)
	defer second.Close()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs:      []string{"http://127.0.0.1:9333"},
		S3URLs:          []string{first.URL, second.URL},
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		S3EndpointPolicy: seaweed.EndpointPolicy{
			CircuitBreaker: seaweed.EndpointCircuitBreakerPolicy{
				Enabled:          true,
				FailureThreshold: 1,
				OpenTimeout:      time.Hour,
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	s3Client, err := client.S3(context.Background())
	if err != nil {
		t.Fatalf("S3() error = %v", err)
	}

	if _, err := s3Client.ListBuckets(context.Background(), nil); err != nil {
		t.Fatalf("ListBuckets() error = %v", err)
	}
	if firstCalls.Load() != 1 || secondCalls.Load() == 0 {
		t.Fatalf("s3 calls = %d/%d, want first endpoint once then retry on second", firstCalls.Load(), secondCalls.Load())
	}
}

func newListBucketsServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/" {
			t.Errorf("path = %q, want /", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(status)
		if status == http.StatusForbidden {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>denied</Message></Error>`))
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Owner><ID>test</ID><DisplayName>test</DisplayName></Owner><Buckets></Buckets></ListAllMyBucketsResult>`))
	}))
	return server, &calls
}

func newListUsersServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/" {
			t.Errorf("path = %q, want /", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<ListUsersResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/"><ListUsersResult><Users></Users><IsTruncated>false</IsTruncated></ListUsersResult><ResponseMetadata><RequestId>test</RequestId></ResponseMetadata></ListUsersResponse>`))
	}))
	return server, &calls
}
