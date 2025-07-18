// Copyright © 2024 The vjailbreak authors

package migrate

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/pkg/errors"
	"github.com/platform9/vjailbreak/v2v-helper/nbd"
	"github.com/platform9/vjailbreak/v2v-helper/openstack"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/utils"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/utils/migrateutils"
	"github.com/platform9/vjailbreak/v2v-helper/vcenter"
	"github.com/platform9/vjailbreak/v2v-helper/virtv2v"
	"github.com/platform9/vjailbreak/v2v-helper/vm"
	"sigs.k8s.io/controller-runtime/pkg/client"

	probing "github.com/prometheus-community/pro-bing"
	"github.com/vmware/govmomi/vim25/types"
)

type Migrate struct {
	URL                    string
	UserName               string
	Password               string
	Insecure               bool
	Networknames           []string
	Networkports           []string
	Volumetypes            []string
	Virtiowin              string
	Ostype                 string
	Thumbprint             string
	Convert                bool
	Openstackclients       openstack.OpenstackOperations
	Vcclient               vcenter.VCenterOperations
	VMops                  vm.VMOperations
	Nbdops                 []nbd.NBDOperations
	EventReporter          chan string
	PodLabelWatcher        chan string
	InPod                  bool
	MigrationTimes         MigrationTimes
	MigrationType          string
	PerformHealthChecks    bool
	HealthCheckPort        string
	K8sClient              client.Client
	TargetFlavorId         string
	TargetAvailabilityZone string
	AssignedIP             string
}

type MigrationTimes struct {
	DataCopyStart  time.Time
	VMCutoverStart time.Time
	VMCutoverEnd   time.Time
}

func (migobj *Migrate) logMessage(message string) {
	if migobj.InPod {
		migobj.EventReporter <- message
	}
	utils.PrintLog(message)
}

// This function creates volumes in OpenStack and attaches them to the helper vm
func (migobj *Migrate) CreateVolumes(vminfo vm.VMInfo) (vm.VMInfo, error) {
	openstackops := migobj.Openstackclients
	migobj.logMessage("Creating volumes in OpenStack")
	for idx, vmdisk := range vminfo.VMDisks {
		volume, err := openstackops.CreateVolume(vminfo.Name+"-"+vmdisk.Name, vmdisk.Size, vminfo.OSType, vminfo.UEFI, migobj.Volumetypes[idx])
		if err != nil {
			return vminfo, fmt.Errorf("failed to create volume: %s", err)
		}
		vminfo.VMDisks[idx].OpenstackVol = volume
		if vminfo.VMDisks[idx].Boot {
			err = openstackops.SetVolumeBootable(volume)
			if err != nil {
				return vminfo, fmt.Errorf("failed to set volume as bootable: %s", err)
			}
		}
	}
	migobj.logMessage("Volumes created successfully")
	return vminfo, nil
}

func (migobj *Migrate) AttachVolume(disk vm.VMDisk) (string, error) {
	openstackops := migobj.Openstackclients
	migobj.logMessage("Attaching volumes to VM")
	if disk.OpenstackVol == nil {
		return "", fmt.Errorf("OpenStack volume is nil")
	}
	volumeID := disk.OpenstackVol.ID
	if err := openstackops.AttachVolumeToVM(volumeID); err != nil {
		return "", errors.Wrap(err, "failed to attach volume to VM")
	}

	// Get the Path of the attached volume
	devicePath, err := openstackops.FindDevice(volumeID)
	if err != nil {
		return "", fmt.Errorf("failed to find device: %s", err)
	}
	return devicePath, nil
}

func (migobj *Migrate) DetachVolume(disk vm.VMDisk) error {
	openstackops := migobj.Openstackclients

	if err := openstackops.DetachVolumeFromVM(disk.OpenstackVol.ID); err != nil {
		return errors.Wrap(err, "failed to detach volume from VM")
	}

	err := openstackops.WaitForVolume(disk.OpenstackVol.ID)
	if err != nil {
		return fmt.Errorf("failed to wait for volume to become available: %s", err)
	}
	return nil
}

func (migobj *Migrate) DetachAllVolumes(vminfo vm.VMInfo) error {
	openstackops := migobj.Openstackclients
	for _, vmdisk := range vminfo.VMDisks {

		if err := openstackops.DetachVolumeFromVM(vmdisk.OpenstackVol.ID); err != nil && !strings.Contains(err.Error(), "is not attached to volume") {
			return errors.Wrap(err, "failed to detach volume from VM")
		}

		err := openstackops.WaitForVolume(vmdisk.OpenstackVol.ID)
		if err != nil {
			return fmt.Errorf("failed to wait for volume to become available: %s", err)
		}
		log.Printf("Volume %s detached from VM\n", vmdisk.Name)
	}
	time.Sleep(1 * time.Second)
	return nil
}

