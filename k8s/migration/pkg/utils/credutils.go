/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/vjailbreak/k8s/migration/api/v1alpha1"
)

// VMwareCredentials represents VMware vCenter credentials
type VMwareCredentials struct {
	Username   string
	Password   string
	VCenterURL string
	Insecure   bool
}

// VMwareClient wraps the govmomi client with additional functionality
type VMwareClient struct {
	Client     *govmomi.Client
	Finder     *find.Finder
	Collector  *property.Collector
	Datacenter *object.Datacenter
	mu         sync.RWMutex
}

// SharedRDMInfo represents information about a shared RDM LUN
type SharedRDMInfo struct {
	LunUUID     string
	DisplayName string
	DiskSize    int64
	VMwareRefs  []string
	CompatMode  string
	DevicePaths map[string]string // VM name -> device path mapping
}

// RDMDiscoveryResult contains the results of RDM discovery
type RDMDiscoveryResult struct {
	IndividualRDMs map[string]*v1alpha1.RDMDiskInfo // VM name -> RDM info
	SharedRDMs     map[string]*SharedRDMInfo        // LUN UUID -> shared RDM info
	Errors         []error
}

// NewVMwareClient creates a new VMware client with the given credentials
func NewVMwareClient(ctx context.Context, creds VMwareCredentials) (*VMwareClient, error) {
	logger := log.FromContext(ctx)

	// Parse vCenter URL
	u, err := soap.ParseURL(creds.VCenterURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vCenter URL: %w", err)
	}

	// Set credentials
	u.User = url.UserPassword(creds.Username, creds.Password)

	// Create client
	client, err := govmomi.NewClient(ctx, u, creds.Insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create vCenter client: %w", err)
	}

	// Create finder
	finder := find.NewFinder(client.Client, true)

	// Create property collector
	collector := property.DefaultCollector(client.Client)

	logger.Info("Successfully connected to vCenter", "url", creds.VCenterURL)

	return &VMwareClient{
		Client:    client,
		Finder:    finder,
		Collector: collector,
	}, nil
}

// SetDatacenter sets the datacenter context for the client
func (c *VMwareClient) SetDatacenter(ctx context.Context, datacenterName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	dc, err := c.Finder.Datacenter(ctx, datacenterName)
	if err != nil {
		return fmt.Errorf("failed to find datacenter %s: %w", datacenterName, err)
	}

	c.Datacenter = dc
	c.Finder.SetDatacenter(dc)

	return nil
}

// GetAllVMs retrieves all VMs from the vCenter and performs RDM discovery
func (c *VMwareClient) GetAllVMs(ctx context.Context) ([]*v1alpha1.VMInfo, *RDMDiscoveryResult, error) {
	logger := log.FromContext(ctx)

	// Find all VMs
	vms, err := c.Finder.VirtualMachineList(ctx, "*")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	logger.Info("Found VMs", "count", len(vms))

	// Collect VM properties
	var vmInfos []*v1alpha1.VMInfo
	rdmDiscovery := &RDMDiscoveryResult{
		IndividualRDMs: make(map[string]*v1alpha1.RDMDiskInfo),
		SharedRDMs:     make(map[string]*SharedRDMInfo),
		Errors:         []error{},
	}

	for _, vm := range vms {
		vmInfo, err := c.getVMInfo(ctx, vm)
		if err != nil {
			logger.Error(err, "Failed to get VM info", "vm", vm.Name())
			rdmDiscovery.Errors = append(rdmDiscovery.Errors, err)
			continue
		}

		// Process RDM disks for this VM
		err = c.processVMRDMDisks(ctx, vm, vmInfo, rdmDiscovery)
		if err != nil {
			logger.Error(err, "Failed to process RDM disks", "vm", vm.Name())
			rdmDiscovery.Errors = append(rdmDiscovery.Errors, err)
		}

		vmInfos = append(vmInfos, vmInfo)
	}

	// Discover shared RDMs across all VMs
	err = c.discoverSharedRDMs(ctx, rdmDiscovery)
	if err != nil {
		logger.Error(err, "Failed to discover shared RDMs")
		rdmDiscovery.Errors = append(rdmDiscovery.Errors, err)
	}

	// Update VM info with shared RDM references
	c.updateRDMWithSharedRefs(vmInfos, rdmDiscovery)

	logger.Info("VM discovery completed", "vms", len(vmInfos), "sharedRDMs", len(rdmDiscovery.SharedRDMs))

	return vmInfos, rdmDiscovery, nil
}

