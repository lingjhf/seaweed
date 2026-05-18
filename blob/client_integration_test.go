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