func (migobj *Migrate) DeleteAllVolumes(vminfo vm.VMInfo) error {
	openstackops := migobj.Openstackclients
	for _, vmdisk := range vminfo.VMDisks {
		err := openstackops.DeleteVolume(vmdisk.OpenstackVol.ID)
		if err != nil {
			return fmt.Errorf("failed to delete volume: %s", err)
		}
		utils.PrintLog(fmt.Sprintf("Volume %s deleted\n", vmdisk.Name))
	}
	return nil
}

// This function enables CBT on the VM if it is not enabled and takes a snapshot for initializing CBT
func (migobj *Migrate) EnableCBTWrapper() error {
	vmops := migobj.VMops
	cbt, err := vmops.IsCBTEnabled()
	if err != nil {
		return errors.Wrap(err, "failed to check if CBT is enabled")
	}
	migobj.logMessage(fmt.Sprintf("CBT Enabled: %t", cbt))

	if !cbt {
		// 7.5. Enable CBT
		migobj.logMessage("CBT is not enabled. Enabling CBT")
		err = vmops.EnableCBT()
		if err != nil {
			return errors.Wrap(err, "failed to enable CBT")
		}
		_, err := vmops.IsCBTEnabled()
		if err != nil {
			return fmt.Errorf("failed to check if CBT is enabled: %s", err)
		}
		migobj.logMessage("Creating temporary snapshot of the source VM")
		err = vmops.TakeSnapshot("tmp-snap")
		if err != nil {
			return fmt.Errorf("failed to take snapshot of source VM: %s", err)
		}
		utils.PrintLog("Snapshot created successfully")
		err = vmops.DeleteSnapshot("tmp-snap")
		if err != nil {
			return fmt.Errorf("failed to delete snapshot of source VM: %s", err)
		}
		utils.PrintLog("Snapshot deleted successfully")
		migobj.logMessage("CBT enabled successfully")
	}
	return nil
}

func (migobj *Migrate) WaitforCutover() error {
	var zerotime time.Time
	if !migobj.MigrationTimes.VMCutoverStart.Equal(zerotime) && migobj.MigrationTimes.VMCutoverStart.After(time.Now()) {
		migobj.logMessage("Waiting for VM Cutover start time")
		time.Sleep(time.Until(migobj.MigrationTimes.VMCutoverStart))
		migobj.logMessage("VM Cutover start time reached")
	} else {
		if !migobj.MigrationTimes.VMCutoverEnd.Equal(zerotime) && migobj.MigrationTimes.VMCutoverEnd.Before(time.Now()) {
			return fmt.Errorf("VM Cutover End time has already passed")
		}
	}
	return nil
}

func (migobj *Migrate) WaitforAdminCutover() error {
	migobj.logMessage("Waiting for Cutover conditions to be met")
	for {
		label := <-migobj.PodLabelWatcher
		migobj.logMessage(fmt.Sprintf("Label: %s", label))
		if label == "yes" {
			break
		}
	}
	migobj.logMessage("Cutover conditions met")
	return nil
}

