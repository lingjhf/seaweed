//go:build integration

package seaweed_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/lingjhf/seaweed"
	"github.com/lingjhf/seaweed/internal/testweed"
)

func TestS3AndIAMIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolumeFilerS3(t, ctx)
	client, err := seaweed.New(seaweed.Config{
		MasterURLs:      []string{cluster.MasterURL},
		FilerURLs:       []string{cluster.FilerURL},
		S3URLs:          []string{cluster.S3URL},
		AccessKeyID:     "seaweed_admin",
		SecretAccessKey: "seaweed_secret",
		Region:          "us-east-1",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	s3Client, err := client.S3(ctx)
	if err != nil {
		t.Fatalf("S3() error = %v", err)
	}
	bucket := fmt.Sprintf("sdk-test-%d", time.Now().UnixNano())
	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = s3Client.DeleteObject(cleanupCtx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String("hello.txt"),
		})
		_, _ = s3Client.DeleteBucket(cleanupCtx, &s3.DeleteBucketInput{
			Bucket: aws.String(bucket),
		})
	})
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String("hello.txt"),
		Body:        strings.NewReader("s3-data"),
		ContentType: aws.String("text/plain"),
	})
	if err != nil {
		t.Fatalf("PutObject() error = %v", err)
	}
	head, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("hello.txt"),
	})
	if err != nil {
		t.Fatalf("HeadObject() error = %v", err)
	}
	if got := aws.ToInt64(head.ContentLength); got != int64(len("s3-data")) {
		t.Fatalf("HeadObject().ContentLength = %d, want %d", got, len("s3-data"))
	}
	if got := aws.ToString(head.ContentType); got != "text/plain" {
		t.Fatalf("HeadObject().ContentType = %q, want text/plain", got)
	}
	got, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("hello.txt"),
	})
	if err != nil {
		t.Fatalf("GetObject() error = %v", err)
	}
	body, err := io.ReadAll(got.Body)
	got.Body.Close()
	if err != nil {
		t.Fatalf("read S3 body: %v", err)
	}
	if string(body) != "s3-data" {
		t.Fatalf("S3 body = %q, want s3-data", body)
	}
	list, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("hello"),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2() error = %v", err)
	}
	if !containsObjectKey(list.Contents, "hello.txt") {
		t.Fatalf("ListObjectsV2() keys = %#v, want hello.txt", list.Contents)
	}
	_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("hello.txt"),
	})
	if err != nil {
		t.Fatalf("DeleteObject() error = %v", err)
	}
	list, err = s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("hello"),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2() after delete error = %v", err)
	}
	if containsObjectKey(list.Contents, "hello.txt") {
		t.Fatalf("ListObjectsV2() after delete still contains hello.txt: %#v", list.Contents)
	}
	_, err = s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		t.Fatalf("DeleteBucket() error = %v", err)
	}

	iamClient, err := client.IAM(ctx)
	if err != nil {
		t.Fatalf("IAM() error = %v", err)
	}
	if _, err := iamClient.ListUsers(ctx, &iam.ListUsersInput{}); err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
}

func TestS3EndpointPolicyMultiEndpointIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolumeFilerS3(t, ctx)
	secondS3URL := cluster.StartS3(t, ctx)
	firstProxy := newS3Proxy(t, cluster.S3URL)
	defer firstProxy.Close()
	secondProxy := newS3Proxy(t, secondS3URL)
	defer secondProxy.Close()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs:      []string{cluster.MasterURL},
		S3URLs:          []string{firstProxy.URL(), secondProxy.URL()},
		AccessKeyID:     "seaweed_admin",
		SecretAccessKey: "seaweed_secret",
		Region:          "us-east-1",
		S3EndpointPolicy: seaweed.EndpointPolicy{
			Mode: seaweed.EndpointPolicyRoundRobin,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	s3Client, err := client.S3(ctx)
	if err != nil {
		t.Fatalf("S3() error = %v", err)
	}

	for range 4 {
		if _, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{}); err != nil {
			t.Fatalf("ListBuckets() error = %v", err)
		}
	}
	if firstProxy.Calls() == 0 || secondProxy.Calls() == 0 {
		t.Fatalf("proxy calls = %d/%d, want both endpoints used", firstProxy.Calls(), secondProxy.Calls())
	}
}

func TestS3EndpointPolicyFailoverIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolumeFilerS3(t, ctx)
	secondS3URL := cluster.StartS3(t, ctx)
	firstProxy := newS3Proxy(t, cluster.S3URL)
	firstProxy.FailNext(http.StatusInternalServerError)
	defer firstProxy.Close()
	secondProxy := newS3Proxy(t, secondS3URL)
	defer secondProxy.Close()

	client, err := seaweed.New(seaweed.Config{
		MasterURLs:      []string{cluster.MasterURL},
		S3URLs:          []string{firstProxy.URL(), secondProxy.URL()},
		AccessKeyID:     "seaweed_admin",
		SecretAccessKey: "seaweed_secret",
		Region:          "us-east-1",
		S3EndpointPolicy: seaweed.EndpointPolicy{
			CircuitBreaker: seaweed.EndpointCircuitBreakerPolicy{
				Enabled:          true,
				FailureThreshold: 1,
				OpenTimeout:      time.Minute,
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	s3Client, err := client.S3(ctx)
	if err != nil {
		t.Fatalf("S3() error = %v", err)
	}

	if _, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{}); err != nil {
		t.Fatalf("ListBuckets() error = %v", err)
	}
	if firstProxy.Calls() == 0 || secondProxy.Calls() == 0 {
		t.Fatalf("proxy calls = %d/%d, want failed endpoint then retry endpoint", firstProxy.Calls(), secondProxy.Calls())
	}
}

func containsObjectKey(objects []s3types.Object, key string) bool {
	for _, object := range objects {
		if aws.ToString(object.Key) == key {
			return true
		}
	}
	return false
}

type s3Proxy struct {
	server     *httptest.Server
	calls      atomic.Int32
	failStatus atomic.Int32
}

func newS3Proxy(t *testing.T, targetURL string) *s3Proxy {
	t.Helper()

	target, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("parse s3 target url: %v", err)
	}
	proxy := &s3Proxy{}
	reverseProxy := httputil.NewSingleHostReverseProxy(target)
	proxy.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.calls.Add(1)
		if status := int(proxy.failStatus.Swap(0)); status != 0 {
			w.WriteHeader(status)
			return
		}
		reverseProxy.ServeHTTP(w, r)
	}))
	return proxy
}

func (p *s3Proxy) URL() string {
	return p.server.URL
}

func (p *s3Proxy) Calls() int32 {
	return p.calls.Load()
}

func (p *s3Proxy) FailNext(status int) {
	p.failStatus.Store(int32(status))
}

func (p *s3Proxy) Close() {
	p.server.Close()
}
