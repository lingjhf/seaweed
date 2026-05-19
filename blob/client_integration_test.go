//go:build integration

package blob_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lingjhf/seaweed"
	"github.com/lingjhf/seaweed/blob"
	"github.com/lingjhf/seaweed/internal/testweed"
)

func TestBlobPutGetDeleteIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolume(t, ctx)
	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{cluster.MasterURL},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	put, err := client.Blob().Put(ctx, strings.NewReader("blob-data"), blob.PutOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("blob-data")),
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	resp, err := client.Blob().Get(ctx, put.FileID, blob.GetOptions{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "blob-data" {
		t.Fatalf("body = %q, want blob-data", body)
	}

	if err := client.Blob().Delete(ctx, put.FileID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestBlobAuthorizationIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolumeWithSecurity(t, ctx, `
[jwt.signing]
key = "write-secret"
expires_after_seconds = 60

[jwt.signing.read]
key = "read-secret"
expires_after_seconds = 60

[access]
ui = true
`)
	client, err := seaweed.New(seaweed.Config{
		MasterURLs:                []string{cluster.MasterURL},
		EnableVolumeAuthorization: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	put, err := client.Blob().Put(ctx, strings.NewReader("secure-blob-data"), blob.PutOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("secure-blob-data")),
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	resp, err := client.Blob().Get(ctx, put.FileID, blob.GetOptions{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "secure-blob-data" {
		t.Fatalf("body = %q, want secure-blob-data", body)
	}

	if header, err := client.Blob().Head(ctx, put.FileID); err != nil {
		t.Fatalf("Head() error = %v", err)
	} else if header.Get("ETag") == "" {
		t.Fatal("Head().ETag is empty")
	}
	if err := client.Blob().Delete(ctx, put.FileID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}