func (migobj *Migrate) LiveReplicateDisks(ctx context.Context, vminfo vm.VMInfo) (vm.VMInfo, error) {
	vmops := migobj.VMops
	nbdops := migobj.Nbdops
	envURL := migobj.URL
	envUserName := migobj.UserName
	envPassword := migobj.Password
	thumbprint := migobj.Thumbprint

	if migobj.MigrationType == "cold" {
		if err := vmops.VMPowerOff(); err != nil {
			return vminfo, fmt.Errorf("failed to power off VM: %s", err)
		}
	}

	// clean up snapshots
	utils.PrintLog("Cleaning up snapshots before copy")
	err := vmops.CleanUpSnapshots(false)
	if err != nil {
		return vminfo, fmt.Errorf("failed to clean up snapshots: %s, please delete manually before starting again", err)
	}

	utils.PrintLog("Starting NBD server")
	err = vmops.TakeSnapshot(constants.MigrationSnapshotName)
	if err != nil {
		return vminfo, fmt.Errorf("failed to take snapshot of source VM: %s", err)
	}

	err = vmops.UpdateDisksInfo(&vminfo)
	if err != nil {
		return vminfo, fmt.Errorf("failed to update disk info: %s", err)
	}

	for idx, vmdisk := range vminfo.VMDisks {
		migobj.logMessage(fmt.Sprintf("Copying disk %d, Completed: 0%%", idx))
		err = nbdops[idx].StartNBDServer(vmops.GetVMObj(), envURL, envUserName, envPassword, thumbprint, vmdisk.Snapname, vmdisk.SnapBackingDisk, migobj.EventReporter)
		if err != nil {
			return vminfo, fmt.Errorf("failed to start NBD server: %s", err)
		}
	}
	// sleep for 2 seconds to allow the NBD server to start
	time.Sleep(2 * time.Second)
	final := false

	for idx, vmdisk := range vminfo.VMDisks {
		vminfo.VMDisks[idx].Path, err = migobj.AttachVolume(vmdisk)
		if err != nil {
			return vminfo, fmt.Errorf("failed to attach volume: %s", err)
		}
	}

	incrementalCopyCount := 0
	for {
		// If its the first copy, copy the entire disk
		if incrementalCopyCount == 0 {
			for idx := range vminfo.VMDisks {
				err = nbdops[idx].CopyDisk(ctx, vminfo.VMDisks[idx].Path, idx)
				if err != nil {
					return vminfo, fmt.Errorf("failed to copy disk: %s", err)
				}
				migobj.logMessage(fmt.Sprintf("Disk %d copied successfully: %s", idx, vminfo.VMDisks[idx].Path))
			}
		} else {
			migration_snapshot, err := vmops.GetSnapshot(constants.MigrationSnapshotName)
			if err != nil {
				return vminfo, fmt.Errorf("failed to get snapshot: %s", err)
			}

			var changedAreas types.DiskChangeInfo
			done := true

			for idx := range vminfo.VMDisks {
				changedAreas, err = vmops.CustomQueryChangedDiskAreas(vminfo.VMDisks[idx].ChangeID, migration_snapshot, vminfo.VMDisks[idx].Disk, 0)
				if err != nil {
					return vminfo, fmt.Errorf("failed to get changed disk areas: %s", err)
				}

				if len(changedAreas.ChangedArea) == 0 {
					migobj.logMessage(fmt.Sprintf("Disk %d: No changed blocks found. Skipping copy", idx))
				} else {
					migobj.logMessage(fmt.Sprintf("Disk %d: Blocks have Changed.", idx))

					utils.PrintLog("Restarting NBD server")
					err = nbdops[idx].StopNBDServer()
					if err != nil {
						return vminfo, fmt.Errorf("failed to stop NBD server: %s", err)
					}

					err = nbdops[idx].StartNBDServer(vmops.GetVMObj(), envURL, envUserName, envPassword, thumbprint, vminfo.VMDisks[idx].Snapname, vminfo.VMDisks[idx].SnapBackingDisk, migobj.EventReporter)
					if err != nil {
						return vminfo, fmt.Errorf("failed to start NBD server: %s", err)
					}
					// sleep for 2 seconds to allow the NBD server to start
					time.Sleep(2 * time.Second)

					// 11. Copy Changed Blocks over
					done = false
					changedBlockCopySuccess := true
					migobj.logMessage("Copying changed blocks")

					err = nbdops[idx].CopyChangedBlocks(ctx, changedAreas, vminfo.VMDisks[idx].Path)
					if err != nil {
						changedBlockCopySuccess = false
					}

					err = vmops.UpdateDiskInfo(&vminfo, vminfo.VMDisks[idx], changedBlockCopySuccess)
					if err != nil {
						return vminfo, fmt.Errorf("failed to update disk info: %s", err)
					}
					if !changedBlockCopySuccess {
						migobj.logMessage(fmt.Sprintf("Failed to copy changed blocks: %s", err))
						migobj.logMessage(fmt.Sprintf("Since full copy has completed, Retrying copy of changed block	s for disk: %d", idx))
					}
					migobj.logMessage("Finished copying changed blocks")
					migobj.logMessage(fmt.Sprintf("Syncing Changed blocks [%d/20]", incrementalCopyCount))
				}
			}
			if final {
				break
			}
			if done || incrementalCopyCount > 20 {
				utils.PrintLog("Shutting down source VM and performing final copy")
				if err := migobj.WaitforCutover(); err != nil {
					return vminfo, fmt.Errorf("failed to start VM Cutover: %s", err)
				}
				if err := migobj.WaitforAdminCutover(); err != nil {
					return vminfo, fmt.Errorf("failed to start Admin initated Cutover: %s", err)
				}
				err = vmops.VMPowerOff()
				if err != nil {
					return vminfo, fmt.Errorf("failed to power off VM: %s", err)
				}
				final = true
			}

		}

		// Update old change id to the new base change id value
		// Only do this after you have gone through all disks with old change id.
		// If you dont, only your first disk will have the updated changes

		err = vmops.CleanUpSnapshots(false)
		if err != nil {
			return vminfo, fmt.Errorf("failed to delete snapshot of source VM: %s", err)
		}
		err = vmops.TakeSnapshot(constants.MigrationSnapshotName)
		if err != nil {
			return vminfo, fmt.Errorf("failed to take snapshot of source VM: %s", err)
		}

		incrementalCopyCount += 1

	}

	err = migobj.DetachAllVolumes(vminfo)
	if err != nil {
		return vminfo, errors.Wrap(err, "Failed to detach all volumes from VM")
	}

	utils.PrintLog("Stopping NBD server")
	for _, nbdserver := range nbdops {
		err = nbdserver.StopNBDServer()
		if err != nil {
			return vminfo, fmt.Errorf("failed to stop NBD server: %s", err)
		}
	}

	utils.PrintLog("Deleting migration snapshot")
	err = vmops.CleanUpSnapshots(true)
	if err != nil {
		migobj.logMessage(fmt.Sprintf(`Failed to delete snapshot of source VM: %s, since copy is completed, 
		continuing with the migration`, err))
	}
	return vminfo, nil
}

