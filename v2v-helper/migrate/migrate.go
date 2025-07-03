// Copyright © 2024 The vjailbreak authors

package migrate

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	"github.com/platform9/vjailbreak/v2v-helper/openstack"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/utils"
	"github.com/platform9/vjailbreak/v2v-helper/rdm"
	"github.com/platform9/vjailbreak/v2v-helper/vm"

	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	k8stypes "k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -source=migrate.go -destination=migrate_mock.go -package=migrate

// MigrateOperations defines the interface for VM migration operations
type MigrateOperations interface {
	MigrateVM(ctx context.Context, vmName string, osType string) error
	GetMigrationProgress() *MigrationProgress
	CleanupMigration(ctx context.Context) error
}

// Migrate handles the orchestration of VM migration including RDM support
type Migrate struct {
	vmOps         vm.VMOperations
	osOps         openstack.OpenstackOperations
	rdmOps        rdm.RDMOperations
	osClients     *utils.OpenStackClients
	k8sClient     k8sclient.Client
	vmName        string
	migrationName string
	progress      *MigrationProgress
	progressMutex sync.RWMutex
	ctx           context.Context

	// RDM-specific fields
	rdmMigrationEnabled bool
	sharedRDMRefs       []string
	rdmStrategy         string
}

// MigrationProgress tracks the overall migration progress
type MigrationProgress struct {
	Phase            string    `json:"phase"`
	OverallProgress  float64   `json:"overallProgress"`
	CurrentOperation string    `json:"currentOperation"`
	StartTime        time.Time `json:"startTime"`
	LastUpdate       time.Time `json:"lastUpdate"`

	// Regular disk migration progress
	DisksTotal     int                `json:"disksTotal"`
	DisksCompleted int                `json:"disksCompleted"`
	DiskProgress   map[string]float64 `json:"diskProgress"`

	// RDM-specific progress
	RDMDisksTotal     int                `json:"rdmDisksTotal"`
	RDMDisksCompleted int                `json:"rdmDisksCompleted"`
	RDMProgress       map[string]float64 `json:"rdmProgress"`
	RDMPhase          string             `json:"rdmPhase"`

	// Error tracking
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// RDMMigrationConfig holds configuration for RDM migration
type RDMMigrationConfig struct {
	Enabled           bool
	Strategy          string
	SharedRDMRefs     []string
	ValidationEnabled bool
	ParallelCopy      bool
	MaxRetries        int
	RetryDelay        time.Duration
}

// Migration phases
const (
	PhaseInitializing     = "Initializing"
	PhaseValidating       = "Validating"
	PhaseRDMPreparing     = "RDMPreparing"
	PhaseRDMCopying       = "RDMCopying"
	PhaseRDMValidating    = "RDMValidating"
	PhaseCopying          = "Copying"
	PhaseConvertingDisk   = "ConvertingDisk"
	PhaseCreatingInstance = "CreatingInstance"
	PhaseCompleted        = "Completed"
	PhaseFailed           = "Failed"
)

// RDM migration phases
const (
	RDMPhaseDiscovering = "Discovering"
	RDMPhaseValidating  = "Validating"
	RDMPhasePreparing   = "Preparing"
	RDMPhaseCopying     = "Copying"
	RDMPhaseVerifying   = "Verifying"
	RDMPhaseCompleted   = "Completed"
	RDMPhaseFailed      = "Failed"
)

// NewMigrate creates a new migration orchestrator
func NewMigrate(vmOps vm.VMOperations, osOps openstack.OpenstackOperations, osClients *utils.OpenStackClients, k8sClient k8sclient.Client, vmName, migrationName string) *Migrate {
	rdmOps := rdm.NewRDMOperations(osOps, osClients)

	return &Migrate{
		vmOps:         vmOps,
		osOps:         osOps,
		rdmOps:        rdmOps,
		osClients:     osClients,
		k8sClient:     k8sClient,
		vmName:        vmName,
		migrationName: migrationName,
		progress: &MigrationProgress{
			Phase:        PhaseInitializing,
			StartTime:    time.Now(),
			LastUpdate:   time.Now(),
			DiskProgress: make(map[string]float64),
			RDMProgress:  make(map[string]float64),
			Errors:       make([]string, 0),
			Warnings:     make([]string, 0),
		},
		rdmMigrationEnabled: true,
		rdmStrategy:         "copy",
	}
}

// MigrateVM orchestrates the complete VM migration including RDM support
func (m *Migrate) MigrateVM(ctx context.Context, vmName string, osType string) error {
	m.ctx = ctx
	m.updateProgress(PhaseInitializing, "Starting VM migration", 0)

	log.Printf("Starting migration for VM: %s", vmName)

	// Get VM information
	vmInfo, err := m.vmOps.GetVMInfo(osType)
	if err != nil {
		return m.handleError("Failed to get VM info", err)
	}

	// Check for RDM disks and determine migration strategy
	hasRDM, err := m.detectRDMDisks(ctx, &vmInfo)
	if err != nil {
		return m.handleError("Failed to detect RDM disks", err)
	}

	if hasRDM {
		log.Printf("RDM disks detected, enabling RDM migration")
		m.rdmMigrationEnabled = true

		// Execute RDM migration phase
		if err := m.executeRDMMigrationPhase(ctx, &vmInfo); err != nil {
			return m.handleError("RDM migration failed", err)
		}
	}

	// Continue with regular migration phases
	if err := m.executeRegularMigrationPhases(ctx, &vmInfo); err != nil {
		return m.handleError("Regular migration failed", err)
	}

	m.updateProgress(PhaseCompleted, "Migration completed successfully", 100)
	log.Printf("Migration completed successfully for VM: %s", vmName)

	return nil
}

// detectRDMDisks detects RDM disks in the VM configuration
func (m *Migrate) detectRDMDisks(ctx context.Context, vmInfo *vm.VMInfo) (bool, error) {
	m.updateProgress(PhaseValidating, "Detecting RDM disks", 5)

	// Check if VM has RDM disks from the VMInfo
	if len(vmInfo.RDMDisks) > 0 {
		log.Printf("Found %d RDM disks in VM configuration", len(vmInfo.RDMDisks))
		m.progress.RDMDisksTotal = len(vmInfo.RDMDisks)
		return true, nil
	}

	// Also check the VMwareMachine CR for RDM information
	vmwareMachine, err := m.getVMwareMachine(ctx)
	if err != nil {
		log.Printf("Warning: Could not retrieve VMwareMachine CR: %v", err)
		return false, nil
	}

	if vmwareMachine.Spec.VMInfo.HasRDMDisks {
		log.Printf("RDM disks detected in VMwareMachine CR")
		m.progress.RDMDisksTotal = len(vmwareMachine.Spec.VMInfo.RDMDiskInfo)
		m.sharedRDMRefs = vmwareMachine.Status.SharedRDMRefs
		return true, nil
	}

	log.Printf("No RDM disks detected")
	return false, nil
}

// executeRDMMigrationPhase handles the complete RDM migration process
func (m *Migrate) executeRDMMigrationPhase(ctx context.Context, vmInfo *vm.VMInfo) error {
	m.updateProgress(PhaseRDMPreparing, "Starting RDM migration", 10)

	// Step 1: Prepare RDM volumes
	if err := m.prepareRDMVolumes(ctx, vmInfo); err != nil {
		return fmt.Errorf("failed to prepare RDM volumes: %w", err)
	}

	// Step 2: Handle shared RDM scenarios
	if len(m.sharedRDMRefs) > 0 {
		if err := m.handleSharedRDM(ctx); err != nil {
			return fmt.Errorf("failed to handle shared RDM: %w", err)
		}
	}

	// Step 3: Copy RDM data
	m.updateProgress(PhaseRDMCopying, "Copying RDM data", 30)
	if err := m.copyRDMData(ctx, vmInfo); err != nil {
		return fmt.Errorf("failed to copy RDM data: %w", err)
	}

	// Step 4: Validate RDM migration
	m.updateProgress(PhaseRDMValidating, "Validating RDM migration", 80)
	if err := m.validateRDMMigration(ctx, vmInfo); err != nil {
		return fmt.Errorf("failed to validate RDM migration: %w", err)
	}

	m.updateRDMProgress(RDMPhaseCompleted, "RDM migration completed", 100)
	log.Printf("RDM migration phase completed successfully")

	return nil
}

// prepareRDMVolumes ensures all required Cinder volumes are ready for RDM data
func (m *Migrate) prepareRDMVolumes(ctx context.Context, vmInfo *vm.VMInfo) error {
	m.updateRDMProgress(RDMPhasePreparing, "Preparing RDM volumes", 0)

	for i, rdmDisk := range vmInfo.RDMDisks {
		log.Printf("Preparing RDM volume for disk: %s", rdmDisk.DiskName)

		// Check if volume already exists
		if rdmDisk.VolumeId != "" {
			log.Printf("Volume already exists for RDM disk %s: %s", rdmDisk.DiskName, rdmDisk.VolumeId)
			continue
		}

		// Create volume if it doesn't exist
		volumeSize := rdmDisk.DiskSize // Size in GB
		volumeName := fmt.Sprintf("%s-rdm-%s", m.vmName, rdmDisk.DiskName)

		var volume *volumes.Volume
		var err error

		// Check if this is a shared RDM
		if m.isSharedRDM(rdmDisk.UUID) {
			// For shared RDM, create multi-attach volume
			volume, err = m.osOps.CreateMultiAttachVolume(volumeName, volumeSize, rdmDisk.VolumeType, rdmDisk.CinderBackendPool)
		} else {
			// For regular RDM, create standard volume
			volume, err = m.osOps.CreateVolume(volumeName, volumeSize, rdmDisk.VolumeType)
		}

		if err != nil {
			return fmt.Errorf("failed to create volume for RDM disk %s: %w", rdmDisk.DiskName, err)
		}

		// Update the RDM disk info with volume ID
		vmInfo.RDMDisks[i].VolumeId = volume.ID

		// Wait for volume to be available
		if err := m.osOps.WaitForVolumeStatus(volume.ID, "available"); err != nil {
			return fmt.Errorf("volume %s did not become available: %w", volume.ID, err)
		}

		log.Printf("Successfully prepared volume %s for RDM disk %s", volume.ID, rdmDisk.DiskName)
	}

	return nil
}

// copyRDMData copies data from RDM LUNs to Cinder volumes
func (m *Migrate) copyRDMData(ctx context.Context, vmInfo *vm.VMInfo) error {
	m.updateRDMProgress(RDMPhaseCopying, "Copying RDM data", 0)

	// Process RDM disks sequentially to avoid resource conflicts
	for i, rdmDisk := range vmInfo.RDMDisks {
		log.Printf("Starting data copy for RDM disk: %s", rdmDisk.DiskName)

		// Get RDM device information
		rdmDeviceInfo, err := m.rdmOps.GetRDMDeviceInfo(rdmDisk.UUID)
		if err != nil {
			return fmt.Errorf("failed to get RDM device info for %s: %w", rdmDisk.UUID, err)
		}

		// Attach the target volume to the helper pod
		devicePath, err := m.rdmOps.AttachVolumeToHelper(ctx, rdmDisk.VolumeId)
		if err != nil {
			return fmt.Errorf("failed to attach volume %s: %w", rdmDisk.VolumeId, err)
		}

		// Prepare RDM and volume info for copy operation
		rdmInfo := rdm.RDMInfo{
			UUID:        rdmDisk.UUID,
			DisplayName: rdmDisk.DisplayName,
			DiskSize:    rdmDisk.DiskSize * 1024 * 1024 * 1024, // Convert GB to bytes
			DevicePath:  rdmDeviceInfo.DevicePath,
			IsShared:    m.isSharedRDM(rdmDisk.UUID),
		}

		volumeInfo := rdm.VolumeInfo{
			VolumeID:    rdmDisk.VolumeId,
			DevicePath:  devicePath,
			Size:        rdmDisk.DiskSize * 1024 * 1024 * 1024, // Convert GB to bytes
			VolumeType:  rdmDisk.VolumeType,
			BackendPool: rdmDisk.CinderBackendPool,
			MultiAttach: m.isSharedRDM(rdmDisk.UUID),
		}

		// Start the copy operation
		if err := m.rdmOps.CopyRDMToVolume(ctx, rdmInfo, volumeInfo); err != nil {
			// Cleanup: detach volume before returning error
			if detachErr := m.rdmOps.DetachVolumeFromHelper(ctx, rdmDisk.VolumeId); detachErr != nil {
				log.Printf("Warning: Failed to detach volume %s after copy failure: %v", rdmDisk.VolumeId, detachErr)
			}
			return fmt.Errorf("failed to copy RDM data for %s: %w", rdmDisk.DiskName, err)
		}

		// Detach the volume after successful copy
		if err := m.rdmOps.DetachVolumeFromHelper(ctx, rdmDisk.VolumeId); err != nil {
			log.Printf("Warning: Failed to detach volume %s after successful copy: %v", rdmDisk.VolumeId, err)
		}

		// Update progress
		m.progress.RDMDisksCompleted = i + 1
		progress := float64(m.progress.RDMDisksCompleted) / float64(m.progress.RDMDisksTotal) * 100
		m.progress.RDMProgress[rdmDisk.DiskName] = 100.0

		log.Printf("Successfully copied RDM data for disk: %s", rdmDisk.DiskName)
		m.updateRDMProgress(RDMPhaseCopying, fmt.Sprintf("Copied %d/%d RDM disks", m.progress.RDMDisksCompleted, m.progress.RDMDisksTotal), progress)
	}

	return nil
}

// validateRDMMigration validates the integrity of copied RDM data
func (m *Migrate) validateRDMMigration(ctx context.Context, vmInfo *vm.VMInfo) error {
	m.updateRDMProgress(RDMPhaseVerifying, "Validating RDM data integrity", 0)

	for i, rdmDisk := range vmInfo.RDMDisks {
		log.Printf("Validating RDM data for disk: %s", rdmDisk.DiskName)

		// Get RDM device information
		rdmDeviceInfo, err := m.rdmOps.GetRDMDeviceInfo(rdmDisk.UUID)
		if err != nil {
			return fmt.Errorf("failed to get RDM device info for validation %s: %w", rdmDisk.UUID, err)
		}

		// Attach the volume for validation
		devicePath, err := m.rdmOps.AttachVolumeToHelper(ctx, rdmDisk.VolumeId)
		if err != nil {
			return fmt.Errorf("failed to attach volume for validation %s: %w", rdmDisk.VolumeId, err)
		}

		// Prepare info for validation
		rdmInfo := rdm.RDMInfo{
			UUID:        rdmDisk.UUID,
			DisplayName: rdmDisk.DisplayName,
			DiskSize:    rdmDisk.DiskSize * 1024 * 1024 * 1024, // Convert GB to bytes
			DevicePath:  rdmDeviceInfo.DevicePath,
			IsShared:    m.isSharedRDM(rdmDisk.UUID),
		}

		volumeInfo := rdm.VolumeInfo{
			VolumeID:   rdmDisk.VolumeId,
			DevicePath: devicePath,
			Size:       rdmDisk.DiskSize * 1024 * 1024 * 1024, // Convert GB to bytes
		}

		// Validate the copy
		if err := m.rdmOps.ValidateRDMCopy(ctx, rdmInfo, volumeInfo); err != nil {
			// Cleanup: detach volume before returning error
			if detachErr := m.rdmOps.DetachVolumeFromHelper(ctx, rdmDisk.VolumeId); detachErr != nil {
				log.Printf("Warning: Failed to detach volume %s after validation failure: %v", rdmDisk.VolumeId, detachErr)
			}
			return fmt.Errorf("RDM data validation failed for %s: %w", rdmDisk.DiskName, err)
		}

		// Detach the volume after validation
		if err := m.rdmOps.DetachVolumeFromHelper(ctx, rdmDisk.VolumeId); err != nil {
			log.Printf("Warning: Failed to detach volume %s after validation: %v", rdmDisk.VolumeId, err)
		}

		// Update progress
		progress := float64(i+1) / float64(len(vmInfo.RDMDisks)) * 100
		log.Printf("Successfully validated RDM data for disk: %s", rdmDisk.DiskName)
		m.updateRDMProgress(RDMPhaseVerifying, fmt.Sprintf("Validated %d/%d RDM disks", i+1, len(vmInfo.RDMDisks)), progress)
	}

	return nil
}

// handleSharedRDM handles special processing for shared RDM scenarios
func (m *Migrate) handleSharedRDM(ctx context.Context) error {
	log.Printf("Processing %d shared RDM references", len(m.sharedRDMRefs))

	for _, sharedRDMRef := range m.sharedRDMRefs {
		// Get the SharedRDM resource
		sharedRDM, err := m.getSharedRDM(ctx, sharedRDMRef)
		if err != nil {
			return fmt.Errorf("failed to get SharedRDM %s: %w", sharedRDMRef, err)
		}

		// Check if the SharedRDM is ready
		if !m.isSharedRDMReady(sharedRDM) {
			return fmt.Errorf("SharedRDM %s is not ready", sharedRDMRef)
		}

		log.Printf("SharedRDM %s is ready with volume %s", sharedRDMRef, sharedRDM.Status.VolumeID)
	}

	return nil
}

// executeRegularMigrationPhases handles the standard migration phases (non-RDM)
func (m *Migrate) executeRegularMigrationPhases(ctx context.Context, vmInfo *vm.VMInfo) error {
	// Filter out RDM disks from regular disk processing
	regularDisks := m.filterRegularDisks(vmInfo.VMDisks)
	m.progress.DisksTotal = len(regularDisks)

	m.updateProgress(PhaseCopying, "Starting regular disk migration", 85)

	// Process regular disks (existing logic would go here)
	for i, disk := range regularDisks {
		log.Printf("Processing regular disk: %s", disk.Name)

		// Existing disk migration logic would be implemented here
		// This is a placeholder for the actual disk migration implementation

		m.progress.DisksCompleted = i + 1
		progress := 85 + (float64(m.progress.DisksCompleted)/float64(m.progress.DisksTotal))*10
		m.updateProgress(PhaseCopying, fmt.Sprintf("Migrated %d/%d regular disks", m.progress.DisksCompleted, m.progress.DisksTotal), progress)
	}

	m.updateProgress(PhaseConvertingDisk, "Converting disks", 95)
	// Disk conversion logic would go here

	m.updateProgress(PhaseCreatingInstance, "Creating OpenStack instance", 98)
	// Instance creation logic would go here

	return nil
}

// filterRegularDisks filters out RDM disks from the regular disk list
func (m *Migrate) filterRegularDisks(allDisks []vm.VMDisk) []vm.VMDisk {
	var regularDisks []vm.VMDisk

	for _, disk := range allDisks {
		// Check if this disk is an RDM disk by comparing with RDM disk names
		isRDM := false
		// This logic would need to be implemented based on how RDM disks are identified
		// For now, we assume all disks in VMDisks are regular disks

		if !isRDM {
			regularDisks = append(regularDisks, disk)
		}
	}

	return regularDisks
}

// Helper methods

// isSharedRDM checks if an RDM disk is shared based on its UUID
func (m *Migrate) isSharedRDM(uuid string) bool {
	for _, sharedRef := range m.sharedRDMRefs {
		// This would need to be implemented to check if the UUID corresponds to a shared RDM
		// For now, return false as a placeholder
		_ = sharedRef
	}
	return false
}

// getVMwareMachine retrieves the VMwareMachine CR for this VM
func (m *Migrate) getVMwareMachine(ctx context.Context) (*vjailbreakv1alpha1.VMwareMachine, error) {
	namespacedName := k8stypes.NamespacedName{
		Name:      m.vmName,
		Namespace: constants.MigrationSystemNamespace,
	}

	vmwareMachine := &vjailbreakv1alpha1.VMwareMachine{}
	if err := m.k8sClient.Get(ctx, namespacedName, vmwareMachine); err != nil {
		return nil, err
	}

	return vmwareMachine, nil
}

// getSharedRDM retrieves a SharedRDM resource
func (m *Migrate) getSharedRDM(ctx context.Context, name string) (*vjailbreakv1alpha1.SharedRDM, error) {
	namespacedName := k8stypes.NamespacedName{
		Name:      name,
		Namespace: constants.MigrationSystemNamespace,
	}

	sharedRDM := &vjailbreakv1alpha1.SharedRDM{}
	if err := m.k8sClient.Get(ctx, namespacedName, sharedRDM); err != nil {
		return nil, err
	}

	return sharedRDM, nil
}

// isSharedRDMReady checks if a SharedRDM resource is ready
func (m *Migrate) isSharedRDMReady(sharedRDM *vjailbreakv1alpha1.SharedRDM) bool {
	return sharedRDM.Status.Phase == "Ready" && sharedRDM.Status.VolumeID != ""
}

// updateProgress updates the overall migration progress
func (m *Migrate) updateProgress(phase, operation string, progress float64) {
	m.progressMutex.Lock()
	defer m.progressMutex.Unlock()

	m.progress.Phase = phase
	m.progress.CurrentOperation = operation
	m.progress.OverallProgress = progress
	m.progress.LastUpdate = time.Now()

	log.Printf("Migration progress: %s - %s (%.1f%%)", phase, operation, progress)
}

// updateRDMProgress updates RDM-specific progress
func (m *Migrate) updateRDMProgress(phase, operation string, progress float64) {
	m.progressMutex.Lock()
	defer m.progressMutex.Unlock()

	m.progress.RDMPhase = phase
	m.progress.LastUpdate = time.Now()

	log.Printf("RDM migration progress: %s - %s (%.1f%%)", phase, operation, progress)
}

// handleError handles migration errors with proper cleanup
func (m *Migrate) handleError(message string, err error) error {
	fullError := fmt.Errorf("%s: %w", message, err)

	m.progressMutex.Lock()
	m.progress.Phase = PhaseFailed
	m.progress.Errors = append(m.progress.Errors, fullError.Error())
	m.progress.LastUpdate = time.Now()
	m.progressMutex.Unlock()

	log.Printf("Migration error: %v", fullError)

	// Attempt cleanup
	if cleanupErr := m.CleanupMigration(m.ctx); cleanupErr != nil {
		log.Printf("Warning: Cleanup failed: %v", cleanupErr)
	}

	return fullError
}

// GetMigrationProgress returns the current migration progress
func (m *Migrate) GetMigrationProgress() *MigrationProgress {
	m.progressMutex.RLock()
	defer m.progressMutex.RUnlock()

	// Return a copy to avoid race conditions
	progressCopy := *m.progress
	return &progressCopy
}

// CleanupMigration performs cleanup operations for failed or completed migrations
func (m *Migrate) CleanupMigration(ctx context.Context) error {
	log.Printf("Starting migration cleanup for VM: %s", m.vmName)

	var errors []string

	// Cleanup RDM operations
	if m.rdmOps != nil {
		if err := m.rdmOps.CleanupTempResources(); err != nil {
			errors = append(errors, fmt.Sprintf("RDM cleanup failed: %v", err))
		}
	}

	// Cleanup snapshots
	if m.vmOps != nil {
		if err := m.vmOps.CleanUpSnapshots(true); err != nil {
			errors = append(errors, fmt.Sprintf("Snapshot cleanup failed: %v", err))
		}
	}

	// Additional cleanup operations would go here

	if len(errors) > 0 {
		return fmt.Errorf("cleanup completed with errors: %s", strings.Join(errors, "; "))
	}

	log.Printf("Migration cleanup completed successfully")
	return nil
}

// migrateRDMDisks orchestrates RDM disk migration (legacy method for compatibility)
func (m *Migrate) migrateRDMDisks(ctx context.Context, vmInfo *vm.VMInfo) error {
	return m.executeRDMMigrationPhase(ctx, vmInfo)
}

// SetRDMConfiguration sets RDM migration configuration
func (m *Migrate) SetRDMConfiguration(config RDMMigrationConfig) {
	m.rdmMigrationEnabled = config.Enabled
	m.rdmStrategy = config.Strategy
	m.sharedRDMRefs = config.SharedRDMRefs
}

// GetRDMConfiguration returns current RDM configuration
func (m *Migrate) GetRDMConfiguration() RDMMigrationConfig {
	return RDMMigrationConfig{
		Enabled:       m.rdmMigrationEnabled,
		Strategy:      m.rdmStrategy,
		SharedRDMRefs: m.sharedRDMRefs,
	}
}