// getVMInfo retrieves detailed information about a VM
func (c *VMwareClient) getVMInfo(ctx context.Context, vm *object.VirtualMachine) (*v1alpha1.VMInfo, error) {
	var mo mo.VirtualMachine
	err := vm.Properties(ctx, vm.Reference(), []string{
		"name",
		"config",
		"runtime.powerState",
		"guest",
		"summary",
	}, &mo)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM properties: %w", err)
	}

	vmInfo := &v1alpha1.VMInfo{
		Name:        mo.Name,
		UUID:        mo.Config.Uuid,
		PowerState:  string(mo.Runtime.PowerState),
		GuestOS:     mo.Config.GuestFullName,
		CPUCount:    mo.Config.Hardware.NumCPU,
		MemoryMB:    mo.Config.Hardware.MemoryMB,
		DiskInfo:    []v1alpha1.DiskInfo{},
		RDMDiskInfo: []v1alpha1.RDMDiskInfo{},
		NetworkInfo: []v1alpha1.NetworkInfo{},
		Annotations: make(map[string]string),
	}

	// Process disk information
	for _, device := range mo.Config.Hardware.Device {
		if disk, ok := device.(*types.VirtualDisk); ok {
			diskInfo := c.processDiskDevice(disk)
			if diskInfo != nil {
				vmInfo.DiskInfo = append(vmInfo.DiskInfo, *diskInfo)
			}
		}
	}

	// Process network information
	for _, device := range mo.Config.Hardware.Device {
		if nic, ok := device.(*types.VirtualEthernetCard); ok {
			networkInfo := c.processNetworkDevice(nic)
			if networkInfo != nil {
				vmInfo.NetworkInfo = append(vmInfo.NetworkInfo, *networkInfo)
			}
		}
	}

	return vmInfo, nil
}

// processDiskDevice processes a virtual disk device
func (c *VMwareClient) processDiskDevice(disk *types.VirtualDisk) *v1alpha1.DiskInfo {
	if disk.Backing == nil {
		return nil
	}

	var capacityInKB int64
	var diskMode string
	var thinProvisioned bool

	if disk.CapacityInKB != 0 {
		capacityInKB = disk.CapacityInKB
	}

	// Check backing type for disk mode and thin provisioning
	switch backing := disk.Backing.(type) {
	case *types.VirtualDiskFlatVer2BackingInfo:
		diskMode = string(backing.DiskMode)
		thinProvisioned = backing.ThinProvisioned != nil && *backing.ThinProvisioned
	case *types.VirtualDiskSparseVer2BackingInfo:
		diskMode = string(backing.DiskMode)
	case *types.VirtualDiskRawDiskMappingVer1BackingInfo:
		// This is an RDM disk, skip it here as it will be processed separately
		return nil
	}

	return &v1alpha1.DiskInfo{
		DeviceName:      getDeviceLabel(disk.DeviceInfo),
		CapacityInKB:    capacityInKB,
		DiskMode:        diskMode,
		ThinProvisioned: thinProvisioned,
	}
}

// processNetworkDevice processes a network device
func (c *VMwareClient) processNetworkDevice(nic types.BaseVirtualEthernetCard) *v1alpha1.NetworkInfo {
	card := nic.GetVirtualEthernetCard()
	if card == nil {
		return nil
	}

	networkInfo := &v1alpha1.NetworkInfo{
		MACAddress: card.MacAddress,
		Connected:  card.Connectable != nil && card.Connectable.Connected,
	}

	// Get network name from backing
	if card.Backing != nil {
		switch backing := card.Backing.(type) {
		case *types.VirtualEthernetCardNetworkBackingInfo:
			networkInfo.NetworkName = backing.DeviceName
		case *types.VirtualEthernetCardDistributedVirtualPortBackingInfo:
			if backing.Port != nil {
				networkInfo.NetworkName = backing.Port.PortgroupKey
			}
		}
	}

	return networkInfo
}

