//go:build integration

package volume_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lingjhf/seaweed"
	"github.com/lingjhf/seaweed/internal/testweed"
)

func TestVolumePutGetDeleteIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolume(t, ctx)
	client, err := seaweed.New(seaweed.Config{
		MasterURL: cluster.MasterURL,
		VolumeURL: cluster.VolumeURL,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	assigned, err := client.Master().Assign(ctx, seaweedMasterAssignOptions())
	if err != nil {
		t.Fatalf("Assign() error = %v", err)
	}
	_, err = client.Volume().Put(ctx, assigned.FID, strings.NewReader("volume-data"), seaweedVolumePutOptions())
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	resp, err := client.Volume().Get(ctx, assigned.FID, seaweedVolumeGetOptions())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "volume-data" {
		t.Fatalf("body = %q, want volume-data", body)
	}

	if err := client.Volume().Delete(ctx, assigned.FID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}
