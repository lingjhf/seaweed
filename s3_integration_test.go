//go:build integration

package seaweed_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	bucket := "sdk-test-bucket"
	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String("hello.txt"),
		Body:        strings.NewReader("s3-data"),
		ContentType: aws.String("text/plain"),
	})
	if err != nil {
		t.Fatalf("PutObject() error = %v", err)
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

	iamClient, err := client.IAM(ctx)
	if err != nil {
		t.Fatalf("IAM() error = %v", err)
	}
	if _, err := iamClient.ListUsers(ctx, &iam.ListUsersInput{}); err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
}