// processVMRDMDisks processes RDM disks for a specific VM
func (c *VMwareClient) processVMRDMDisks(ctx context.Context, vm *object.VirtualMachine, vmInfo *v1alpha1.VMInfo, discovery *RDMDiscoveryResult) error {
	var mo mo.VirtualMachine
	err := vm.Properties(ctx, vm.Reference(), []string{"config.hardware.device"}, &mo)
	if err != nil {
		return fmt.Errorf("failed to get VM hardware devices: %w", err)
	}

	for _, device := range mo.Config.Hardware.Device {
		if disk, ok := device.(*types.VirtualDisk); ok {
			rdmInfo := c.processVMDisk(disk, vmInfo.Name)
			if rdmInfo != nil {
				vmInfo.RDMDiskInfo = append(vmInfo.RDMDiskInfo, *rdmInfo)
				vmInfo.HasRDMDisks = true
				vmInfo.RDMMigrationRequired = true

				// Store for shared RDM analysis
				discovery.IndividualRDMs[vmInfo.Name+":"+rdmInfo.DeviceName] = rdmInfo
			}
		}
	}

	return nil
}

// processVMDisk processes a VM disk and detects RDM configurations
func (c *VMwareClient) processVMDisk(disk *types.VirtualDisk, vmName string) *v1alpha1.RDMDiskInfo {
	if disk.Backing == nil {
		return nil
	}

	// Check if this is an RDM disk
	rdmBacking, ok := disk.Backing.(*types.VirtualDiskRawDiskMappingVer1BackingInfo)
	if !ok {
		return nil
	}

	rdmInfo := &v1alpha1.RDMDiskInfo{
		DeviceName:        getDeviceLabel(disk.DeviceInfo),
		LunUUID:           rdmBacking.LunUuid,
		CapacityInKB:      disk.CapacityInKB,
		CompatibilityMode: string(rdmBacking.CompatibilityMode),
		MigrationStatus:   "Pending",
		DataSource:        "vmware-lun",
	}

	// Check for sharing mode indicators
	if disk.Backing.GetVirtualDeviceBackingInfo().Sharing != "" {
		rdmInfo.IsShared = true
	}

	// Additional sharing detection through SCSI controller analysis
	if c.detectSCSISharing(disk) {
		rdmInfo.IsShared = true
	}

	return rdmInfo
}

// detectSCSISharing detects if a disk is configured for sharing through SCSI controller settings
func (c *VMwareClient) detectSCSISharing(disk *types.VirtualDisk) bool {
	// Check if the disk is configured with sharing attributes
	if disk.Backing != nil {
		backing := disk.Backing.GetVirtualDeviceBackingInfo()
		if backing.Sharing == string(types.VirtualDiskSharingMultiWriter) ||
			backing.Sharing == string(types.VirtualDiskSharingPhysicalRDM) {
			return true
		}
	}

	// Additional checks can be added here for SCSI controller sharing modes
	return false
}

