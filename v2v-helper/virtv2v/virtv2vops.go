// Copyright © 2024 The vjailbreak authors

package virtv2v

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"
	"github.com/platform9/vjailbreak/v2v-helper/vm"
)

//go:generate mockgen -source=../virtv2v/virtv2vops.go -destination=../virtv2v/virtv2vops_mock.go -package=virtv2v

type VirtV2VOperations interface {
	RetainAlphanumeric(input string) string
	GetPartitions(disk string) ([]string, error)
	NTFSFix(path string) error
	ConvertDisk(ctx context.Context, path, ostype, virtiowindriver string, firstbootscripts []string, useSingleDisk bool, diskPath string) error
	AddWildcardNetplan(path string) error
	GetOsRelease(path string) (string, error)
	AddFirstBootScript(firstbootscript, firstbootscriptname string) error
	AddUdevRules(disks []vm.VMDisk, useSingleDisk bool, diskPath string, interfaces []string, macs []string) error
	GetNetworkInterfaceNames(path string) ([]string, error)
	IsRHELFamily(osRelease string) (bool, error)
	GetOsReleaseAllVolumes(disks []vm.VMDisk) (string, error)
}

func RetainAlphanumeric(input string) string {
	var builder strings.Builder
	for _, char := range input {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func IsRHELFamily(osRelease string) bool {
	lowerRelease := strings.ToLower(osRelease)
	return strings.Contains(lowerRelease, "red hat") ||
		strings.Contains(lowerRelease, "rhel") ||
		strings.Contains(lowerRelease, "centos") ||
		strings.Contains(lowerRelease, "rocky") ||
		strings.Contains(lowerRelease, "alma")
}

func GetPartitions(disk string) ([]string, error) {
	// Execute lsblk command to get partition information
	cmd := exec.Command("lsblk", "-no", "NAME", disk)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to execute lsblk: %w", err)
	}

	var partitions []string
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && line != disk {
			partitions = append(partitions, "/dev/"+RetainAlphanumeric(line))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading lsblk output: %w", err)
	}

	return partitions, nil
}

func NTFSFix(path string) error {
	// Fix NTFS
	partitions, err := GetPartitions(path)
	if err != nil {
		return fmt.Errorf("failed to get partitions: %w", err)
	}
	log.Printf("Partitions: %v", partitions)
	for _, partition := range partitions {
		if partition == path {
			continue
		}
		cmd := exec.Command("ntfsfix", partition)
		log.Printf("Executing %s", cmd.String())

		err := cmd.Run()
		if err != nil {
			log.Printf("Skipping NTFS fix on %s", partition)
		}
		log.Printf("Fixed NTFS on %s", partition)
	}
	return nil
}

func downloadFile(url, filePath string) error {
	// Get the data from the URL
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download file: %s", err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create the file
	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %s", err)
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write to file: %s", err)
	}
	return nil
}

func CheckForVirtioDrivers() (bool, error) {

	// Before downloading virtio windrivers Check if iso is present in the path
	preDownloadPath := "/home/fedora/virtio-win"

	// Check if path exists
	_, err := os.Stat(preDownloadPath)
	if err != nil {
		return false, fmt.Errorf("failed to check if path exists: %s", err)
	}
	// Check if iso is present in the path
	files, err := os.ReadDir(preDownloadPath)
	if err != nil {
		return false, fmt.Errorf("failed to read directory: %s", err)
	}
	for _, file := range files {
		if file.Name() == "virtio-win.iso" {
			log.Println("Found virtio windrivers")
			return true, nil
		}
	}
	return false, nil
}

