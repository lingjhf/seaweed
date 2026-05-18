package seaweed_test

import (
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
	if client.Config().TusBasePath != "/.tus" {
		t.Fatalf("TusBasePath = %q", client.Config().TusBasePath)
	}
}