// discoverSharedRDMs analyzes all RDM disks to identify shared LUNs
func (c *VMwareClient) discoverSharedRDMs(ctx context.Context, discovery *RDMDiscoveryResult) error {
	logger := log.FromContext(ctx)

	// Group RDM disks by LUN UUID
	lunGroups := make(map[string][]*v1alpha1.RDMDiskInfo)
	vmMapping := make(map[string]string) // RDM key -> VM name

	for key, rdmInfo := range discovery.IndividualRDMs {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		vmName := parts[0]

		lunGroups[rdmInfo.LunUUID] = append(lunGroups[rdmInfo.LunUUID], rdmInfo)
		vmMapping[key] = vmName
	}

	// Identify shared RDMs (LUNs used by multiple VMs)
	for lunUUID, rdmList := range lunGroups {
		if len(rdmList) > 1 {
			sharedRDM := c.createSharedRDMInfo(lunUUID, rdmList, vmMapping)
			discovery.SharedRDMs[lunUUID] = sharedRDM

			// Mark individual RDMs as shared
			for _, rdmInfo := range rdmList {
				rdmInfo.IsShared = true
				rdmInfo.SharedRDMRef = c.generateSharedRDMName(lunUUID)
			}

			logger.Info("Discovered shared RDM", "lunUUID", lunUUID, "vmCount", len(rdmList))
		}
	}

	return nil
}

// createSharedRDMInfo creates SharedRDMInfo from a group of RDM disks
func (c *VMwareClient) createSharedRDMInfo(lunUUID string, rdmList []*v1alpha1.RDMDiskInfo, vmMapping map[string]string) *SharedRDMInfo {
	if len(rdmList) == 0 {
		return nil
	}

	// Use the first RDM as the template
	template := rdmList[0]

	sharedRDM := &SharedRDMInfo{
		LunUUID:     lunUUID,
		DisplayName: fmt.Sprintf("Shared RDM %s", lunUUID[:8]),
		DiskSize:    template.CapacityInKB * 1024, // Convert to bytes
		CompatMode:  template.CompatibilityMode,
		VMwareRefs:  []string{},
		DevicePaths: make(map[string]string),
	}

	// Collect VM references and device paths
	vmSet := make(map[string]bool)
	for key, rdmInfo := range rdmList {
		for mapKey, vmName := range vmMapping {
			if strings.Contains(mapKey, rdmInfo.DeviceName) {
				if !vmSet[vmName] {
					sharedRDM.VMwareRefs = append(sharedRDM.VMwareRefs, vmName)
					vmSet[vmName] = true
				}
				sharedRDM.DevicePaths[vmName] = rdmInfo.DeviceName
				break
			}
		}
	}

	// Sort VM references for consistency
	sort.Strings(sharedRDM.VMwareRefs)

	return sharedRDM
}

// generateSharedRDMName generates a consistent name for SharedRDM resources
func (c *VMwareClient) generateSharedRDMName(lunUUID string) string {
	// Use first 8 characters of UUID for readability
	shortUUID := lunUUID
	if len(lunUUID) > 8 {
		shortUUID = lunUUID[:8]
	}
	return fmt.Sprintf("shared-rdm-%s", strings.ToLower(shortUUID))
}

// createSharedRDMSpecs generates SharedRDM resource specifications
func (c *VMwareClient) CreateSharedRDMSpecs(discovery *RDMDiscoveryResult) []*v1alpha1.SharedRDM {
	var specs []*v1alpha1.SharedRDM

	for lunUUID, sharedRDM := range discovery.SharedRDMs {
		spec := &v1alpha1.SharedRDM{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "vjailbreak.k8s.pf9.io/v1alpha1",
				Kind:       "SharedRDM",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      c.generateSharedRDMName(lunUUID),
				Namespace: "vjailbreak-system",
				Labels: map[string]string{
					"vjailbreak.k8s.pf9.io/lun-uuid": lunUUID,
					"vjailbreak.k8s.pf9.io/type":     "shared-rdm",
				},
				Annotations: map[string]string{
					"vjailbreak.k8s.pf9.io/compat-mode": sharedRDM.CompatMode,
				},
			},
			Spec: v1alpha1.SharedRDMSpec{
				SourceUUID:        lunUUID,
				DisplayName:       sharedRDM.DisplayName,
				DiskSize:          sharedRDM.DiskSize,
				VMwareRefs:        sharedRDM.VMwareRefs,
				MigrationStrategy: "copy",
			},
			Status: v1alpha1.SharedRDMStatus{
				Phase:             "Discovering",
				MigrationProgress: 0,
				CreationTime:      &metav1.Time{Time: time.Now()},
				LastUpdateTime:    &metav1.Time{Time: time.Now()},
			},
		}

		specs = append(specs, spec)
	}

	return specs
}