func ConvertDisk(ctx context.Context, xmlFile, path, ostype, virtiowindriver string, firstbootscripts []string, useSingleDisk bool, diskPath string) error {
	// Step 1: Handle Windows driver injection
	if strings.ToLower(ostype) == constants.OSFamilyWindows {
		filePath := "/home/fedora/virtio-win/virtio-win.iso"

		found, err := CheckForVirtioDrivers()
		if err != nil {
			log.Printf("failed to check for virtio drivers: %s", err)
			log.Println("Downloading virtio windrivers instead of using the existing one")
		}
		if found {
			log.Println("Found virtio windrivers")
		} else {
			log.Println("Downloading virtio windrivers")
			err := downloadFile(virtiowindriver, filePath)
			if err != nil {
				return fmt.Errorf("failed to download virtio-win: %s", err)
			}
			log.Println("Downloaded virtio windrivers")
		}
		os.Setenv("VIRTIO_WIN", filePath)
	}

	// Step 2: Set guestfs backend
	os.Setenv("LIBGUESTFS_BACKEND", "direct")

	// Step 3: Prepare virt-v2v args
	args := []string{"-v", "--firstboot", "/home/fedora/scripts/user_firstboot.sh"}
	for _, script := range firstbootscripts {
		args = append(args, "--firstboot", fmt.Sprintf("/home/fedora/%s.sh", script))
	}
	if useSingleDisk {
		args = append(args, "-i", "disk", diskPath)
	} else {
		args = append(args, "-i", "libvirtxml", xmlFile, "--root", path)
	}

	start := time.Now()
	// Step 5: Run virt-v2v-in-place
	cmd := exec.CommandContext(ctx, "virt-v2v-in-place", args...)
	log.Printf("Executing %s", cmd.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		return fmt.Errorf("failed to run virt-v2v-in-place: %s", err)
	}
	log.Printf("virt-v2v-in-place conversion took: %s", duration)
	return nil
}

func GetOsRelease(path string) (string, error) {
	os.Setenv("LIBGUESTFS_BACKEND", "direct")

	// Attempt to cat /etc/os-release
	cmd := exec.Command("guestfish", "--ro", "-a", path, "-i")
	input := `cat /etc/os-release`
	cmd.Stdin = strings.NewReader(input)
	log.Printf("Executing %s with input: %s", cmd.String(), input)
	out, err := cmd.CombinedOutput() // Use CombinedOutput to capture stderr for error parsing
	if err == nil {
		return strings.ToLower(string(out)), nil
	}

	// Check if the error is due to missing /etc/os-release
	errorOutput := strings.TrimSpace(string(out))
	log.Printf("Failed to get /etc/os-release: %v, error: %v", errorOutput, err)
	err = nil
	if strings.Contains(strings.ToLower(errorOutput), "no such file or directory") {
		// Fallback to /etc/redhat-release
		cmd = exec.Command("guestfish", "--ro", "-a", path, "-i")
		input = `cat /etc/redhat-release`
		cmd.Stdin = strings.NewReader(input)
		log.Printf("Executing %s with input: %s", cmd.String(), input)
		out, err = cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to get redhat-release: %s, %v", string(out), err)
		}
		return strings.ToLower(string(out)), nil
	}

	// If not a missing file error, return the original error
	return "", fmt.Errorf("failed to get os-release: %s, %v", errorOutput, err)
}

func AddWildcardNetplan(disks []vm.VMDisk, useSingleDisk bool, diskPath string) error {
	// Add wildcard to netplan
	var ans string
	netplan := `[Match]
Name=en*

[Network]
DHCP=yes`

	// Create the netplan file
	err := os.WriteFile("/home/fedora/99-wildcard.network", []byte(netplan), 0644)
	if err != nil {
		return fmt.Errorf("failed to create netplan file: %s", err)
	}
	log.Println("Created local netplan file")
	log.Println("Uploading netplan file to disk")
	// Upload it to the disk
	os.Setenv("LIBGUESTFS_BACKEND", "direct")
	if useSingleDisk {
		command := `upload /home/fedora/99-wildcard.network /etc/systemd/network/99-wildcard.network`
		ans, err = RunCommandInGuest(diskPath, command, true)
	} else {
		command := "upload"
		ans, err = RunCommandInGuestAllVolumes(disks, command, true, "/home/fedora/99-wildcard.network", "/etc/systemd/network/99-wildcard.network")
	}
	if err != nil {
		fmt.Printf("failed to run command (%s): %v: %s\n", "upload", err, strings.TrimSpace(ans))
		return err
	}
	return nil
}

func AddFirstBootScript(firstbootscript, firstbootscriptname string) error {
	// Create the firstboot script
	firstbootscriptpath := fmt.Sprintf("/home/fedora/%s.sh", firstbootscriptname)
	err := os.WriteFile(firstbootscriptpath, []byte(firstbootscript), 0644)
	if err != nil {
		return fmt.Errorf("failed to create firstboot script: %s", err)
	}
	log.Printf("Created firstboot script %s", firstbootscriptname)
	return nil
}