func (migobj *Migrate) ConvertVolumes(ctx context.Context, vminfo vm.VMInfo) error {
	migobj.logMessage("Converting disk")

	var (
		osRelease                   = ""
		bootVolumeIndex             = -1
		err                         error
		lvm, osPath, getBootCommand string
		useSingleDisk               bool
	)

	if strings.ToLower(vminfo.OSType) == constants.OSFamilyWindows {
		getBootCommand = "ls /Windows"
	} else if strings.ToLower(vminfo.OSType) == constants.OSFamilyLinux {
		getBootCommand = "ls /boot"
	} else {
		getBootCommand = "inspect-os"
	}

	// attach all volumes at once
	for idx, vmdisk := range vminfo.VMDisks {
		vminfo.VMDisks[idx].Path, err = migobj.AttachVolume(vmdisk)
		if err != nil {
			return fmt.Errorf("failed to attach volume: %s", err)
		}
	}

	// create XML for conversion
	err = migrateutils.GenerateXMLConfig(vminfo)
	if err != nil {
		return fmt.Errorf("failed to generate XML: %s", err)
	}

	for idx := range vminfo.VMDisks {
		// check if individual disks are bootable
		ans, err := virtv2v.RunCommandInGuest(vminfo.VMDisks[idx].Path, getBootCommand, false)
		if err != nil {
			utils.PrintLog(fmt.Sprintf("Error running '%s'. Error: '%s', Output: %s\n", getBootCommand, err, strings.TrimSpace(ans)))
			continue
		}

		if ans == "" {
			// OS is not installed on this disk
			continue
		}
		utils.PrintLog(fmt.Sprintf("Output from '%s' - '%s'\n", getBootCommand, strings.TrimSpace(ans)))

		osPath = strings.TrimSpace(ans)
		bootVolumeIndex = idx
		useSingleDisk = true
		break
	}

	if strings.ToLower(vminfo.OSType) == constants.OSFamilyLinux {
		if useSingleDisk {
			// skip checking LVM, because its a single disk
			osRelease, err = virtv2v.GetOsRelease(vminfo.VMDisks[bootVolumeIndex].Path)
			if err != nil {
				return fmt.Errorf("failed to get os release: %s", err)
			}
		} else {
			// check for LVM
			lvm, err = virtv2v.CheckForLVM(vminfo.VMDisks)
			if err != nil || lvm == "" {
				return errors.Wrap(err, "OS install location not found, Failed to check for LVM")
			}
			osPath = strings.TrimSpace(lvm)
			// check for bootable volume in case of LVM
			bootVolumeIndex, err = virtv2v.GetBootableVolumeIndex(vminfo.VMDisks)
			if err != nil {
				return errors.Wrap(err, "Failed to get bootable volume index")
			}
			osRelease, err = virtv2v.RunCommandInGuestAllVolumes(vminfo.VMDisks, "cat", false, "/etc/os-release")
			if err != nil {
				return fmt.Errorf("failed to get os release: %s: %s\n", err, strings.TrimSpace(osRelease))
			}
		}
		osDetected := strings.ToLower(strings.TrimSpace(osRelease))
		utils.PrintLog(fmt.Sprintf("OS detected by guestfish: %s", osDetected))
		// Supported OSes
		supportedOS := []string{
			"redhat",
			"red hat",
			"rhel",
			"centos",
			"scientific linux",
			"oracle linux",
			"fedora",
			"sles",
			"sled",
			"opensuse",
			"alt linux",
			"debian",
			"ubuntu",
		}

		supported := false
		for _, s := range supportedOS {
			if strings.Contains(osDetected, s) {
				supported = true
				break
			}
		}

		if !supported {
			return fmt.Errorf("unsupported OS detected by guestfish: %s", osDetected)
		}
		utils.PrintLog("OS compatibility check passed")

	} else if strings.ToLower(vminfo.OSType) == constants.OSFamilyWindows {
		utils.PrintLog("OS compatibility check passed")
	} else {
		return fmt.Errorf("unsupported OS type: %s", vminfo.OSType)
	}

	if bootVolumeIndex == -1 {
		return fmt.Errorf("boot volume not found, cannot create target VM")
	}

	// save the index of bootVolume
	utils.PrintLog(fmt.Sprintf("Setting up boot volume as: %s", vminfo.VMDisks[bootVolumeIndex].Name))
	vminfo.VMDisks[bootVolumeIndex].Boot = true
	if migobj.Convert {
		firstbootscripts := []string{}
		// Fix NTFS
		if strings.ToLower(vminfo.OSType) == constants.OSFamilyWindows {
			err = virtv2v.NTFSFix(vminfo.VMDisks[bootVolumeIndex].Path)
			if err != nil {
				return fmt.Errorf("failed to run ntfsfix: %s", err)
			}
		}
		// Turn on DHCP for interfaces in rhel VMs
		if strings.ToLower(vminfo.OSType) == constants.OSFamilyLinux {
			if strings.Contains(osRelease, "rhel") {
				firstbootscriptname := "rhel_enable_dhcp"
				firstbootscript := constants.RhelFirstBootScript
				firstbootscripts = append(firstbootscripts, firstbootscriptname)
				err = virtv2v.AddFirstBootScript(firstbootscript, firstbootscriptname)
				if err != nil {
					return fmt.Errorf("failed to add first boot script: %s", err)
				}
			}
		}

		err := virtv2v.ConvertDisk(ctx, constants.XMLFileName, osPath, vminfo.OSType, migobj.Virtiowin, firstbootscripts, useSingleDisk, vminfo.VMDisks[bootVolumeIndex].Path)
		if err != nil {
			return fmt.Errorf("failed to run virt-v2v: %s", err)
		}

		openstackops := migobj.Openstackclients
		err = openstackops.SetVolumeBootable(vminfo.VMDisks[bootVolumeIndex].OpenstackVol)
		if err != nil {
			return fmt.Errorf("failed to set volume as bootable: %s", err)
		}
	}

	//TODO(omkar): can disable DHCP here
	if strings.ToLower(vminfo.OSType) == constants.OSFamilyLinux {
		if strings.Contains(osRelease, "ubuntu") {
			// Check if netplan is supported
			versionID := parseVersionID(osRelease)
			utils.PrintLog(fmt.Sprintf("Version ID: %s", versionID))
			if versionID == "" {
				return fmt.Errorf("failed to get version ID")
			}
			if isNetplanSupported(versionID) {
				// Add Wildcard Netplan
				utils.PrintLog("Adding wildcard netplan")
				err := virtv2v.AddWildcardNetplan(vminfo.VMDisks, useSingleDisk, vminfo.VMDisks[bootVolumeIndex].Path)
				if err != nil {
					return fmt.Errorf("failed to add wildcard netplan: %s", err)
				}
				utils.PrintLog("Wildcard netplan added successfully")
			} else {
				utils.PrintLog("Ubuntu version does not support netplan, going to use udev rules")
				// Since netplan is not supported need to get the ip,mac and network interface mapping
				// To inject udev rules so that after migration the network interfaces names are consistent
				// and they get the correct ip address.
				// Get the network interface mapping from /etc/network/interfaces

				interfaces, err := virtv2v.GetNetworkInterfaceNames(vminfo.VMDisks[bootVolumeIndex].Path)
				if err != nil {
					return fmt.Errorf("failed to get network interface names: %s", err)
				}
				if len(interfaces) == 0 {
					log.Printf("Failed to get network interface names, cannot add udev rules, network might not come up post migration, please check the network configuration post migration")
				} else {
					utils.PrintLog("Adding udev rules")
					utils.PrintLog(fmt.Sprintf("Interfaces: %v", interfaces))
					macs := []string{}

					// By default the network interfaces macs are in the same order as the interfaces
					for _, nic := range vminfo.NetworkInterfaces {
						macs = append(macs, nic.MAC)
					}
					utils.PrintLog(fmt.Sprintf("MACs: %v", macs))
					err = virtv2v.AddUdevRules(vminfo.VMDisks, useSingleDisk, vminfo.VMDisks[bootVolumeIndex].Path, interfaces, macs)
					if err != nil {
						log.Printf(`Warning Failed to add udev rules: %s, incase of interface name mismatch,
					 network might not come up post migration, please check the network configuration post migration`, err)
						log.Println("Continuing with migration")
						err = nil
					}
				}
			}
		}
	}
	err = migobj.DetachAllVolumes(vminfo)
	if err != nil {
		return errors.Wrap(err, "Failed to detach all volumes from VM")
	}
	migobj.logMessage("Successfully converted disk")
	return nil
}

