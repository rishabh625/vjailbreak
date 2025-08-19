package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/utils/migrateutils"
	"github.com/platform9/vjailbreak/v2v-helper/vm"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// VolumePayload represents the payload structure for managing a volume in OpenStack.
type VolumePayload struct {
	Volume Volume `json:"volume"`
}

// Volume represents the structure of a volume in OpenStack.
type Volume struct {
	Host             string            `json:"host"`
	Ref              map[string]string `json:"ref"`
	Name             string            `json:"name"`
	VolumeType       string            `json:"volume_type"`
	Description      string            `json:"description"`
	Bootable         bool              `json:"bootable"`
	AvailabilityZone interface{}       `json:"availability_zone,omitempty"`
}

// ImportLUNToCinder imports a LUN into OpenStack Cinder and returns the volume ID.
func ImportLUNToCinder(ctx context.Context, openstackClient *migrateutils.OpenStackClients, rdmDisk vm.RDMDisk, volumeAPIVersion string) (string, error) {
	ctxlog := logf.FromContext(ctx)
	ctxlog.Info("Importing LUN", "DiskName", rdmDisk.DiskName)

	volume, err := ExecuteVolumeManageRequest(ctx, rdmDisk, openstackClient, volumeAPIVersion)
	if err != nil {
		return "", fmt.Errorf("failed to import LUN %s: %w", rdmDisk.DiskName, err)
	}
	if volume == nil || volume.ID == "" {
		return "", fmt.Errorf("failed to import LUN %s: received empty volume ID", rdmDisk.DiskName)
	}

	ctxlog.Info("LUN imported successfully, waiting for volume to become available", "VolumeID", volume.ID)

	// Wait for the volume to become available
	err = openstackClient.WaitForVolume(volume.ID)
	if err != nil {
		return "", fmt.Errorf("failed to wait for volume %s to become available: %w", volume.ID, err)
	}

	ctxlog.Info("Volume is now available", "VolumeID", volume.ID)
	return volume.ID, nil
}

// BuildVolumeManagePayload builds the request payload for manage volume.
func BuildVolumeManagePayload(rdmDisk vm.RDMDisk) (map[string]interface{}, error) {
	// Validate required fields
	if rdmDisk.DiskName == "" {
		return nil, fmt.Errorf("disk name cannot be empty")
	}
	if rdmDisk.CinderBackendPool == "" {
		return nil, fmt.Errorf("cinder backend pool cannot be empty")
	}
	if rdmDisk.VolumeType == "" {
		return nil, fmt.Errorf("volume type cannot be empty")
	}
	if len(rdmDisk.VolumeRef) == 0 {
		return nil, fmt.Errorf("volume reference cannot be empty")
	}

	// Assumes only one key-value pair in VolumeRef
	var key, value string
	for k, rm := range rdmDisk.VolumeRef {
		key = k
		value = rm
		break
	}

	volumePayload := VolumePayload{
		Volume: Volume{
			Host:             rdmDisk.CinderBackendPool,
			Ref:              map[string]string{key: value},
			Name:             rdmDisk.DiskName,
			VolumeType:       rdmDisk.VolumeType,
			Description:      "Volume for " + rdmDisk.DiskName,
			Bootable:         false,
			AvailabilityZone: nil,
		},
	}

	var payload map[string]interface{}
	data, err := json.Marshal(volumePayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal volume payload: %w", err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal volume payload: %w", err)
	}
	return payload, nil
}

// ExecuteVolumeManageRequest triggers the volume manage request and returns volume.
func ExecuteVolumeManageRequest(ctx context.Context, rdmDisk vm.RDMDisk, osclient *migrateutils.OpenStackClients, openstackAPIVersion string) (*volumes.Volume, error) {
	body, err := BuildVolumeManagePayload(rdmDisk)
	if err != nil {
		return nil, fmt.Errorf("failed to build volume manage payload: %w", err)
	}

	var result map[string]interface{}
	response, err := osclient.BlockStorageClient.Post(osclient.BlockStorageClient.ServiceURL("manageable_volumes"), body, &result, &gophercloud.RequestOpts{
		OkCodes:     []int{http.StatusAccepted},
		MoreHeaders: map[string]string{"OpenStack-API-Version": openstackAPIVersion},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute volume manage request: %w", err)
	}

	if response != nil && response.Body != nil {
		defer func() {
			if err := response.Body.Close(); err != nil {
				logf.FromContext(ctx).Error(err, "failed to close response body")
			}
		}()
	}

	volumeMap, ok := result["volume"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to assert type for volume map")
	}

	volumeJSON, err := json.Marshal(volumeMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal volume map to JSON: %w", err)
	}

	var v volumes.Volume
	if err := json.Unmarshal(volumeJSON, &v); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to volume struct: %w", err)
	}

	return &v, nil
}