// Runs command inside temporary qemu-kvm that virt-v2v creates
func RunCommandInGuest(path string, command string, write bool) (string, error) {
	os.Setenv("LIBGUESTFS_BACKEND", "direct")
	option := "--ro"
	if write {
		option = "--rw"
	}
	cmd := exec.Command(
		"guestfish",
		option,
		"-a",
		path,
		"-i")
	cmd.Stdin = strings.NewReader(command)
	log.Printf("Executing %s", cmd.String()+" "+command)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run command (%s): %v: %s", command, err, strings.TrimSpace(string(out)))
	}
	return strings.ToLower(strings.TrimSpace(string(out))), nil
}

// Runs command inside temporary qemu-kvm that virt-v2v creates
func CheckForLVM(disks []vm.VMDisk) (string, error) {
	os.Setenv("LIBGUESTFS_BACKEND", "direct")

	// Get the installed os info
	command := "inspect-os"
	osPath, err := RunCommandInGuestAllVolumes(disks, command, false)
	if err != nil {
		return "", fmt.Errorf("failed to run command (%s): %v: %s", command, err, strings.TrimSpace(osPath))
	}

	// Get the lvs list
	command = "lvs"
	lvsStr, err := RunCommandInGuestAllVolumes(disks, command, false)
	if err != nil {
		return "", fmt.Errorf("failed to run command (%s): %v: %s", command, err, strings.TrimSpace(lvsStr))
	}

	lvs := strings.Split(string(lvsStr), "\n")
	if slices.Contains(lvs, strings.TrimSpace(string(osPath))) {
		return string(strings.TrimSpace(string(osPath))), nil
	}

	return "", fmt.Errorf("LVM not found: %v, %d", lvs, len(lvs))
}

func prepareGuestfishCommand(disks []vm.VMDisk, command string, write bool, args ...string) *exec.Cmd {
	option := "--ro"
	if write {
		option = "--rw"
	}
	cmd := exec.Command(
		"guestfish",
		option)

	for _, disk := range disks {
		cmd.Args = append(cmd.Args, "-a", disk.Path)
	}
	cmd.Args = append(cmd.Args, "-i", command)
	cmd.Args = append(cmd.Args, args...)
	return cmd
}

func RunCommandInGuestAllVolumes(disks []vm.VMDisk, command string, write bool, args ...string) (string, error) {
	os.Setenv("LIBGUESTFS_BACKEND", "direct")
	cmd := prepareGuestfishCommand(disks, command, write, args...)
	log.Printf("Executing %s", cmd.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run command (%s): %v: %s", command, err, strings.TrimSpace(string(out)))
	}
	return strings.ToLower(string(out)), nil
}

func GetBootableVolumeIndex(disks []vm.VMDisk) (int, error) {
	command := "list-partitions"
	partitionsStr, err := RunCommandInGuestAllVolumes(disks, command, false)
	if err != nil {
		return -1, fmt.Errorf("failed to run command (%s): %v: %s", command, err, strings.TrimSpace(partitionsStr))
	}

	partitions := strings.Split(strings.TrimSpace(partitionsStr), "\n")
	for _, partition := range partitions {
		command := "part-to-dev"
		device, err := RunCommandInGuestAllVolumes(disks, command, false, strings.TrimSpace(partition))
		if err != nil {
			fmt.Printf("failed to run command (%s): %v: %s\n", device, err, strings.TrimSpace(device))
			return -1, err
		}

		command = "part-to-partnum"
		num, err := RunCommandInGuestAllVolumes(disks, command, false, strings.TrimSpace(partition))
		if err != nil {
			fmt.Printf("failed to run command (%s): %v: %s\n", num, err, strings.TrimSpace(num))
			return -1, err
		}

		command = "part-get-bootable"
		bootable, err := RunCommandInGuestAllVolumes(disks, command, false, strings.TrimSpace(device), strings.TrimSpace(num))
		if err != nil {
			fmt.Printf("failed to run command (%s): %v: %s\n", bootable, err, strings.TrimSpace(bootable))
			return -1, err
		}

		if strings.TrimSpace(bootable) == "true" {
			command = "device-index"
			index, err := RunCommandInGuestAllVolumes(disks, command, false, strings.TrimSpace(device))
			if err != nil {
				fmt.Printf("failed to run command (%s): %v: %s\n", index, err, strings.TrimSpace(index))
				return -1, err
			}
			return strconv.Atoi(strings.TrimSpace(index))
		}
	}
	return -1, errors.New("bootable volume not found")
}