func (migobj *Migrate) CreateTargetInstance(vminfo vm.VMInfo) error {
	migobj.logMessage("Creating target instance")
	openstackops := migobj.Openstackclients
	networknames := migobj.Networknames
	var flavor *flavors.Flavor
	var err error

	if migobj.TargetFlavorId == "" {
		flavor, err = openstackops.GetClosestFlavour(vminfo.CPU, vminfo.Memory)
		if err != nil {
			return fmt.Errorf("failed to get closest OpenStack flavor: %s", err)
		}
		utils.PrintLog(fmt.Sprintf("Closest OpenStack flavor: %s: CPU: %dvCPUs\tMemory: %dMB\n", flavor.Name, flavor.VCPUs, flavor.RAM))
	} else {
		flavor, err = openstackops.GetFlavor(migobj.TargetFlavorId)
		if err != nil {
			return fmt.Errorf("failed to get OpenStack flavor: %s", err)
		}
	}

	networkids := []string{}
	ipaddresses := []string{}
	portids := []string{}

	if len(migobj.Networkports) != 0 {
		if len(migobj.Networkports) != len(networknames) {
			return fmt.Errorf("number of network ports does not match number of network names")
		}
		for _, port := range migobj.Networkports {
			retrPort, err := openstackops.GetPort(port)
			if err != nil {
				return fmt.Errorf("failed to get port: %s", err)
			}
			networkids = append(networkids, retrPort.NetworkID)
			portids = append(portids, retrPort.ID)
			ipaddresses = append(ipaddresses, retrPort.FixedIPs[0].IPAddress)
		}
	} else {
		for idx, networkname := range networknames {
			// Create Port Group with the same mac address as the source VM
			// Find the network with the given ID
			network, err := openstackops.GetNetwork(networkname)
			if err != nil {
				return fmt.Errorf("failed to get network: %s", err)
			}

			if network == nil {
				return fmt.Errorf("network not found")
			}

			ip := ""
			if len(vminfo.Mac) != len(vminfo.IPs) {
				ip = ""
			} else {
				ip = vminfo.IPs[idx]
			}

			if migobj.AssignedIP != "" {
				ip = migobj.AssignedIP
			}
			port, err := openstackops.CreatePort(network, vminfo.Mac[idx], ip, vminfo.Name)
			if err != nil {
				return fmt.Errorf("failed to create port group: %s", err)
			}

			utils.PrintLog(fmt.Sprintf("Port created successfully: MAC:%s IP:%s\n", port.MACAddress, port.FixedIPs[0].IPAddress))
			networkids = append(networkids, network.ID)
			portids = append(portids, port.ID)
			ipaddresses = append(ipaddresses, port.FixedIPs[0].IPAddress)
		}
	}

	// Create a new VM in OpenStack
	newVM, err := openstackops.CreateVM(flavor, networkids, portids, vminfo, migobj.TargetAvailabilityZone)
	if err != nil {
		return fmt.Errorf("failed to create VM: %s", err)
	}

	// Wait for VM to become active
	for i := 0; i < constants.MaxVMActiveCheckCount; i++ {
		utils.PrintLog(fmt.Sprintf("Waiting for VM to become active: %d/%d retries\n", i+1, constants.MaxVMActiveCheckCount))
		active, err := openstackops.WaitUntilVMActive(newVM.ID)
		if err != nil {
			return fmt.Errorf("failed to wait for VM to become active: %s", err)
		}
		if active {
			break
		}
		if i == constants.MaxVMActiveCheckCount-1 {
			return fmt.Errorf("VM is not active after %d retries", constants.MaxVMActiveCheckCount)
		}
		time.Sleep(constants.VMActiveCheckInterval)
	}

	migobj.logMessage(fmt.Sprintf("VM created successfully: ID: %s", newVM.ID))

	if migobj.PerformHealthChecks {
		err = migobj.HealthCheck(vminfo, ipaddresses)
		if err != nil {
			migobj.logMessage(fmt.Sprintf("Health Check failed: %s", err))
		}
	} else {
		migobj.logMessage("Skipping Health Checks")
	}

	return nil
}
func parseVersionID(osRelease string) string {
	for _, line := range strings.Split(osRelease, "\n") {
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToUpper(kv[0]))
		val := strings.Trim(kv[1], `"`) // Remove any quotes

		if key == "VERSION_ID" {
			return val
		}
	}
	return ""
}

