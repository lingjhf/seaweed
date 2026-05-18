//go:build integration

package volume_test

import (
	"github.com/lingjhf/seaweed/master"
	"github.com/lingjhf/seaweed/volume"
)

func seaweedMasterAssignOptions() master.AssignOptions {
	return master.AssignOptions{}
}

func seaweedVolumePutOptions() volume.PutOptions {
	return volume.PutOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("volume-data")),
	}
}

func seaweedVolumeGetOptions() volume.GetOptions {
	return volume.GetOptions{}
}
