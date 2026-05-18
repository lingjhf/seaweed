//go:build integration

package master_test

import (
	"context"
	"strconv"
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

	dirStatus, err := client.Master().DirStatus(ctx)
	if err != nil {
		t.Fatalf("DirStatus() error = %v", err)
	}
	if len(dirStatus.Topology.DataCenters) == 0 {
		t.Fatal("DirStatus().Topology.DataCenters is empty")
	}
	if len(dirStatus.Topology.DataCenters[0].Racks) == 0 {
		t.Fatal("DirStatus().Topology.DataCenters[0].Racks is empty")
	}

	volumeID := strings.Split(assigned.FID, ",")[0]
	volumeIDInt, err := strconv.Atoi(volumeID)
	if err != nil {
		t.Fatalf("assigned volume id = %q, want integer: %v", volumeID, err)
	}
	volumeStatus, err := client.Master().VolumeStatus(ctx)
	if err != nil {
		t.Fatalf("VolumeStatus() error = %v", err)
	}
	if len(volumeStatus.Volumes.DataCenters) == 0 {
		t.Fatal("VolumeStatus().Volumes.DataCenters is empty")
	}
	if !hasVolume(volumeStatus, volumeIDInt) {
		t.Fatalf("VolumeStatus() does not contain assigned volume %d", volumeIDInt)
	}

	lookup, err := client.Master().Lookup(ctx, volumeID, master.LookupOptions{})
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if len(lookup.Locations) == 0 {
		t.Fatal("Lookup().Locations is empty")
	}
}

func hasVolume(status *master.VolumeStatusResponse, volumeID int) bool {
	for _, racks := range status.Volumes.DataCenters {
		for _, nodes := range racks {
			for _, volumes := range nodes {
				for _, volume := range volumes {
					if volume.ID == volumeID {
						return true
					}
				}
			}
		}
	}
	return false
}