func isNetplanSupported(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		log.Printf("Warning: unexpected VERSION_ID format: %q", version)
		return true // assume modern if uncertain
	}

	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		log.Printf("Warning: failed to parse VERSION_ID %q: %v %v", version, err1, err2)
		return true
	}

	// Compare with 17.10
	if major > 17 {
		return true
	}
	if major == 17 && minor >= 10 {
		return true
	}
	return false
}

func (migobj *Migrate) pingVM(ips []string) error {
	for _, ip := range ips {
		migobj.logMessage(fmt.Sprintf("Pinging VM: %s", ip))
		pinger, err := probing.NewPinger(ip)
		if err != nil {
			return fmt.Errorf("failed to create pinger: %s", err)
		}
		pinger.Count = 1
		pinger.Timeout = time.Second * 10
		err = pinger.Run()
		if err != nil {
			return fmt.Errorf("failed to run pinger: %s", err)
		}
		if pinger.Statistics().PacketLoss == 0 {
			migobj.logMessage("Ping succeeded")
		} else {
			return fmt.Errorf("Ping failed")
		}
	}
	return nil
}

func (migobj *Migrate) checkHTTPGet(ips []string, port string) error {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: time.Second * 10,
	}
	for _, ip := range ips {
		// Try HTTP first
		httpURL := fmt.Sprintf("http://%s:%s", ip, port)
		if err := migobj.tryConnection(client, httpURL); err == nil {
			migobj.logMessage("HTTP succeeded")
			continue // Success with HTTP, move to next IP
		}

		// If HTTP fails, try HTTPS
		httpsURL := fmt.Sprintf("https://%s:%s", ip, port)
		if err := migobj.tryConnection(client, httpsURL); err == nil {
			migobj.logMessage("HTTPS succeeded")
			continue // Success with HTTPS, move to next IP
		}

		// Both HTTP and HTTPS failed
		return fmt.Errorf("Both HTTP and HTTPS failed for %s:%s", ip, port)
	}

	return nil
}

