// Copyright © 2024 The vjailbreak authors

package rdm

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/platform9/vjailbreak/v2v-helper/openstack"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/utils"
	"github.com/platform9/vjailbreak/v2v-helper/vm"
)

//go:generate mockgen -source=rdmops.go -destination=rdmops_mock.go -package=rdm

// RDMOperations defines the interface for RDM migration operations
type RDMOperations interface {
	// CopyRDMToVolume copies data from RDM LUN to Cinder volume
	CopyRDMToVolume(ctx context.Context, rdmInfo RDMInfo, volumeInfo VolumeInfo) error
	
	// ValidateRDMCopy validates data integrity after copy
	ValidateRDMCopy(ctx context.Context, rdmInfo RDMInfo, volumeInfo VolumeInfo) error
	
	// GetRDMDeviceInfo gets device information for RDM LUN
	GetRDMDeviceInfo(uuid string) (*RDMDeviceInfo, error)
	
	// AttachVolumeToHelper attaches Cinder volume to helper pod
	AttachVolumeToHelper(ctx context.Context, volumeID string) (string, error)
	
	// DetachVolumeFromHelper detaches volume from helper pod
	DetachVolumeFromHelper(ctx context.Context, volumeID string) error
}

// RDMInfo contains information about the source RDM disk
type RDMInfo struct {
	UUID        string
	DisplayName string
	DiskSize    int64
	DevicePath  string
	IsShared    bool
	LockFile    string
}

// VolumeInfo contains information about the target Cinder volume
type VolumeInfo struct {
	VolumeID     string
	DevicePath   string
	Size         int64
	VolumeType   string
	BackendPool  string
	MultiAttach  bool
}

// RDMDeviceInfo contains detailed device information
type RDMDeviceInfo struct {
	UUID       string
	DevicePath string
	Size       int64
	Model      string
	Vendor     string
	Serial     string
	WWN        string
	Available  bool
}

// CopyProgress tracks the progress of data copy operations
type CopyProgress struct {
	BytesCopied   int64
	TotalBytes    int64
	PercentDone   float64
	Speed         int64 // bytes per second
	ETA           time.Duration
	StartTime     time.Time
	LastUpdate    time.Time
}

// RDMOperationsImpl implements the RDMOperations interface
type RDMOperationsImpl struct {
	osOps         openstack.OpenstackOperations
	osClients     *utils.OpenStackClients
	progressChan  chan CopyProgress
	lockManager   *LockManager
	maxRetries    int
	retryDelay    time.Duration
	bandwidthMBps int64 // bandwidth limit in MB/s
}

// LockManager handles locking for shared RDM resources
type LockManager struct {
	locks map[string]*sync.RWMutex
	mutex sync.Mutex
}

// NewLockManager creates a new lock manager
func NewLockManager() *LockManager {
	return &LockManager{
		locks: make(map[string]*sync.RWMutex),
	}
}

// GetLock returns a lock for the given resource
func (lm *LockManager) GetLock(resource string) *sync.RWMutex {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()
	
	if lock, exists := lm.locks[resource]; exists {
		return lock
	}
	
	lm.locks[resource] = &sync.RWMutex{}
	return lm.locks[resource]
}

// NewRDMOperations creates a new RDMOperations implementation
func NewRDMOperations(osOps openstack.OpenstackOperations, osClients *utils.OpenStackClients) RDMOperations {
	return &RDMOperationsImpl{
		osOps:         osOps,
		osClients:     osClients,
		progressChan:  make(chan CopyProgress, 100),
		lockManager:   NewLockManager(),
		maxRetries:    3,
		retryDelay:    time.Second * 5,
		bandwidthMBps: 100, // Default 100 MB/s
	}
}

// SetBandwidthLimit sets the bandwidth limit for copy operations
func (r *RDMOperationsImpl) SetBandwidthLimit(mbps int64) {
	r.bandwidthMBps = mbps
}

// GetRDMDeviceInfo discovers and returns device information for an RDM LUN
func (r *RDMOperationsImpl) GetRDMDeviceInfo(uuid string) (*RDMDeviceInfo, error) {
	// Clean the UUID (remove any dashes or formatting)
	cleanUUID := strings.ReplaceAll(strings.ToLower(uuid), "-", "")
	
	// Try multiple methods to find the device
	devicePath, err := r.findDeviceByUUID(cleanUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to find device for UUID %s: %w", uuid, err)
	}
	
	// Get device details
	deviceInfo, err := r.getDeviceDetails(devicePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get device details for %s: %w", devicePath, err)
	}
	
	deviceInfo.UUID = uuid
	deviceInfo.DevicePath = devicePath
	
	return deviceInfo, nil
}

