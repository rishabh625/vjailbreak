package utils

import (
	"fmt"

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