func (migobj *Migrate) tryConnection(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		migobj.logMessage(fmt.Sprintf("GET failed for %s: %v", url, err))
		return err
	}
	defer resp.Body.Close()

	migobj.logMessage(fmt.Sprintf("GET response for %s: %d", url, resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET returned non-OK status for %s: %d", url, resp.StatusCode)
	}

	return nil
}

func (migobj *Migrate) HealthCheck(vminfo vm.VMInfo, ips []string) error {
	migobj.logMessage("Performing Health Checks")
	healthChecks := make(map[string]bool)
	healthChecks["Ping"] = false
	healthChecks["HTTP Get"] = false
	for i := 0; i < len(vminfo.IPs); i++ {
		if ips[i] != vminfo.IPs[i] {
			migobj.logMessage(fmt.Sprintf("VM has been assigned a new IP: %s instead of the original IP %s. Using the new IP for tests", ips[i], vminfo.IPs[i]))
		}
	}
	for i := 0; i < 10; i++ {
		migobj.logMessage(fmt.Sprintf("Health Check Attempt %d", i+1))
		// 1. Ping
		if !healthChecks["Ping"] {
			err := migobj.pingVM(ips)
			if err != nil {
				migobj.logMessage(fmt.Sprintf("Ping(s) failed: %s", err))
			} else {
				healthChecks["Ping"] = true
			}
		}
		// 2. HTTP GET check
		if !healthChecks["HTTP Get"] {
			err := migobj.checkHTTPGet(ips, migobj.HealthCheckPort)
			if err != nil {
				migobj.logMessage(fmt.Sprintf("HTTP Get failed: %s", err))
			} else {
				healthChecks["HTTP Get"] = true
			}
		}
		if healthChecks["Ping"] && healthChecks["HTTP Get"] {
			break
		}
		migobj.logMessage("Waiting for 60 seconds before retrying health checks")
		time.Sleep(60 * time.Second)
	}
	for key, value := range healthChecks {
		if !value {
			migobj.logMessage(fmt.Sprintf("Health Check %s failed", key))
		} else {
			migobj.logMessage(fmt.Sprintf("Health Check %s succeeded", key))
		}
	}
	return nil
}

func (migobj *Migrate) gracefulTerminate(vminfo vm.VMInfo, cancel context.CancelFunc) {
	gracefulShutdown := make(chan os.Signal, 1)
	// Handle SIGTERM
	signal.Notify(gracefulShutdown, syscall.SIGTERM, syscall.SIGINT)
	<-gracefulShutdown
	migobj.logMessage("Gracefully terminating")
	cancel()
	migobj.cleanup(vminfo, "Migration terminated")
	os.Exit(0)
}

