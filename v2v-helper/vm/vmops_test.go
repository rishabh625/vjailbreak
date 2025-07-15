// Copyright © 2024 The vjailbreak authors

package vm

import (
	"context"
	"log"
	"net/url"
	"testing"

	"github.com/platform9/vjailbreak/v2v-helper/vcenter"

	"github.com/stretchr/testify/assert"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

func simulateVCenter() (*vcenter.VCenterClient, *simulator.Model, *simulator.Server, error) {
	// Create a new simulator instance
	model := simulator.VPX()
	err := model.Create()
	if err != nil {
		log.Fatal(err)
	}
	server := model.Service.NewServer()

	// Connect to the simulator
	u, err := soap.ParseURL(server.URL.String())
	if err != nil {
		log.Fatal(err)
	}
	u.User = url.UserPassword("user", "pass")
	ctx := context.Background()

	// Create a new client
	client, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		log.Fatal(err)
	}
	return &vcenter.VCenterClient{
		VCClient:            client.Client,
		VCFinder:            find.NewFinder(client.Client, false),
		VCPropertyCollector: property.DefaultCollector(client.Client),
	}, model, server, nil
}

func cleanupSimulator(model *simulator.Model, server *simulator.Server) {
	model.Remove()
	server.Close()
}
func TestGetVMInfo(t *testing.T) {
	simVC, model, server, err := simulateVCenter()
	defer cleanupSimulator(model, server)
	assert.Nil(t, err)

	negone := int64(-1)
	pointzero := int32(0)
	pointf := false
	pointt := true
	vmName := "DC0_H0_VM0"
	expectedVMInfo := VMInfo{
		CPU:    1,
		Memory: 32,
		State:  "poweredOn",
		Mac:    []string{"00:0c:29:36:63:62"},
		IPs:    []string{},
		UUID:   "265104de-1472-547c-b873-6dc7883fb6cb",
		Host:   "host-22",
		VMDisks: []VMDisk{
			{
				Name: "disk-202-0",
				Size: 10737418240,
				Disk: &types.VirtualDisk{
					VirtualDevice: types.VirtualDevice{
						Key: 204,
						DeviceInfo: &types.Description{
							Label:   "disk-202-0",
							Summary: "10,485,760 KB",
						},
						Backing: &types.VirtualDiskFlatVer2BackingInfo{
							VirtualDeviceFileBackingInfo: types.VirtualDeviceFileBackingInfo{
								VirtualDeviceBackingInfo: types.VirtualDeviceBackingInfo{
									DynamicData: types.DynamicData{},
								},
								FileName: "[LocalDS_0] DC0_H0_VM0/disk1.vmdk",
								Datastore: &types.ManagedObjectReference{
									Type:  "Datastore",
									Value: "datastore-60",
								},
							},
							DiskMode:        "persistent",
							Split:           &pointf,
							WriteThrough:    &pointf,
							ThinProvisioned: &pointt,
							EagerlyScrub:    &pointf,
							Uuid:            "0f7d94a1-43f3-5cdd-a5b7-cd730a719f51",
							DigestEnabled:   &pointf,
						},
						ControllerKey: 202,
						UnitNumber:    &pointzero,
					},
					CapacityInKB:    10485760,
					CapacityInBytes: 10737418240,
					StorageIOAllocation: &types.StorageIOAllocationInfo{
						DynamicData: types.DynamicData{},
						Limit:       &negone,
					},
				},
			},
		},
		UEFI:   false,
		Name:   "DC0_H0_VM0",
		OSType: "linux",
	}
	// TODO: Add client  to VMOpsBuilder
	vmops, _ := VMOpsBuilder(context.Background(), *simVC, vmName, nil)

	vminfo, err := vmops.GetVMInfo("linux")
	assert.NoError(t, err)
	assert.Equal(t, expectedVMInfo, vminfo)
}

func TestEnableCBT(t *testing.T) {
	simVC, model, server, err := simulateVCenter()
	defer cleanupSimulator(model, server)
	assert.Nil(t, err)

	vmName := "DC0_H0_VM0"
	// TODO: Add client  to VMOpsBuilder
	vmops, _ := VMOpsBuilder(context.Background(), *simVC, vmName, nil)

	err = vmops.EnableCBT()
	assert.NoError(t, err)
}

func TestIsCBTEnabled(t *testing.T) {
	simVC, model, server, err := simulateVCenter()
	defer cleanupSimulator(model, server)
	assert.Nil(t, err)

	vmName := "DC0_H0_VM0"
	// TODO: Add client  to VMOpsBuilder
	vmops, _ := VMOpsBuilder(context.Background(), *simVC, vmName, nil)

	_ = vmops.EnableCBT()
	enabled, err := vmops.IsCBTEnabled()
	assert.NoError(t, err)
	assert.True(t, enabled)
}

func TestTakeSnapshot(t *testing.T) {
	simVC, model, server, err := simulateVCenter()
	defer cleanupSimulator(model, server)
	assert.Nil(t, err)

	vmName := "DC0_H0_VM0"
	// TODO: Add client  to VMOpsBuilder
	vmops, _ := VMOpsBuilder(context.Background(), *simVC, vmName, nil)

	snapshotName := "snapshot-1"
	err = vmops.TakeSnapshot(snapshotName)
	assert.NoError(t, err)
}

func TestDeleteSnapshot(t *testing.T) {
	simVC, model, server, err := simulateVCenter()
	defer cleanupSimulator(model, server)
	assert.Nil(t, err)

	vmName := "DC0_H0_VM0"
	// TODO: Add client  to VMOpsBuilder
	vmops, _ := VMOpsBuilder(context.Background(), *simVC, vmName, nil)

	snapshotName := "snapshot-1"
	_ = vmops.TakeSnapshot(snapshotName)
	err = vmops.DeleteSnapshot(snapshotName)
	assert.NoError(t, err)
}

func TestGetSnapshot(t *testing.T) {
	simVC, model, server, err := simulateVCenter()
	defer cleanupSimulator(model, server)
	assert.Nil(t, err)

	vmName := "DC0_H0_VM0"
	// TODO: Add client  to VMOpsBuilder
	vmops, _ := VMOpsBuilder(context.Background(), *simVC, vmName, nil)

	snapshotName := "snapshot-1"
	_ = vmops.TakeSnapshot(snapshotName)
	snapshot, err := vmops.GetSnapshot(snapshotName)
	assert.NoError(t, err)
	assert.NotNil(t, snapshot)
}

// Could not make unit tests for CustomQueryChangedDiskAreas and UpdateDiskInfo
// as they rely on change block tracking which is not supported by the simulator
