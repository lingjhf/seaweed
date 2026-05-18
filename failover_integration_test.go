//go:build integration

package seaweed_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/lingjhf/seaweed"
	"github.com/lingjhf/seaweed/filer"
	"github.com/lingjhf/seaweed/internal/testweed"
)

func TestNativeEndpointFailoverIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolumeFiler(t, ctx)
	deadURL := closedHTTPURL(t)
	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{deadURL, cluster.MasterURL},
		FilerURLs:  []string{deadURL, cluster.FilerURL},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := client.Master().Health(ctx); err != nil {
		t.Fatalf("Master().Health() error = %v", err)
	}
	if _, err := client.Filer().List(ctx, "/", filer.ListOptions{Limit: 1}); err != nil {
		t.Fatalf("Filer().List() error = %v", err)
	}
}

func closedHTTPURL(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return fmt.Sprintf("http://%s", addr)
}