func (migobj *Migrate) MigrateVM(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	// Wait until the data copy start time
	var zerotime time.Time
	if !migobj.MigrationTimes.DataCopyStart.Equal(zerotime) && migobj.MigrationTimes.DataCopyStart.After(time.Now()) {
		migobj.logMessage("Waiting for data copy start time")
		time.Sleep(time.Until(migobj.MigrationTimes.DataCopyStart))
		migobj.logMessage("Data copy start time reached")
	}
	// Get Info about VM
	vminfo, err := migobj.VMops.GetVMInfo(migobj.Ostype)
	if err != nil {
		cancel()
		return errors.Wrap(err, "failed to get all info")
	}
	if len(vminfo.VMDisks) != len(migobj.Volumetypes) {
		return fmt.Errorf("number of volume types does not match number of disks vm(%d) volume(%d)", len(vminfo.VMDisks), len(migobj.Volumetypes))
	}
	if len(vminfo.Mac) != len(migobj.Networknames) {
		return fmt.Errorf("number of mac addresses does not match number of network names mac(%d) network(%d)", len(vminfo.Mac), len(migobj.Networknames))
	}
	// Graceful Termination clean-up volumes and snapshots
	go migobj.gracefulTerminate(vminfo, cancel)

	// Create and Add Volumes to Host
	vminfo, err = migobj.CreateVolumes(vminfo)
	if err != nil {
		return errors.Wrap(err, "failed to add volumes to host")
	}
	// Enable CBT
	err = migobj.EnableCBTWrapper()
	if err != nil {
		migobj.cleanup(vminfo, fmt.Sprintf("CBT Failure: %s", err))
		return errors.Wrap(err, "CBT Failure")
	}

	// Create NBD servers
	for range vminfo.VMDisks {
		migobj.Nbdops = append(migobj.Nbdops, &nbd.NBDServer{})
	}

	// Live Replicate Disks
	vminfo, err = migobj.LiveReplicateDisks(ctx, vminfo)
	if err != nil {
		if cleanuperror := migobj.cleanup(vminfo, fmt.Sprintf("failed to live replicate disks: %s", err)); cleanuperror != nil {
			// combine both errors
			return errors.Wrapf(err, "failed to live replicate disks: %s", cleanuperror)
		}
		return errors.Wrap(err, "failed to live replicate disks")
	}
	// Import LUN and MigrateRDM disk
	for idx, rdmDisk := range vminfo.RDMDisks {
		volumeID, err := migobj.cinderManage(rdmDisk)
		if err != nil {
			migobj.cleanup(vminfo, fmt.Sprintf("failed to import LUN: %s", err))
			return errors.Wrap(err, "failed to import LUN")
		}
		vminfo.RDMDisks[idx].VolumeId = volumeID
	}
	// Convert the Boot Disk to raw format
	err = migobj.ConvertVolumes(ctx, vminfo)
	if err != nil {
		if cleanuperror := migobj.cleanup(vminfo, fmt.Sprintf("failed to convert volumes: %s", err)); cleanuperror != nil {
			// combine both errors
			return errors.Wrapf(err, "failed to convert disks: %s", cleanuperror)
		}
		return errors.Wrap(err, "failed to convert disks")
	}

	err = migobj.CreateTargetInstance(vminfo)
	if err != nil {
		if cleanuperror := migobj.cleanup(vminfo, fmt.Sprintf("failed to create target instance: %s", err)); cleanuperror != nil {
			// combine both errors
			return errors.Wrapf(err, "failed to create target instance: %s", cleanuperror)
		}
		return errors.Wrap(err, "failed to create target instance")
	}
	cancel()
	return nil
}

func (migobj *Migrate) cleanup(vminfo vm.VMInfo, message string) error {
	migobj.logMessage(fmt.Sprintf("%s. Trying to perform cleanup", message))
	err := migobj.DetachAllVolumes(vminfo)
	if err != nil {
		utils.PrintLog(fmt.Sprintf("Failed to detach all volumes from VM: %s\n", err))
	}
	err = migobj.DeleteAllVolumes(vminfo)
	if err != nil {
		utils.PrintLog(fmt.Sprintf("Failed to delete all volumes from host: %s\n", err))
	}
	err = migobj.VMops.CleanUpSnapshots(true)
	if err != nil {
		utils.PrintLog(fmt.Sprintf("Failed to delete snapshot of source VM: %s\n", err))
		return errors.Wrap(err, fmt.Sprintf("Failed to delete snapshot of source VM: %s\n", err))
	}
	return nil
}

// cinderManage imports a LUN into OpenStack Cinder and returns the volume ID.
func (migobj *Migrate) cinderManage(rdmDisk vm.RDMDisk) (string, error) {
	openstackops := migobj.Openstackclients
	migobj.logMessage(fmt.Sprintf("Importing LUN: %s", rdmDisk.DiskName))
	volume, err := openstackops.CinderManage(rdmDisk, "volume 3.8")
	if err != nil || volume == nil {
		return "", fmt.Errorf("failed to import LUN: %s", err)
	} else if volume.ID == "" {
		return "", fmt.Errorf("failed to import LUN: received empty volume ID")
	}
	migobj.logMessage(fmt.Sprintf("LUN imported successfully, waiting for volume %s to become available", volume.ID))
	// Wait for the volume to become available
	err = openstackops.WaitForVolume(volume.ID)
	if err != nil {
		return "", fmt.Errorf("failed to wait for volume to become available: %s", err)
	}
	migobj.logMessage(fmt.Sprintf("Volume %s is now available", volume.ID))
	return volume.ID, nil
}