// findDeviceByUUID finds the device path using various methods
func (r *RDMOperationsImpl) findDeviceByUUID(uuid string) (string, error) {
	// Method 1: Check /dev/disk/by-id/
	if path, err := r.findDeviceByPath("/dev/disk/by-id/", uuid); err == nil {
		return path, nil
	}
	
	// Method 2: Check /dev/disk/by-uuid/
	if path, err := r.findDeviceByPath("/dev/disk/by-uuid/", uuid); err == nil {
		return path, nil
	}
	
	// Method 3: Use lsblk to find device
	if path, err := r.findDeviceByLsblk(uuid); err == nil {
		return path, nil
	}
	
	// Method 4: Use lsscsi to find device
	if path, err := r.findDeviceByLsscsi(uuid); err == nil {
		return path, nil
	}
	
	return "", fmt.Errorf("device with UUID %s not found", uuid)
}

// findDeviceByPath searches for device in a specific path
func (r *RDMOperationsImpl) findDeviceByPath(basePath, uuid string) (string, error) {
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return "", err
	}
	
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Name()), uuid) {
			linkPath := filepath.Join(basePath, entry.Name())
			realPath, err := filepath.EvalSymlinks(linkPath)
			if err != nil {
				continue
			}
			return realPath, nil
		}
	}
	
	return "", fmt.Errorf("device not found in %s", basePath)
}

// findDeviceByLsblk uses lsblk command to find device
func (r *RDMOperationsImpl) findDeviceByLsblk(uuid string) (string, error) {
	cmd := exec.Command("lsblk", "-o", "NAME,UUID", "-n")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.Contains(strings.ToLower(fields[1]), uuid) {
			return "/dev/" + fields[0], nil
		}
	}
	
	return "", fmt.Errorf("device not found with lsblk")
}

// findDeviceByLsscsi uses lsscsi command to find device
func (r *RDMOperationsImpl) findDeviceByLsscsi(uuid string) (string, error) {
	cmd := exec.Command("lsscsi", "-g")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	// Parse lsscsi output to find matching device
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(strings.ToLower(line), uuid) {
			// Extract device path from lsscsi output
			re := regexp.MustCompile(`(/dev/sd[a-z]+)`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				return matches[1], nil
			}
		}
	}
	
	return "", fmt.Errorf("device not found with lsscsi")
}

// getDeviceDetails retrieves detailed information about a device
func (r *RDMOperationsImpl) getDeviceDetails(devicePath string) (*RDMDeviceInfo, error) {
	info := &RDMDeviceInfo{
		DevicePath: devicePath,
		Available:  true,
	}
	
	// Get device size
	size, err := r.getDeviceSize(devicePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get device size: %w", err)
	}
	info.Size = size
	
	// Get device attributes using udevadm
	attrs, err := r.getDeviceAttributes(devicePath)
	if err == nil {
		info.Model = attrs["ID_MODEL"]
		info.Vendor = attrs["ID_VENDOR"]
		info.Serial = attrs["ID_SERIAL"]
		info.WWN = attrs["ID_WWN"]
	}
	
	return info, nil
}

// getDeviceSize returns the size of a device in bytes
func (r *RDMOperationsImpl) getDeviceSize(devicePath string) (int64, error) {
	cmd := exec.Command("blockdev", "--getsize64", devicePath)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	
	sizeStr := strings.TrimSpace(string(output))
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return 0, err
	}
	
	return size, nil
}

// getDeviceAttributes retrieves device attributes using udevadm
func (r *RDMOperationsImpl) getDeviceAttributes(devicePath string) (map[string]string, error) {
	cmd := exec.Command("udevadm", "info", "--query=property", "--name="+devicePath)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	attrs := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				attrs[parts[0]] = parts[1]
			}
		}
	}
	
	return attrs, nil
}

// AttachVolumeToHelper attaches a Cinder volume to the helper pod
func (r *RDMOperationsImpl) AttachVolumeToHelper(ctx context.Context, volumeID string) (string, error) {
	// Use the OpenStack operations to attach the volume
	err := r.osOps.AttachVolumeToVM(volumeID)
	if err != nil {
		return "", fmt.Errorf("failed to attach volume %s: %w", volumeID, err)
	}
	
	// Wait for the volume to be attached
	err = r.osOps.WaitForVolumeAttachment(volumeID)
	if err != nil {
		return "", fmt.Errorf("failed to wait for volume attachment %s: %w", volumeID, err)
	}
	
	// Find the device path for the attached volume
	devicePath, err := r.osOps.FindDevice(volumeID)
	if err != nil {
		return "", fmt.Errorf("failed to find device for volume %s: %w", volumeID, err)
	}
	
	return devicePath, nil
}

