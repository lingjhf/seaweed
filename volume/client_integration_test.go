//go:build integration

package volume_test

import (
	"context"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lingjhf/seaweed"
	"github.com/lingjhf/seaweed/internal/testweed"
	"github.com/lingjhf/seaweed/master"
	"github.com/lingjhf/seaweed/volume"
)

const volumeJWTSecurityTOML = `
[jwt.signing]
key = "write-secret"
expires_after_seconds = 60

[jwt.signing.read]
key = "read-secret"
expires_after_seconds = 60

[access]
ui = true
`

func TestVolumePutGetDeleteIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolume(t, ctx)
	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{cluster.MasterURL},
		VolumeURLs: []string{cluster.VolumeURL},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := client.Volume().Health(ctx); err != nil {
		t.Fatalf("Health() error = %v", err)
	}

	assigned, err := client.Master().Assign(ctx, seaweedMasterAssignOptions())
	if err != nil {
		t.Fatalf("Assign() error = %v", err)
	}
	_, err = client.Volume().Put(ctx, assigned.FID, strings.NewReader("volume-data"), seaweedVolumePutOptions())
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	status, err := client.Volume().Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Version == "" {
		t.Fatal("Status().Version is empty")
	}
	if len(status.DiskStatuses) == 0 {
		t.Fatal("Status().DiskStatuses is empty")
	}
	volumeID, err := strconv.Atoi(strings.Split(assigned.FID, ",")[0])
	if err != nil {
		t.Fatalf("assigned volume id = %q, want integer: %v", assigned.FID, err)
	}
	if !hasVolume(status, volumeID) {
		t.Fatalf("Status().Volumes does not contain assigned volume %d", volumeID)
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

	header, err := client.Volume().Head(ctx, assigned.FID, volume.HeadOptions{})
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if header.Get("ETag") == "" {
		t.Fatal("Head().ETag is empty")
	}

	if err := client.Volume().Delete(ctx, assigned.FID, volume.DeleteOptions{}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestVolumeAuthorizationIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cluster := testweed.StartMasterVolumeWithSecurity(t, ctx, volumeJWTSecurityTOML)
	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{cluster.MasterURL},
		VolumeURLs: []string{cluster.VolumeURL},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	assigned, err := client.Master().Assign(ctx, seaweedMasterAssignOptions())
	if err != nil {
		t.Fatalf("Assign() error = %v", err)
	}
	if assigned.Authorization == "" {
		t.Fatal("Assign().Authorization is empty")
	}
	if _, err := client.Volume().Put(ctx, assigned.FID, strings.NewReader("secure-volume-data"), volume.PutOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("secure-volume-data")),
	}); err == nil {
		t.Fatal("Put() without Authorization error = nil, want authorization error")
	}
	if _, err := client.Volume().Put(ctx, assigned.FID, strings.NewReader("secure-volume-data"), volume.PutOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("secure-volume-data")),
		Authorization: assigned.Authorization,
	}); err != nil {
		t.Fatalf("Put() with Authorization error = %v", err)
	}

	volumeID := strings.Split(assigned.FID, ",")[0]
	readLookup, err := client.Master().Lookup(ctx, volumeID, master.LookupOptions{
		FileID: assigned.FID,
		Read:   true,
	})
	if err != nil {
		t.Fatalf("Lookup(read) error = %v", err)
	}
	if readLookup.Authorization == "" {
		t.Fatal("Lookup(read).Authorization is empty")
	}
	resp, err := client.Volume().Get(ctx, assigned.FID, volume.GetOptions{Authorization: readLookup.Authorization})
	if err != nil {
		t.Fatalf("Get() with Authorization error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "secure-volume-data" {
		t.Fatalf("body = %q, want secure-volume-data", body)
	}

	deleteLookup, err := client.Master().Lookup(ctx, volumeID, master.LookupOptions{FileID: assigned.FID})
	if err != nil {
		t.Fatalf("Lookup(delete) error = %v", err)
	}
	if deleteLookup.Authorization == "" {
		t.Fatal("Lookup(delete).Authorization is empty")
	}
	if err := client.Volume().Delete(ctx, assigned.FID, volume.DeleteOptions{Authorization: deleteLookup.Authorization}); err != nil {
		t.Fatalf("Delete() with Authorization error = %v", err)
	}
}

func hasVolume(status *volume.StatusResponse, volumeID int) bool {
	for _, vol := range status.Volumes {
		if vol.ID == volumeID {
			return true
		}
	}
	return false
}
