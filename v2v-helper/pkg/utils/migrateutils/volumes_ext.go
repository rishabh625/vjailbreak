package migrateutils

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/platform9/vjailbreak/v2v-helper/vm"
)

// RDMDiskToVolumeManageMap builds the request payload for manage volume.
func RDMDiskToVolumeManageMap(rdmDisk vm.RDMDisk) (map[string]interface{}, error) {
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

// CinderManage Manage triggers the volume manage request and returns volume.
func (osclient *OpenStackClients) CinderManage(rdmDisk vm.RDMDisk, openstackAPIVersion string) (*volumes.Volume, error) {

	body, err := RDMDiskToVolumeManageMap(rdmDisk)
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

	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}

	volumeMap := result["volume"].(map[string]interface{})

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