func AddUdevRules(disks []vm.VMDisk, useSingleDisk bool, diskPath string, interfaces []string, macs []string) error {

	if len(interfaces) != len(macs) {
		return fmt.Errorf("mismatch between number of interfaces and MACs")
	}
	var ans string

	// Create the udev rules content
	var udevRules strings.Builder
	for i, iface := range interfaces {
		udevRules.WriteString(fmt.Sprintf("SUBSYSTEM==\"net\", ACTION==\"add\", ATTR{address}==\"%s\", NAME=\"%s\"\n", macs[i], iface))
		log.Printf("Adding udev rule: %s", udevRules.String())
	}

	err := os.WriteFile("/home/fedora/70-persistent-net.rules", []byte(udevRules.String()), 0644)
	if err != nil {
		return fmt.Errorf("failed to create udev rules file: %s", err)
	}
	log.Println("Uploading udev rules file to disk")
	// Upload it to the disk
	os.Setenv("LIBGUESTFS_BACKEND", "direct")
	if useSingleDisk {
		command := `upload /home/fedora/70-persistent-net.rules /etc/udev/rules.d/70-persistent-net.rules`
		ans, err = RunCommandInGuest(diskPath, command, true)
	} else {
		command := "upload"
		ans, err = RunCommandInGuestAllVolumes(disks, command, true, "/home/fedora/70-persistent-net.rules", "/etc/udev/rules.d/70-persistent-net.rules")
	}
	if err != nil {
		fmt.Printf("failed to run command (%s): %v: %s\n", "upload", err, strings.TrimSpace(ans))
		return err
	}
	return nil
}

func GetNetworkInterfaceNames(path string) ([]string, error) {
	// Get the network interface names
	command := "cat /etc/network/interfaces"
	ans, err := RunCommandInGuest(path, command, false)
	if err != nil {
		return nil, fmt.Errorf("failed to run command (%s): %v: %s", command, err, strings.TrimSpace(ans))
	}

	// Parse the output
	lines := strings.Split(ans, "\n")
	var interfaces []string
	for _, line := range lines {
		if strings.HasPrefix(line, "iface") && !strings.Contains(line, "lo") {
			interfaces = append(interfaces, strings.Fields(line)[1])
		}
	}
	return interfaces, nil

}

func GetInterfaceNames(path string) ([]string, error) {
	cmd := "ls /etc/sysconfig/network-scripts | grep '^ifcfg-'"
	lsOut, err := RunCommandInGuest(path, cmd, false)
	if err != nil {
		return nil, err
	}
	interfaces := []string{}
	// Parse the output
	// Split by newline and trim spaces
	// Ignore 'ifcfg-lo' as it is the loopback interface
	files := strings.Split(strings.TrimSpace(lsOut), "\n")
	for _, file := range files {
		if file == "ifcfg-lo" {
			continue
		}
		content, err := RunCommandInGuest(path, fmt.Sprintf("cat /etc/sysconfig/network-scripts/%s", file), false)
		if err != nil {
			continue
		}
		// Extract DEVICE or infer from filename
		device := extractKeyValue(content, "DEVICE")
		if device != "" {
			interfaces = append(interfaces, device)
		} else {
			// Fall back to filename if DEVICE not found
			device = strings.TrimPrefix(file, "ifcfg-")
			if device != "" {
				interfaces = append(interfaces, device)
			}
		}
	}

	return interfaces, nil
}

// Helper: Extract key=value from content, trim quotes/spaces
func extractKeyValue(content, key string) string {
	re := regexp.MustCompile(fmt.Sprintf(`(?m)^%s=(.*)$`, key))
	match := re.FindStringSubmatch(content)
	if len(match) > 1 {
		return strings.Trim(strings.Trim(match[1], `"'`), " ")
	}
	return ""
}

func GetOsReleaseAllVolumes(disks []vm.VMDisk) (string, error) {
	// Attempt /etc/os-release first
	osRelease, err := RunCommandInGuestAllVolumes(disks, "cat", false, "/etc/os-release")
	if err == nil {
		return osRelease, nil
	}
	log.Printf("Failed to get /etc/os-release: %v", err)
	// Fallback if file is missing
	if strings.Contains(err.Error(), "No such file or directory") {
		fallbackOutput, fallbackErr := RunCommandInGuestAllVolumes(disks, "cat", false, "/etc/redhat-release")
		if fallbackErr != nil {
			return "", fmt.Errorf("failed to get OS release: primary (/etc/os-release): %v, fallback (/etc/redhat-release): %v", err, fallbackErr)
		}
		return fallbackOutput, nil
	}

	// Return original error if not a missing file issue
	return "", err
}
