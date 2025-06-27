package utils

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type MigrationParams struct {
	// vCenter params
	SourceVMName string

	// openstack params
	OpenstackNetworkNames string
	OpenstackNetworkPorts string
	OpenstackVolumeTypes  string
	OpenstackVirtioWin    string
	OpenstackOSType       string
	OpenstackConvert      bool

	// Migration params
	DataCopyStart          string
	VMcutoverStart         string
	VMcutoverEnd           string
	MigrationType          string
	PerformHealthChecks    bool
	HealthCheckPort        string
	Debug                  bool
	TARGET_FLAVOR_ID       string
	TargetAvailabilityZone string
	AssignedIP             string
}

// GetMigrationConfigMapName is function that returns the name of the secret
func GetMigrationConfigMapName(vmname string) (string, error) {
	vmname, err := ConvertToK8sName(vmname)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("migration-config-%s", vmname), nil
}

// GetMigrationParams is function that returns the migration parameters
func GetMigrationParams(ctx context.Context, client client.Client) (*MigrationParams, error) {
	// Get the values from the secret
	configMapName, err := GetMigrationConfigMapName(os.Getenv("SOURCE_VM_NAME"))
	if err != nil {
		return nil, err
	}
	configMap := &v1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: constants.MigrationSystemNamespace,
	}, configMap)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get configmap")
	}
	return &MigrationParams{
		SourceVMName:           os.Getenv("SOURCE_VM_NAME"),
		OpenstackNetworkNames:  string(configMap.Data["NEUTRON_NETWORK_NAMES"]),
		OpenstackNetworkPorts:  string(configMap.Data["NEUTRON_PORT_IDS"]),
		OpenstackVolumeTypes:   string(configMap.Data["CINDER_VOLUME_TYPES"]),
		OpenstackVirtioWin:     string(configMap.Data["VIRTIO_WIN_DRIVER"]),
		OpenstackOSType:        string(configMap.Data["OS_FAMILY"]),
		OpenstackConvert:       string(configMap.Data["CONVERT"]) == constants.TrueString,
		DataCopyStart:          string(configMap.Data["DATACOPYSTART"]),
		VMcutoverStart:         string(configMap.Data["CUTOVERSTART"]),
		VMcutoverEnd:           string(configMap.Data["CUTOVEREND"]),
		MigrationType:          string(configMap.Data["TYPE"]),
		PerformHealthChecks:    string(configMap.Data["PERFORM_HEALTH_CHECKS"]) == constants.TrueString,
		HealthCheckPort:        string(configMap.Data["HEALTH_CHECK_PORT"]),
		Debug:                  string(configMap.Data["DEBUG"]) == constants.TrueString,
		TARGET_FLAVOR_ID:       string(configMap.Data["TARGET_FLAVOR_ID"]),
		TargetAvailabilityZone: string(configMap.Data["TARGET_AVAILABILITY_ZONE"]),
		AssignedIP:             string(configMap.Data["ASSIGNED_IP"]),
	}, nil
}
