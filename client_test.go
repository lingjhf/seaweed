package seaweed_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/lingjhf/seaweed"
)

func TestNewRequiresMasterURL(t *testing.T) {
	t.Parallel()

	_, err := seaweed.New(seaweed.Config{})
	if err == nil {
		t.Fatal("New() error = nil, want error")
	}
}

func TestNewNormalizesMasterURL(t *testing.T) {
	t.Parallel()

	client, err := seaweed.New(seaweed.Config{
		MasterURL: "http://127.0.0.1:9333/",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if client.Config().MasterURL != "http://127.0.0.1:9333" {
		t.Fatalf("MasterURL = %q", client.Config().MasterURL)
	}
	if client.Config().TUSBasePath != "/.tus" {
		t.Fatalf("TUSBasePath = %q", client.Config().TUSBasePath)
	}
}

func TestNewNormalizesConfiguredURLsAndAccessors(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{}
	client, err := seaweed.New(seaweed.Config{
		MasterURL:       "http://127.0.0.1:9333/master/?q=1#fragment",
		VolumeURL:       "http://127.0.0.1:8080/volume/",
		FilerURL:        "http://127.0.0.1:8888/filer/",
		TUSBasePath:     "uploads",
		S3URL:           "http://127.0.0.1:8333/s3/",
		IAMURL:          "http://127.0.0.1:8333/iam/",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		UserAgent:       "seaweed-test",
		BearerToken:     "token",
		UsePublicURLs:   true,
	}, seaweed.WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	config := client.Config()
	if config.MasterURL != "http://127.0.0.1:9333/master" {
		t.Fatalf("MasterURL = %q", config.MasterURL)
	}
	if config.VolumeURL != "http://127.0.0.1:8080/volume" {
		t.Fatalf("VolumeURL = %q", config.VolumeURL)
	}
	if config.FilerURL != "http://127.0.0.1:8888/filer" {
		t.Fatalf("FilerURL = %q", config.FilerURL)
	}
	if config.S3URL != "http://127.0.0.1:8333/s3" {
		t.Fatalf("S3URL = %q", config.S3URL)
	}
	if config.IAMURL != "http://127.0.0.1:8333/iam" {
		t.Fatalf("IAMURL = %q", config.IAMURL)
	}
	if config.Region != "us-east-1" {
		t.Fatalf("Region = %q, want us-east-1", config.Region)
	}
	if config.Retry.MaxAttempts != 3 {
		t.Fatalf("Retry.MaxAttempts = %d, want 3", config.Retry.MaxAttempts)
	}
	if client.Master() == nil || client.Volume() == nil || client.Blob() == nil || client.Filer() == nil || client.TUS() == nil {
		t.Fatal("client accessors returned nil")
	}
	if s3Client, err := client.S3(context.Background()); err != nil || s3Client == nil {
		t.Fatalf("S3() = %v, %v; want client", s3Client, err)
	}
	if iamClient, err := client.IAM(context.Background()); err != nil || iamClient == nil {
		t.Fatalf("IAM() = %v, %v; want client", iamClient, err)
	}
}

func TestNewRejectsNilHTTPClient(t *testing.T) {
	t.Parallel()

	_, err := seaweed.New(seaweed.Config{
		MasterURL: "http://127.0.0.1:9333",
	}, seaweed.WithHTTPClient(nil))
	if err == nil {
		t.Fatal("New() error = nil, want nil http client error")
	}
}

func TestNewRejectsInvalidURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config seaweed.Config
	}{
		{
			name: "master",
			config: seaweed.Config{
				MasterURL: "127.0.0.1:9333",
			},
		},
		{
			name: "volume",
			config: seaweed.Config{
				MasterURL: "http://127.0.0.1:9333",
				VolumeURL: "127.0.0.1:8080",
			},
		},
		{
			name: "filer",
			config: seaweed.Config{
				MasterURL: "http://127.0.0.1:9333",
				FilerURL:  "127.0.0.1:8888",
			},
		},
		{
			name: "s3",
			config: seaweed.Config{
				MasterURL: "http://127.0.0.1:9333",
				S3URL:     "127.0.0.1:8333",
			},
		},
		{
			name: "iam",
			config: seaweed.Config{
				MasterURL: "http://127.0.0.1:9333",
				IAMURL:    "127.0.0.1:8333",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := seaweed.New(tt.config); err == nil {
				t.Fatal("New() error = nil, want invalid url error")
			}
		})
	}
}

func TestIAMFallsBackToS3Endpoint(t *testing.T) {
	t.Parallel()

	client, err := seaweed.New(seaweed.Config{
		MasterURL:       "http://127.0.0.1:9333",
		S3URL:           "http://127.0.0.1:8333",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	iamClient, err := client.IAM(context.Background())
	if err != nil {
		t.Fatalf("IAM() error = %v", err)
	}
	if iamClient == nil {
		t.Fatal("IAM() = nil, want client")
	}
}