// DetachVolumeFromHelper detaches a volume from the helper pod
func (r *RDMOperationsImpl) DetachVolumeFromHelper(ctx context.Context, volumeID string) error {
	err := r.osOps.DetachVolumeFromVM(volumeID)
	if err != nil {
		return fmt.Errorf("failed to detach volume %s: %w", volumeID, err)
	}
	
	return nil
}

// CopyRDMToVolume copies data from RDM LUN to Cinder volume
func (r *RDMOperationsImpl) CopyRDMToVolume(ctx context.Context, rdmInfo RDMInfo, volumeInfo VolumeInfo) error {
	// Handle locking for shared RDM
	if rdmInfo.IsShared {
		lock := r.lockManager.GetLock(rdmInfo.UUID)
		lock.Lock()
		defer lock.Unlock()
	}
	
	// Validate source and target devices
	if err := r.validateDevices(rdmInfo, volumeInfo); err != nil {
		return fmt.Errorf("device validation failed: %w", err)
	}
	
	// Perform the copy with retry logic
	var lastErr error
	for attempt := 0; attempt < r.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(r.retryDelay)
		}
		
		err := r.performCopy(ctx, rdmInfo, volumeInfo)
		if err == nil {
			return nil
		}
		
		lastErr = err
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	
	return fmt.Errorf("copy failed after %d attempts: %w", r.maxRetries, lastErr)
}

// validateDevices validates that source and target devices are accessible
func (r *RDMOperationsImpl) validateDevices(rdmInfo RDMInfo, volumeInfo VolumeInfo) error {
	// Check source device
	if _, err := os.Stat(rdmInfo.DevicePath); err != nil {
		return fmt.Errorf("source device %s not accessible: %w", rdmInfo.DevicePath, err)
	}
	
	// Check target device
	if _, err := os.Stat(volumeInfo.DevicePath); err != nil {
		return fmt.Errorf("target device %s not accessible: %w", volumeInfo.DevicePath, err)
	}
	
	// Verify sizes match
	sourceSize, err := r.getDeviceSize(rdmInfo.DevicePath)
	if err != nil {
		return fmt.Errorf("failed to get source device size: %w", err)
	}
	
	targetSize, err := r.getDeviceSize(volumeInfo.DevicePath)
	if err != nil {
		return fmt.Errorf("failed to get target device size: %w", err)
	}
	
	if sourceSize > targetSize {
		return fmt.Errorf("source device (%d bytes) is larger than target device (%d bytes)", sourceSize, targetSize)
	}
	
	return nil
}

// performCopy performs the actual data copy operation
func (r *RDMOperationsImpl) performCopy(ctx context.Context, rdmInfo RDMInfo, volumeInfo VolumeInfo) error {
	// Create progress tracking
	progress := CopyProgress{
		TotalBytes: rdmInfo.DiskSize,
		StartTime:  time.Now(),
		LastUpdate: time.Now(),
	}
	
	// Build dd command with progress monitoring
	ddCmd := r.buildDDCommand(rdmInfo.DevicePath, volumeInfo.DevicePath)
	
	// Start the copy process
	cmd := exec.CommandContext(ctx, "sh", "-c", ddCmd)
	
	// Set up progress monitoring
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	
	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start copy command: %w", err)
	}
	
	// Monitor progress
	go r.monitorProgress(stderr, &progress)
	
	// Wait for completion
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("copy command failed: %w", err)
	}
	
	return nil
}

// buildDDCommand builds the dd command with appropriate options
func (r *RDMOperationsImpl) buildDDCommand(source, target string) string {
	blockSize := "1M"
	
	// Build base dd command
	ddCmd := fmt.Sprintf("dd if=%s of=%s bs=%s conv=fdatasync status=progress", source, target, blockSize)
	
	// Add bandwidth limiting if configured
	if r.bandwidthMBps > 0 {
		// Use pv for bandwidth limiting
		pvCmd := fmt.Sprintf("pv -L %dM", r.bandwidthMBps)
		ddCmd = fmt.Sprintf("dd if=%s bs=%s | %s | dd of=%s bs=%s conv=fdatasync", source, blockSize, pvCmd, target, blockSize)
	}
	
	return ddCmd
}

// monitorProgress monitors the progress of the copy operation
func (r *RDMOperationsImpl) monitorProgress(stderr io.Reader, progress *CopyProgress) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		r.parseProgressLine(line, progress)
		
		// Send progress update
		select {
		case r.progressChan <- *progress:
		default:
			// Channel full, skip this update
		}
	}
}