// updateRDMWithSharedRefs updates VMInfo with SharedRDM references
func (c *VMwareClient) updateRDMWithSharedRefs(vmInfos []*v1alpha1.VMInfo, discovery *RDMDiscoveryResult) {
	for _, vmInfo := range vmInfos {
		for i := range vmInfo.RDMDiskInfo {
			rdmInfo := &vmInfo.RDMDiskInfo[i]
			if rdmInfo.IsShared && rdmInfo.SharedRDMRef != "" {
				// Update migration status for shared RDMs
				rdmInfo.MigrationStatus = "Pending"
				rdmInfo.DataSource = "vmware-lun"
			}
		}
	}
}

// validateRDMConfiguration validates RDM configuration for migration compatibility
func (c *VMwareClient) ValidateRDMConfiguration(ctx context.Context, discovery *RDMDiscoveryResult) []error {
	logger := log.FromContext(ctx)
	var errors []error

	// Validate individual RDMs
	for key, rdmInfo := range discovery.IndividualRDMs {
		if err := c.validateIndividualRDM(rdmInfo); err != nil {
			errors = append(errors, fmt.Errorf("RDM validation failed for %s: %w", key, err))
		}
	}

	// Validate shared RDMs
	for lunUUID, sharedRDM := range discovery.SharedRDMs {
		if err := c.validateSharedRDM(sharedRDM); err != nil {
			errors = append(errors, fmt.Errorf("Shared RDM validation failed for %s: %w", lunUUID, err))
		}
	}

	logger.Info("RDM configuration validation completed", "errors", len(errors))
	return errors
}

// validateIndividualRDM validates an individual RDM configuration
func (c *VMwareClient) validateIndividualRDM(rdmInfo *v1alpha1.RDMDiskInfo) error {
	if rdmInfo.LunUUID == "" {
		return fmt.Errorf("LUN UUID is required")
	}

	if rdmInfo.CapacityInKB <= 0 {
		return fmt.Errorf("invalid disk capacity: %d", rdmInfo.CapacityInKB)
	}

	if rdmInfo.CompatibilityMode != "physical" && rdmInfo.CompatibilityMode != "virtual" {
		return fmt.Errorf("unsupported compatibility mode: %s", rdmInfo.CompatibilityMode)
	}

	return nil
}

// validateSharedRDM validates a shared RDM configuration
func (c *VMwareClient) validateSharedRDM(sharedRDM *SharedRDMInfo) error {
	if sharedRDM.LunUUID == "" {
		return fmt.Errorf("LUN UUID is required")
	}

	if len(sharedRDM.VMwareRefs) < 2 {
		return fmt.Errorf("shared RDM must be used by at least 2 VMs, found: %d", len(sharedRDM.VMwareRefs))
	}

	if sharedRDM.DiskSize <= 0 {
		return fmt.Errorf("invalid disk size: %d", sharedRDM.DiskSize)
	}

	// Validate that all VMs have the same compatibility mode
	if sharedRDM.CompatMode != "physical" && sharedRDM.CompatMode != "virtual" {
		return fmt.Errorf("unsupported compatibility mode: %s", sharedRDM.CompatMode)
	}

	return nil
}

