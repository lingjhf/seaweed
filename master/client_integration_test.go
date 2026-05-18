//go:build integration

package master_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lingjhf/seaweed"
	"github.com/lingjhf/seaweed/internal/testweed"
	"github.com/lingjhf/seaweed/master"
)

func TestMasterAssignLookupIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolume(t, ctx)
	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{cluster.MasterURL},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := client.Master().Health(ctx); err != nil {
		t.Fatalf("Health() error = %v", err)
	}

	assigned, err := client.Master().Assign(ctx, master.AssignOptions{})
	if err != nil {
		t.Fatalf("Assign() error = %v", err)
	}
	if assigned.FID == "" {
		t.Fatal("Assign().FID is empty")
	}

	volumeID := strings.Split(assigned.FID, ",")[0]
	lookup, err := client.Master().Lookup(ctx, volumeID, master.LookupOptions{})
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if len(lookup.Locations) == 0 {
		t.Fatal("Lookup().Locations is empty")
	}
}