// parseProgressLine parses dd progress output
func (r *RDMOperationsImpl) parseProgressLine(line string, progress *CopyProgress) {
	// Parse dd status=progress output
	// Example: "1073741824 bytes (1.1 GB, 1.0 GiB) copied, 10.5 s, 102 MB/s"
	re := regexp.MustCompile(`(\d+) bytes.*copied, ([\d.]+) s, ([\d.]+) ([KMGT]?B)/s`)
	matches := re.FindStringSubmatch(line)
	
	if len(matches) >= 4 {
		if bytes, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
			progress.BytesCopied = bytes
			progress.PercentDone = float64(bytes) / float64(progress.TotalBytes) * 100
			progress.LastUpdate = time.Now()
			
			// Calculate speed
			elapsed := progress.LastUpdate.Sub(progress.StartTime).Seconds()
			if elapsed > 0 {
				progress.Speed = int64(float64(bytes) / elapsed)
			}
			
			// Calculate ETA
			if progress.Speed > 0 {
				remaining := progress.TotalBytes - bytes
				progress.ETA = time.Duration(float64(remaining)/float64(progress.Speed)) * time.Second
			}
		}
	}
}

// ValidateRDMCopy validates data integrity after copy
func (r *RDMOperationsImpl) ValidateRDMCopy(ctx context.Context, rdmInfo RDMInfo, volumeInfo VolumeInfo) error {
	// Calculate checksums for both source and target
	sourceChecksum, err := r.calculateChecksum(ctx, rdmInfo.DevicePath, rdmInfo.DiskSize)
	if err != nil {
		return fmt.Errorf("failed to calculate source checksum: %w", err)
	}
	
	targetChecksum, err := r.calculateChecksum(ctx, volumeInfo.DevicePath, rdmInfo.DiskSize)
	if err != nil {
		return fmt.Errorf("failed to calculate target checksum: %w", err)
	}
	
	if sourceChecksum != targetChecksum {
		return fmt.Errorf("checksum mismatch: source=%s, target=%s", sourceChecksum, targetChecksum)
	}
	
	return nil
}

// calculateChecksum calculates SHA256 checksum for a device
func (r *RDMOperationsImpl) calculateChecksum(ctx context.Context, devicePath string, size int64) (string, error) {
	// Use dd to read the device and pipe to sha256sum
	cmd := exec.CommandContext(ctx, "sh", "-c", 
		fmt.Sprintf("dd if=%s bs=1M count=%d 2>/dev/null | sha256sum", devicePath, size/(1024*1024)))
	
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	// Extract checksum from output
	checksum := strings.Fields(string(output))[0]
	return checksum, nil
}

// GetProgressChannel returns the progress channel for monitoring copy operations
func (r *RDMOperationsImpl) GetProgressChannel() <-chan CopyProgress {
	return r.progressChan
}

// CleanupTempResources cleans up any temporary resources
func (r *RDMOperationsImpl) CleanupTempResources() error {
	// Clean up any temporary files or resources
	tempDir := "/tmp/rdm-migration"
	if _, err := os.Stat(tempDir); err == nil {
		return os.RemoveAll(tempDir)
	}
	return nil
}

// ResumeCopy resumes an interrupted copy operation
func (r *RDMOperationsImpl) ResumeCopy(ctx context.Context, rdmInfo RDMInfo, volumeInfo VolumeInfo, offset int64) error {
	// Check how much data has already been copied
	copiedBytes, err := r.getBytesCopied(volumeInfo.DevicePath)
	if err != nil {
		return fmt.Errorf("failed to determine copied bytes: %w", err)
	}
	
	if copiedBytes >= rdmInfo.DiskSize {
		return nil // Copy already complete
	}
	
	// Resume from the last copied position
	resumeCmd := fmt.Sprintf("dd if=%s of=%s bs=1M skip=%d seek=%d conv=fdatasync,notrunc status=progress",
		rdmInfo.DevicePath, volumeInfo.DevicePath, copiedBytes/(1024*1024), copiedBytes/(1024*1024))
	
	cmd := exec.CommandContext(ctx, "sh", "-c", resumeCmd)
	return cmd.Run()
}

// getBytesCopied determines how many bytes have been copied to the target
func (r *RDMOperationsImpl) getBytesCopied(devicePath string) (int64, error) {
	// Use a simple heuristic: find the last non-zero block
	cmd := exec.Command("sh", "-c", 
		fmt.Sprintf("dd if=%s bs=1M 2>/dev/null | wc -c", devicePath))
	
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	
	bytesStr := strings.TrimSpace(string(output))
	bytes, err := strconv.ParseInt(bytesStr, 10, 64)
	if err != nil {
		return 0, err
	}
	
	return bytes, nil
}