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

// ImportLUNToCinder imports a LUN into OpenStack Cinder and returns the volume ID.
func ImportLUNToCinder(ctx context.Context, openstackClient *migrateutils.OpenStackClients, rdmDisk vm.RDMDisk) (string, error) {
	ctxlog := logf.FromContext(ctx)
	ctxlog.Info(fmt.Sprintf("Importing LUN: %s", rdmDisk.DiskName))
	volume, err := ExecuteVolumeManageRequest(ctx, rdmDisk, openstackClient, "volume 3.8")
	if err != nil || volume == nil {
		return "", fmt.Errorf("failed to import LUN: %s", err)
	} else if volume.ID == "" {
		return "", fmt.Errorf("failed to import LUN: received empty volume ID")
	}
	ctxlog.Info(fmt.Sprintf("LUN imported successfully, waiting for volume %s to become available", volume.ID))
	// Wait for the volume to become available
	err = openstackClient.WaitForVolume(volume.ID)
	if err != nil {
		return "", fmt.Errorf("failed to wait for volume to become available: %s", err)
	}
	ctxlog.Info(fmt.Sprintf("Volume %s is now available", volume.ID))
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

	var key, value string
	for k, rm := range rdmDisk.VolumeRef {
		key = k
		value = rm
	}
	payload := map[string]interface{}{
		"volume": map[string]interface{}{
			"host": rdmDisk.CinderBackendPool,
			"ref": map[string]string{
				key: value,
			},
			"name":              rdmDisk.DiskName,
			"volume_type":       rdmDisk.VolumeType,
			"description":       fmt.Sprintf("Volume for %s", rdmDisk.DiskName),
			"bootable":          false,
			"availability_zone": nil,
		},
	}
	return payload, nil
}

// ExecuteVolumeManageRequest triggers the volume manage request and returns volume.
func ExecuteVolumeManageRequest(ctx context.Context, rdmDisk vm.RDMDisk, osclient *migrateutils.OpenStackClients, openstackAPIVersion string) (*volumes.Volume, error) {
	body, err := BuildVolumeManagePayload(rdmDisk)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}

	response, err := osclient.BlockStorageClient.Post(osclient.BlockStorageClient.ServiceURL("manageable_volumes"), body, &result, &gophercloud.RequestOpts{
		OkCodes:     []int{http.StatusAccepted},
		MoreHeaders: map[string]string{"OpenStack-API-Version": openstackAPIVersion},
	})
	if err != nil {
		return nil, err
	}
	// Add error handling for response.Body.Close()
	if response != nil && response.Body != nil {
		defer func() {
			if err := response.Body.Close(); err != nil {
				logf.FromContext(ctx).Error(err, "failed to close response body")
			}
		}()
	}
	// Add error handling for type assertion
	volumeMap, ok := result["volume"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to assert type for volume map")
	}
	// Convert volume map to JSON
	volumeJSON, err := json.Marshal(volumeMap)
	if err != nil {
		return nil, err
	}
	// Unmarshal JSON into your struct
	var v volumes.Volume
	if err := json.Unmarshal(volumeJSON, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
