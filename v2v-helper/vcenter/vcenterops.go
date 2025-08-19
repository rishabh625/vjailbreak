// Copyright © 2024 The vjailbreak authors

package vcenter

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/session/cache"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
)

//go:generate mockgen -source=../vcenter/vcenterops.go -destination=../vcenter/vcenterops_mock.go -package=vcenter

type VCenterOperations interface {
	getDatacenters(ctx context.Context) ([]*object.Datacenter, error)
	GetVMByName(ctx context.Context, name string) (*object.VirtualMachine, error)
}

type VCenterClient struct {
	VCClient            *vim25.Client
	VCFinder            *find.Finder
	VCPropertyCollector *property.Collector
}

func validateVCenter(ctx context.Context, username, password, host string, disableSSLVerification bool) (*vim25.Client, error) {
	// add protocol to host if not present
	if !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}

	// add SDK path if not present
	if len(host) < 4 || !strings.HasSuffix(host, "/sdk") {
		// Length check ensures we don't crash on short hosts when checking suffix
		host += "/sdk"
	}

	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %v", err)
	}
	u.User = url.UserPassword(username, password)

	// Create a session with automatic re-authentication
	s := &cache.Session{
		URL:      u,
		Insecure: disableSSLVerification,
		Reauth:   true, // Enable automatic re-authentication
	}

	// Create the client
	c := new(vim25.Client)
	err = s.Login(ctx, c, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to login: %v", err)
	}

	return c, nil
}

func VCenterClientBuilder(ctx context.Context, username, password, host string, disableSSLVerification bool) (*VCenterClient, error) {
	client, err := validateVCenter(ctx, username, password, host, disableSSLVerification)
	if err != nil {
		return nil, fmt.Errorf("failed to validate vCenter connection: %v", err)
	}
	finder := find.NewFinder(client, false)
	pc := property.DefaultCollector(client)
	return &VCenterClient{VCClient: client, VCFinder: finder, VCPropertyCollector: pc}, nil
}

func GetThumbprint(host string) (string, error) {
	// Get the thumbprint of the vCenter server
	// Establish a TLS connection to the server
	conn, err := tls.Dial("tcp", host+":443", &tls.Config{
		InsecureSkipVerify: true, // Skip verification
	})
	if err != nil {
		return "", fmt.Errorf("failed to connect to vCenter: %v", err)
	}
	defer conn.Close()

	// Get the server's certificates
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", fmt.Errorf("no certificates found")
	}

	// Compute the SHA-1 thumbprint of the first certificate
	cert := certs[0]
	thumbprint := ""

	for idx, thumbyte := range sha1.Sum(cert.Raw) {
		thumbprint += hex.EncodeToString([]byte{thumbyte})
		if idx < len(sha1.Sum(cert.Raw))-1 {
			thumbprint += ":"
		}
	}

	// Return the thumbprint as a hexadecimal string
	return thumbprint, nil
}

// Get all datacenters
func (vcclient *VCenterClient) getDatacenters(ctx context.Context) ([]*object.Datacenter, error) {
	// Find all datacenters
	datacenters, err := vcclient.VCFinder.DatacenterList(ctx, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get datacenters: %v", err)
	}

	return datacenters, nil
}

// get VM by name
func (vcclient *VCenterClient) GetVMByName(ctx context.Context, name string) (*object.VirtualMachine, error) {
	datacenters, err := vcclient.getDatacenters(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get datacenters: %v", err)
	}
	for _, datacenter := range datacenters {
		vcclient.VCFinder.SetDatacenter(datacenter)
		vm, err := vcclient.VCFinder.VirtualMachine(ctx, name)
		if err == nil {
			return vm, nil
		}
	}
	return nil, fmt.Errorf("VM not found")
}

// RenameVM renames a VM in vCenter by appending a suffix to its name
func (vcclient *VCenterClient) RenameVM(ctx context.Context, vmName, newVMName string) error {
	// Find the VM
	vm, err := vcclient.GetVMByName(ctx, vmName)
	if err != nil {
		return fmt.Errorf("failed to find VM '%s': %v", vmName, err)
	}

	// Rename the VM
	spec := types.VirtualMachineConfigSpec{
		Name: newVMName,
	}
	task, err := vm.Reconfigure(ctx, spec)
	if err != nil {
		return fmt.Errorf("failed to reconfigure VM '%s' with new name '%s': %v", vmName, newVMName, err)
	}
	err = task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to rename VM '%s' to '%s': %v", vmName, newVMName, err)
	}

	return nil
}

// MoveVMFolder moves a VM to a specified folder in vCenter
func (vcclient *VCenterClient) MoveVMFolder(ctx context.Context, vmName, folderName string) error {
	// Find the VM
	vm, err := vcclient.GetVMByName(ctx, vmName)
	if err != nil {
		return fmt.Errorf("failed to find VM '%s': %v", vmName, err)
	}

	// Find the target folder
	folderRef, err := vcclient.VCFinder.Folder(ctx, folderName)
	if err != nil {
		return fmt.Errorf("failed to find folder '%s': %v", folderName, err)
	}

	// Move the VM to the folder
	task, err := folderRef.MoveInto(ctx, []types.ManagedObjectReference{vm.Reference()})
	if err != nil {
		return fmt.Errorf("failed to initiate move of VM '%s' to folder '%s': %v", vmName, folderName, err)
	}
	err = task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to move VM '%s' to folder '%s': %v", vmName, folderName, err)
	}

	return nil
}
