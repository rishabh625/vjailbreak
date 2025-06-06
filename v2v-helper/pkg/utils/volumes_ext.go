package utils

import (
	"fmt"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/platform9/vjailbreak/v2v-helper/vm"
)

// ToVolumeManageMap builds the request payload for manage volume.
func ToVolumeManageMap(rdmDisk vm.RDMDisk) (map[string]interface{}, error) {
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

// Manage triggers the volume manage request.
func (osclient *OpenStackClients) CinderManage(rdmDisk vm.RDMDisk) (*volumes.Volume, error) {
	body, err := ToVolumeManageMap(rdmDisk)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}

	response, err := osclient.BlockStorageClient.Post(osclient.BlockStorageClient.ServiceURL("manageable_volumes"), body, &result, &gophercloud.RequestOpts{
		OkCodes:     []int{202},
		MoreHeaders: map[string]string{"OpenStack-API-Version": "volume 3.8"},
	})
	if err != nil {
		return nil, err
	}

	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}

	volume, _ := result["volume"].(map[string]interface{})

	id := volume["id"]
	v := volumes.Volume{
		ID: id.(string),
	}
	return &v, nil
}
