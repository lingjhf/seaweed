package seaweed_test

import (
	"context"
	"testing"

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
		S3URL:      "http://127.0.0.1:8333",
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
		IAMURL:     "http://127.0.0.1:8333",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.IAM(context.Background()); err == nil {
		t.Fatal("IAM() without credentials error = nil, want error")
	}
}