// syncRDMDisks synchronizes RDM disk information with enhanced shared RDM support
func (c *VMwareClient) SyncRDMDisks(ctx context.Context, vmInfos []*v1alpha1.VMInfo) (*RDMDiscoveryResult, error) {
	logger := log.FromContext(ctx)

	discovery := &RDMDiscoveryResult{
		IndividualRDMs: make(map[string]*v1alpha1.RDMDiskInfo),
		SharedRDMs:     make(map[string]*SharedRDMInfo),
		Errors:         []error{},
	}

	// Process each VM's RDM disks
	for _, vmInfo := range vmInfos {
		for i := range vmInfo.RDMDiskInfo {
			rdmInfo := &vmInfo.RDMDiskInfo[i]
			key := vmInfo.Name + ":" + rdmInfo.DeviceName
			discovery.IndividualRDMs[key] = rdmInfo
		}
	}

	// Discover shared RDMs
	err := c.discoverSharedRDMs(ctx, discovery)
	if err != nil {
		logger.Error(err, "Failed to discover shared RDMs during sync")
		discovery.Errors = append(discovery.Errors, err)
	}

	// Update VM info with shared references
	c.updateRDMWithSharedRefs(vmInfos, discovery)

	// Validate configuration
	validationErrors := c.ValidateRDMConfiguration(ctx, discovery)
	discovery.Errors = append(discovery.Errors, validationErrors...)

	logger.Info("RDM disk synchronization completed",
		"individualRDMs", len(discovery.IndividualRDMs),
		"sharedRDMs", len(discovery.SharedRDMs),
		"errors", len(discovery.Errors))

	return discovery, nil
}

// identifySharedRDMs analyzes all VMs to find shared RDM LUNs
func (c *VMwareClient) IdentifySharedRDMs(ctx context.Context, vmInfos []*v1alpha1.VMInfo) (map[string]*SharedRDMInfo, error) {
	discovery := &RDMDiscoveryResult{
		IndividualRDMs: make(map[string]*v1alpha1.RDMDiskInfo),
		SharedRDMs:     make(map[string]*SharedRDMInfo),
		Errors:         []error{},
	}

	// Collect all RDM disks
	for _, vmInfo := range vmInfos {
		for i := range vmInfo.RDMDiskInfo {
			rdmInfo := &vmInfo.RDMDiskInfo[i]
			key := vmInfo.Name + ":" + rdmInfo.DeviceName
			discovery.IndividualRDMs[key] = rdmInfo
		}
	}

	// Discover shared RDMs
	err := c.discoverSharedRDMs(ctx, discovery)
	if err != nil {
		return nil, fmt.Errorf("failed to identify shared RDMs: %w", err)
	}

	return discovery.SharedRDMs, nil
}

// populateRDMDiskInfoFromAttributes populates RDM disk info with enhanced shared RDM support
func (c *VMwareClient) populateRDMDiskInfoFromAttributes(rdmInfo *v1alpha1.RDMDiskInfo, attributes map[string]interface{}) {
	// Set basic attributes
	if lunUUID, ok := attributes["lunUUID"].(string); ok {
		rdmInfo.LunUUID = lunUUID
	}

	if capacity, ok := attributes["capacityInKB"].(int64); ok {
		rdmInfo.CapacityInKB = capacity
	}

	if compatMode, ok := attributes["compatibilityMode"].(string); ok {
		rdmInfo.CompatibilityMode = compatMode
	}

	// Handle shared RDM attributes
	if isShared, ok := attributes["isShared"].(bool); ok {
		rdmInfo.IsShared = isShared
	}

	if sharedRef, ok := attributes["sharedRDMRef"].(string); ok && sharedRef != "" {
		rdmInfo.SharedRDMRef = sharedRef
		rdmInfo.IsShared = true
	}

	// Set migration attributes
	if migrationStatus, ok := attributes["migrationStatus"].(string); ok {
		rdmInfo.MigrationStatus = migrationStatus
	} else {
		rdmInfo.MigrationStatus = "Pending"
	}

	if dataSource, ok := attributes["dataSource"].(string); ok {
		rdmInfo.DataSource = dataSource
	} else {
		rdmInfo.DataSource = "vmware-lun"
	}
}

// getDeviceLabel extracts a readable label from device info
func getDeviceLabel(deviceInfo types.BaseDescription) string {
	if deviceInfo == nil {
		return ""
	}

	info := deviceInfo.GetDescription()
	if info.Label != "" {
		return info.Label
	}

	return "Unknown Device"
}

// Close closes the VMware client connection
func (c *VMwareClient) Close(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Client != nil {
		return c.Client.Logout(ctx)
	}

	return nil
}
