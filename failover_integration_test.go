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
	if _, err := client.Filer().ListPage(ctx, "/", filer.ListOptions{Limit: 1}); err != nil {
		t.Fatalf("Filer().ListPage() error = %v", err)
	}
}

func TestNativeEndpointPolicyIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolumeFiler(t, ctx)
	deadURL := closedHTTPURL(t)
	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{deadURL, cluster.MasterURL},
		FilerURLs:  []string{deadURL, cluster.FilerURL},
		EndpointPolicy: seaweed.EndpointPolicy{
			HealthCheck: seaweed.EndpointHealthCheckPolicy{
				Enabled:          true,
				Interval:         50 * time.Millisecond,
				Timeout:          200 * time.Millisecond,
				FailureThreshold: 1,
				SuccessThreshold: 1,
			},
			CircuitBreaker: seaweed.EndpointCircuitBreakerPolicy{
				Enabled:          true,
				FailureThreshold: 1,
				OpenTimeout:      time.Second,
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()

	if err := client.Master().Health(ctx); err != nil {
		t.Fatalf("Master().Health() error = %v", err)
	}
	if _, err := client.Filer().ListPage(ctx, "/", filer.ListOptions{Limit: 1}); err != nil {
		t.Fatalf("Filer().ListPage() error = %v", err)
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
