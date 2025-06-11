// Package utils provides utility functions for handling credentials and other operations
package utils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"

	gophercloud "github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumetypes"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/pkg/errors"
	"github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	scope "github.com/platform9/vjailbreak/k8s/migration/pkg/scope"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/session/cache"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	govmitypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenStackClients holds clients for interacting with OpenStack services
type OpenStackClients struct {
	// BlockStorageClient is the client for interacting with OpenStack Block Storage
	BlockStorageClient *gophercloud.ServiceClient
	// ComputeClient is the client for interacting with OpenStack Compute
	ComputeClient *gophercloud.ServiceClient
	// NetworkingClient is the client for interacting with OpenStack Networking
	NetworkingClient *gophercloud.ServiceClient
}

const (
	trueString = "true" // Define at package level
)

// GetVMwareCredsInfo retrieves vCenter credentials from a secret
func GetVMwareCredsInfo(ctx context.Context, k3sclient client.Client, credsName string) (vjailbreakv1alpha1.VMwareCredsInfo, error) {
	creds := vjailbreakv1alpha1.VMwareCreds{}
	if err := k3sclient.Get(ctx, k8stypes.NamespacedName{Namespace: constants.NamespaceMigrationSystem, Name: credsName}, &creds); err != nil {
		return vjailbreakv1alpha1.VMwareCredsInfo{}, errors.Wrapf(err, "failed to get VMware credentials '%s'", credsName)
	}
	return GetVMwareCredentialsFromSecret(ctx, k3sclient, creds.Spec.SecretRef.Name)
}

// GetOpenstackCredsInfo retrieves OpenStack credentials from a secret
func GetOpenstackCredsInfo(ctx context.Context, k3sclient client.Client, credsName string) (vjailbreakv1alpha1.OpenStackCredsInfo, error) {
	creds := vjailbreakv1alpha1.OpenstackCreds{}
	if err := k3sclient.Get(ctx, k8stypes.NamespacedName{Namespace: constants.NamespaceMigrationSystem, Name: credsName}, &creds); err != nil {
		return vjailbreakv1alpha1.OpenStackCredsInfo{}, errors.Wrapf(err, "failed to get OpenStack credentials '%s'", credsName)
	}
	return GetOpenstackCredentialsFromSecret(ctx, k3sclient, creds.Spec.SecretRef.Name)
}

// GetVMwareCredentialsFromSecret retrieves vCenter credentials from a secret
func GetVMwareCredentialsFromSecret(ctx context.Context, k3sclient client.Client, secretName string) (vjailbreakv1alpha1.VMwareCredsInfo, error) {
	secret := &corev1.Secret{}

	// Get In cluster client
	if err := k3sclient.Get(ctx, k8stypes.NamespacedName{Namespace: constants.NamespaceMigrationSystem, Name: secretName}, secret); err != nil {
		return vjailbreakv1alpha1.VMwareCredsInfo{}, errors.Wrapf(err, "failed to get secret '%s'", secretName)
	}

	if secret.Data == nil {
		return vjailbreakv1alpha1.VMwareCredsInfo{}, fmt.Errorf("no data in secret '%s'", secretName)
	}

	host := string(secret.Data["VCENTER_HOST"])
	username := string(secret.Data["VCENTER_USERNAME"])
	password := string(secret.Data["VCENTER_PASSWORD"])
	insecureStr := string(secret.Data["VCENTER_INSECURE"])
	datacenter := string(secret.Data["VCENTER_DATACENTER"])

	if host == "" {
		return vjailbreakv1alpha1.VMwareCredsInfo{}, errors.Errorf("VCENTER_HOST is missing in secret '%s'", secretName)
	}
	if username == "" {
		return vjailbreakv1alpha1.VMwareCredsInfo{}, errors.Errorf("VCENTER_USERNAME is missing in secret '%s'", secretName)
	}
	if password == "" {
		return vjailbreakv1alpha1.VMwareCredsInfo{}, errors.Errorf("VCENTER_PASSWORD is missing in secret '%s'", secretName)
	}
	if datacenter == "" {
		return vjailbreakv1alpha1.VMwareCredsInfo{}, errors.Errorf("VCENTER_DATACENTER is missing in secret '%s'", secretName)
	}

	insecure := strings.EqualFold(strings.TrimSpace(insecureStr), trueString)

	return vjailbreakv1alpha1.VMwareCredsInfo{
		Host:       host,
		Username:   username,
		Password:   password,
		Datacenter: datacenter,
		Insecure:   insecure,
	}, nil
}

// GetOpenstackCredentialsFromSecret retrieves and checks the secret
func GetOpenstackCredentialsFromSecret(ctx context.Context, k3sclient client.Client, secretName string) (vjailbreakv1alpha1.OpenStackCredsInfo, error) {
	secret := &corev1.Secret{}
	if err := k3sclient.Get(ctx, k8stypes.NamespacedName{Namespace: constants.NamespaceMigrationSystem, Name: secretName}, secret); err != nil {
		return vjailbreakv1alpha1.OpenStackCredsInfo{}, errors.Wrap(err, "failed to get secret")
	}

	// Extract and validate each field
	fields := map[string]string{
		"AuthURL":    string(secret.Data["OS_AUTH_URL"]),
		"DomainName": string(secret.Data["OS_DOMAIN_NAME"]),
		"Username":   string(secret.Data["OS_USERNAME"]),
		"Password":   string(secret.Data["OS_PASSWORD"]),
		"TenantName": string(secret.Data["OS_TENANT_NAME"]),
		"RegionName": string(secret.Data["OS_REGION_NAME"]),
	}

	for key, value := range fields {
		if value == "" {
			return vjailbreakv1alpha1.OpenStackCredsInfo{}, errors.Errorf("%s is missing in secret '%s'", key, secretName)
		}
	}

	insecureStr := string(secret.Data["OS_INSECURE"])
	insecure := strings.EqualFold(strings.TrimSpace(insecureStr), trueString)

	return vjailbreakv1alpha1.OpenStackCredsInfo{
		AuthURL:    fields["AuthURL"],
		DomainName: fields["DomainName"],
		Username:   fields["Username"],
		Password:   fields["Password"],
		RegionName: fields["RegionName"],
		TenantName: fields["TenantName"],
		Insecure:   insecure,
	}, nil
}

// GetCert retrieves an X.509 certificate from an endpoint
func GetCert(endpoint string) (*x509.Certificate, error) {
	conf := &tls.Config{
		//nolint:gosec // This is required to skip certificate verification
		InsecureSkipVerify: true,
	}
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing URL")
	}
	hostname := parsedURL.Hostname()
	conn, err := tls.Dial("tcp", hostname+":443", conf)
	if err != nil {
		return nil, errors.Wrapf(err, "error connecting to %s", hostname)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			ctrllog.Log.Info("Error closing connection", "error", err)
		}
	}()
	cert := conn.ConnectionState().PeerCertificates[0]
	return cert, nil
}

// VerifyNetworks verifies the existence of specified networks in OpenStack
func VerifyNetworks(ctx context.Context, k3sclient client.Client, openstackcreds *vjailbreakv1alpha1.OpenstackCreds, targetnetworks []string) error {
	openstackClients, err := GetOpenStackClients(ctx, k3sclient, openstackcreds)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack clients")
	}
	allPages, err := networks.List(openstackClients.NetworkingClient, nil).AllPages()
	if err != nil {
		return errors.Wrap(err, "failed to list networks")
	}

	allNetworks, err := networks.ExtractNetworks(allPages)
	if err != nil {
		return errors.Wrap(err, "failed to extract all networks")
	}

	// Build a map of all networks
	networkMap := make(map[string]bool)
	for i := 0; i < len(allNetworks); i++ {
		networkMap[allNetworks[i].Name] = true
	}

	// Verify that all network names in targetnetworks exist in the openstack networks
	for _, targetNetwork := range targetnetworks {
		if _, found := networkMap[targetNetwork]; !found {
			return fmt.Errorf("network '%s' not found in OpenStack", targetNetwork)
		}
	}
	return nil
}

// VerifyPorts verifies the existence of specified ports in OpenStack
func VerifyPorts(ctx context.Context, k3sclient client.Client, openstackcreds *vjailbreakv1alpha1.OpenstackCreds, targetports []string) error {
	openstackClients, err := GetOpenStackClients(ctx, k3sclient, openstackcreds)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack clients")
	}

	allPages, err := ports.List(openstackClients.NetworkingClient, nil).AllPages()
	if err != nil {
		return errors.Wrap(err, "failed to list ports")
	}

	allPorts, err := ports.ExtractPorts(allPages)
	if err != nil {
		return errors.Wrap(err, "failed to extract all ports")
	}

	// Build a map of all ports
	portMap := make(map[string]bool)
	for i := 0; i < len(allPorts); i++ {
		portMap[allPorts[i].ID] = true
	}

	// Verify that all port names in targetports exist in the openstack ports
	for _, targetPort := range targetports {
		if _, found := portMap[targetPort]; !found {
			return errors.Wrap(fmt.Errorf("port '%s' not found in OpenStack", targetPort), "failed to verify ports")
		}
	}
	return nil
}

// VerifyStorage verifies the existence of specified storage in OpenStack
func VerifyStorage(ctx context.Context, k3sclient client.Client, openstackcreds *vjailbreakv1alpha1.OpenstackCreds, targetstorages []string) error {
	openstackClients, err := GetOpenStackClients(ctx, k3sclient, openstackcreds)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack clients")
	}
	allPages, err := volumetypes.List(openstackClients.BlockStorageClient, nil).AllPages()
	if err != nil {
		return errors.Wrap(err, "failed to list volume types")
	}

	allvoltypes, err := volumetypes.ExtractVolumeTypes(allPages)
	if err != nil {
		return errors.Wrap(err, "failed to extract all volume types")
	}

	// Verify that all volume types in targetstorage exist in the openstack volume types
	for _, targetstorage := range targetstorages {
		found := false
		for i := 0; i < len(allvoltypes); i++ {
			if allvoltypes[i].Name == targetstorage {
				found = true
				break
			}
		}
		if !found {
			return errors.Wrap(fmt.Errorf("volume type '%s' not found in OpenStack", targetstorage), "failed to verify volume types")
		}
	}
	return nil
}

// GetOpenstackInfo retrieves OpenStack information using provided credentials
func GetOpenstackInfo(ctx context.Context, k3sclient client.Client, openstackcreds *vjailbreakv1alpha1.OpenstackCreds) (*vjailbreakv1alpha1.OpenstackInfo, error) {
	openstackClients, err := GetOpenStackClients(ctx, k3sclient, openstackcreds)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get openstack clients")
	}
	var openstackvoltypes []string
	var openstacknetworks []string
	allVolumeTypePages, err := volumetypes.List(openstackClients.BlockStorageClient, nil).AllPages()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list volume types")
	}

	allvoltypes, err := volumetypes.ExtractVolumeTypes(allVolumeTypePages)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract all volume types")
	}

	for i := 0; i < len(allvoltypes); i++ {
		openstackvoltypes = append(openstackvoltypes, allvoltypes[i].Name)
	}

	allNetworkPages, err := networks.List(openstackClients.NetworkingClient, nil).AllPages()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list networks")
	}

	allNetworks, err := networks.ExtractNetworks(allNetworkPages)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract all networks")
	}

	for i := 0; i < len(allNetworks); i++ {
		openstacknetworks = append(openstacknetworks, allNetworks[i].Name)
	}

	return &vjailbreakv1alpha1.OpenstackInfo{
		VolumeTypes: openstackvoltypes,
		Networks:    openstacknetworks,
	}, nil
}

// GetOpenStackClients is a function to create openstack clients
func GetOpenStackClients(ctx context.Context, k3sclient client.Client, openstackcreds *vjailbreakv1alpha1.OpenstackCreds) (*OpenStackClients, error) {
	if openstackcreds == nil {
		return nil, errors.New("openstackcreds cannot be nil")
	}

	openstackCredential, err := GetOpenstackCredentialsFromSecret(ctx, k3sclient, openstackcreds.Spec.SecretRef.Name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get openstack credentials from secret")
	}

	endpoint := gophercloud.EndpointOpts{
		Region: openstackCredential.RegionName,
	}
	providerClient, err := ValidateAndGetProviderClient(ctx, k3sclient, openstackcreds)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get provider client for region '%s'", openstackCredential.RegionName))
	}
	if providerClient == nil {
		return nil, fmt.Errorf("failed to get provider client for region '%s'", openstackCredential.RegionName)
	}
	computeClient, err := openstack.NewComputeV2(providerClient, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to create openstack compute client for region '%s'", openstackCredential.RegionName))
	}
	blockStorageClient, err := openstack.NewBlockStorageV3(providerClient, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to create openstack block storage client for region '%s'",
			openstackCredential.RegionName))
	}
	networkingClient, err := openstack.NewNetworkV2(providerClient, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to create openstack networking client for region '%s'",
			openstackCredential.RegionName))
	}

	return &OpenStackClients{
		BlockStorageClient: blockStorageClient,
		ComputeClient:      computeClient,
		NetworkingClient:   networkingClient,
	}, nil
}

// ValidateAndGetProviderClient is a function to get provider client
func ValidateAndGetProviderClient(ctx context.Context, k3sclient client.Client,
	openstackcreds *vjailbreakv1alpha1.OpenstackCreds) (*gophercloud.ProviderClient, error) {
	openstackCredential, err := GetOpenstackCredentialsFromSecret(ctx, k3sclient, openstackcreds.Spec.SecretRef.Name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get openstack credentials from secret")
	}

	providerClient, err := openstack.NewClient(openstackCredential.AuthURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create openstack client")
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if openstackCredential.Insecure {
		tlsConfig.InsecureSkipVerify = true
	} else {
		// Get the certificate for the Openstack endpoint
		caCert, certerr := GetCert(openstackCredential.AuthURL)
		if certerr != nil {
			return nil, errors.Wrap(certerr, "failed to get certificate for openstack")
		}
		// Trying to fetch the system cert pool and add the Openstack certificate to it
		caCertPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to get system cert pool: %w", err)
		}
		if caCertPool == nil {
			caCertPool = x509.NewCertPool()
		}
		caCertPool.AddCert(caCert)
		tlsConfig.RootCAs = caCertPool
	}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	providerClient.HTTPClient = http.Client{
		Transport: transport,
	}
	err = openstack.Authenticate(providerClient, gophercloud.AuthOptions{
		IdentityEndpoint: openstackCredential.AuthURL,
		Username:         openstackCredential.Username,
		Password:         openstackCredential.Password,
		DomainName:       openstackCredential.DomainName,
		TenantName:       openstackCredential.TenantName,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to authenticate to openstack")
	}

	return providerClient, nil
}

// ValidateVMwareCreds validates the VMware credentials
func ValidateVMwareCreds(ctx context.Context, k3sclient client.Client, vmwcreds *vjailbreakv1alpha1.VMwareCreds) (*vim25.Client, error) {
	vmwareCredsinfo, err := GetVMwareCredentialsFromSecret(ctx, k3sclient, vmwcreds.Spec.SecretRef.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get vCenter credentials from secret: %w", err)
	}

	host := vmwareCredsinfo.Host
	username := vmwareCredsinfo.Username
	password := vmwareCredsinfo.Password
	disableSSLVerification := vmwareCredsinfo.Insecure
	if host[:4] != "http" {
		host = "https://" + host
	}
	if host[len(host)-4:] != "/sdk" {
		host += "/sdk"
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}
	u.User = url.UserPassword(username, password)
	// Connect and log in to ESX or vCenter
	s := &cache.Session{
		URL:      u,
		Insecure: disableSSLVerification,
		Reauth:   true,
	}

	c := new(vim25.Client)
	err = s.Login(context.Background(), c, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to login to vSphere: %w", err)
	}

	// Check if the datacenter exists
	finder := find.NewFinder(c, false)
	_, err = finder.Datacenter(context.Background(), vmwareCredsinfo.Datacenter)
	if err != nil {
		return nil, fmt.Errorf("failed to find datacenter: %w", err)
	}

	return c, nil
}

// GetVMwNetworks gets the networks of a VM
func GetVMwNetworks(ctx context.Context, k3sclient client.Client, vmwcreds *vjailbreakv1alpha1.VMwareCreds, datacenter, vmname string) ([]string, error) {
	// Pre-allocate networks slice to avoid append allocations
	networks := make([]string, 0)
	c, err := ValidateVMwareCreds(ctx, k3sclient, vmwcreds)
	if err != nil {
		return nil, fmt.Errorf("failed to validate vCenter connection: %w", err)
	}
	finder := find.NewFinder(c, false)
	dc, err := finder.Datacenter(ctx, datacenter)
	if err != nil {
		return nil, fmt.Errorf("failed to find datacenter: %w", err)
	}
	finder.SetDatacenter(dc)

	// Get the vm
	vm, err := finder.VirtualMachine(ctx, vmname)
	if err != nil {
		return nil, fmt.Errorf("failed to find vm: %w", err)
	}

	// Get the network name of the VM
	var o mo.VirtualMachine
	err = vm.Properties(ctx, vm.Reference(), []string{"config", "network"}, &o)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM properties: %w", err)
	}

	pc := property.DefaultCollector(c)
	for _, netRef := range o.Network {
		var netObj mo.Network
		err := pc.RetrieveOne(ctx, netRef, []string{"name"}, &netObj)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve network name for %s: %w", netRef.Value, err)
		}
		networks = append(networks, netObj.Name)
	}

	return networks, nil
}

// GetVMwDatastore gets the datastores of a VM
func GetVMwDatastore(ctx context.Context, k3sclient client.Client, vmwcreds *vjailbreakv1alpha1.VMwareCreds, datacenter, vmname string) ([]string, error) {
	c, err := ValidateVMwareCreds(ctx, k3sclient, vmwcreds)
	if err != nil {
		return nil, fmt.Errorf("failed to validate vCenter connection: %w", err)
	}
	finder := find.NewFinder(c, false)
	dc, err := finder.Datacenter(ctx, datacenter)
	if err != nil {
		return nil, fmt.Errorf("failed to find datacenter: %w", err)
	}
	finder.SetDatacenter(dc)

	// Get the vm
	vm, err := finder.VirtualMachine(ctx, vmname)
	if err != nil {
		return nil, fmt.Errorf("failed to find vm: %w", err)
	}

	var vmProps mo.VirtualMachine
	err = vm.Properties(ctx, vm.Reference(), []string{"config"}, &vmProps)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM properties: %w", err)
	}

	var datastores []string
	var ds mo.Datastore
	var dsref govmitypes.ManagedObjectReference
	for _, device := range vmProps.Config.Hardware.Device {
		if _, ok := device.(*govmitypes.VirtualDisk); ok {
			switch backing := device.GetVirtualDevice().Backing.(type) {
			case *govmitypes.VirtualDiskFlatVer2BackingInfo:
				dsref = backing.Datastore.Reference()
			case *govmitypes.VirtualDiskSparseVer2BackingInfo:
				dsref = backing.Datastore.Reference()
			case *govmitypes.VirtualDiskRawDiskMappingVer1BackingInfo:
				dsref = backing.Datastore.Reference()
			default:
				return nil, fmt.Errorf("unsupported disk backing type: %T", device.GetVirtualDevice().Backing)
			}
			err := property.DefaultCollector(c).RetrieveOne(ctx, dsref, []string{"name"}, &ds)
			if err != nil {
				return nil, fmt.Errorf("failed to get datastore: %w", err)
			}

			datastores = append(datastores, ds.Name)
		}
	}
	return datastores, nil
}

// GetAllVMs gets all the VMs in a datacenter
func GetAllVMs(ctx context.Context, k3sclient client.Client, vmwcreds *vjailbreakv1alpha1.VMwareCreds, datacenter string) ([]vjailbreakv1alpha1.VMInfo, error) {
	c, err := ValidateVMwareCreds(ctx, k3sclient, vmwcreds)
	if err != nil {
		return nil, fmt.Errorf("failed to validate vCenter connection: %w", err)
	}
	finder := find.NewFinder(c, false)
	dc, err := finder.Datacenter(ctx, datacenter)
	if err != nil {
		return nil, fmt.Errorf("failed to find datacenter: %w", err)
	}
	finder.SetDatacenter(dc)

	vms, err := finder.VirtualMachineList(ctx, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get vms: %w", err)
	}
	ctxlog := ctrllog.FromContext(ctx)
	// Pre-allocate vminfo slice with capacity of vms to avoid append allocations
	vminfo := make([]vjailbreakv1alpha1.VMInfo, 0, len(vms))
	for _, vm := range vms {
		var vmProps mo.VirtualMachine
		err = vm.Properties(ctx, vm.Reference(), []string{
			"config",
			"guest",
			"runtime",
			"network",
			"summary.config.annotation",
		}, &vmProps)
		if err != nil {
			return nil, fmt.Errorf("failed to get VM properties: %w", err)
		}
		if vmProps.Config == nil {
			// VM is not powered on or is in creating state
			fmt.Printf("VM properties not available for vm (%s), skipping this VM\n", vm.Name())
			continue
		}
		var datastores []string
		var networks []string
		var disks []string
		var clusterName string
		if vmProps.Config == nil {
			// VM is not powered on or is in creating state
			fmt.Printf("VM properties not available for vm (%s), skipping this VM", vm.Name())
			continue
		}
		// Fetch details required for RDM disks
		hostStorageMap := sync.Map{}
		controllers := make(map[int32]govmitypes.BaseVirtualSCSIController)
		// Collect all SCSI controller to find shared RDM disks
		for _, device := range vmProps.Config.Hardware.Device {
			if scsiController, ok := device.(govmitypes.BaseVirtualSCSIController); ok {
				controllers[device.GetVirtualDevice().Key] = scsiController
			}
		}
		// Get basic RDM disk info from VM properties
		rdmDiskInfos := make([]vjailbreakv1alpha1.RDMDiskInfo, 0)
		hostStorageInfo, err := getHostStorageDeviceInfo(ctx, vm, &hostStorageMap)
		if err != nil {
			ctxlog.Error(err, "failed to get disk info for vm skipping vm", "vm", vm.Name(), err)
			continue
		}
		attributes := strings.Split(vmProps.Summary.Config.Annotation, "\n")
		pc := property.DefaultCollector(c)
		for _, netRef := range vmProps.Network {
			var netObj mo.Network
			err := pc.RetrieveOne(ctx, netRef, []string{"name"}, &netObj)
			if err != nil {
				return nil, fmt.Errorf("failed to retrieve network name for %s: %w", netRef.Value, err)
			}
			networks = append(networks, netObj.Name)
		}
		var skipVM bool
		for _, device := range vmProps.Config.Hardware.Device {
			disk, ok := device.(*govmitypes.VirtualDisk)
			if !ok {
				continue
			}
			if controller, ok := controllers[disk.ControllerKey]; ok {
				if controller.GetVirtualSCSIController().SharedBus == govmitypes.VirtualSCSISharingPhysicalSharing {
					ctxlog.Info("VM has SCSI controller with shared bus, migration not supported",
						"vm", vm.Name())
					skipVM = true // Skip this VM and move to next one
					break
				}
			}
			var dsref govmitypes.ManagedObjectReference
			switch backing := disk.Backing.(type) {
			case *govmitypes.VirtualDiskFlatVer2BackingInfo:
				dsref = backing.Datastore.Reference()
			case *govmitypes.VirtualDiskSparseVer2BackingInfo:
				dsref = backing.Datastore.Reference()
			case *govmitypes.VirtualDiskRawDiskMappingVer1BackingInfo:
				dsref = backing.Datastore.Reference()
				if hostStorageInfo != nil {
					info := vjailbreakv1alpha1.RDMDiskInfo{
						DiskName: disk.DeviceInfo.GetDescription().Label,
						DiskSize: disk.CapacityInBytes,
					}
					for _, scsiDisk := range hostStorageInfo.ScsiLun {
						lunDetails := scsiDisk.GetScsiLun()
						if backing.LunUuid == lunDetails.Uuid {
							info.DisplayName = lunDetails.DisplayName
							info.UUID = lunDetails.Uuid
						}
					}
					rdmDiskInfos = append(rdmDiskInfos, info)
					continue
				}
			default:
				return nil, fmt.Errorf("unsupported disk backing type: %T", disk.Backing)
			}

			var ds mo.Datastore
			err := pc.RetrieveOne(ctx, dsref, []string{"name"}, &ds)
			if err != nil {
				return nil, fmt.Errorf("failed to get datastore: %w", err)
			}

			datastores = AppendUnique(datastores, ds.Name)
			disks = append(disks, disk.DeviceInfo.GetDescription().Label)
		}

		// Get the host name and parent (cluster) information
		host := mo.HostSystem{}
		err = property.DefaultCollector(c).RetrieveOne(ctx, *vmProps.Runtime.Host, []string{"name", "parent"}, &host)
		if err != nil {
			return nil, fmt.Errorf("failed to get host name: %w", err)
		}

		// Get the cluster name from the host's parent
		if host.Parent != nil {
			// Determine parent type based on the object reference type
			parentType := host.Parent.Type
			// Get the parent name
			var parentEntity mo.ManagedEntity
			err = property.DefaultCollector(c).RetrieveOne(ctx, *host.Parent, []string{"name"}, &parentEntity)
			if err != nil {
				fmt.Printf("failed to get parent info for host %s: %v\n", host.Name, err)
			} else {
				// Handle based on the parent's type
				switch parentType {
				case "ClusterComputeResource":
					var cluster mo.ClusterComputeResource
					err = property.DefaultCollector(c).RetrieveOne(ctx, *host.Parent, []string{"name"}, &cluster)
					if err != nil {
						fmt.Printf("failed to get cluster name for host %s: %v\n", host.Name, err)
					} else {
						clusterName = cluster.Name
					}
				case "ComputeResource":
					var compute mo.ComputeResource
					err = property.DefaultCollector(c).RetrieveOne(ctx, *host.Parent, []string{"name"}, &compute)
					if err != nil {
						fmt.Printf("failed to get compute resource name for host %s: %v\n", host.Name, err)
					} else {
						clusterName = compute.Name
					}
				default:
					fmt.Printf("unknown parent type for host %s: %s\n", host.Name, parentType)
				}
			}
		} else {
			clusterName = ""
		}
		if skipVM {
			continue
		}
		if len(rdmDiskInfos) >= 1 && len(disks) == 0 {
			ctxlog.Info("VM has multiple RDM disks but no regular bootable disks found", "vm", vm.Name(), "hence VM cannot be migrated")
			continue
		}
		var rdmDisks []vjailbreakv1alpha1.RDMDiskInfo
		if len(rdmDiskInfos) > 0 {
			rdmDisks, err = populateRDMDiskInfoFromAttributes(ctx, rdmDiskInfos, attributes)
			if err != nil {
				ctxlog.Error(err, "failed to populate RDM disk info from attributes for vm", "vm", vm.Name)
				continue
			}
		}
		vminfo = append(vminfo, vjailbreakv1alpha1.VMInfo{
			Name:        vmProps.Config.Name,
			Datastores:  datastores,
			Disks:       disks,
			Networks:    networks,
			IPAddress:   vmProps.Guest.IpAddress,
			VMState:     vmProps.Guest.GuestState,
			OSFamily:    vmProps.Guest.GuestFamily,
			CPU:         int(vmProps.Config.Hardware.NumCPU),
			Memory:      int(vmProps.Config.Hardware.MemoryMB),
			ESXiName:    host.Name,
			ClusterName: clusterName,
			RDMDisks:    rdmDisks,
		})
	}
	return vminfo, nil
}

// AppendUnique appends unique values to a slice
func AppendUnique(slice []string, values ...string) []string {
	for _, value := range values {
		if !slices.Contains(slice, value) {
			slice = append(slice, value)
		}
	}
	return slice
}

// CreateOrUpdateVMwareMachines creates or updates VMwareMachine objects for the given VMs
func CreateOrUpdateVMwareMachines(ctx context.Context, client client.Client,
	vmwcreds *vjailbreakv1alpha1.VMwareCreds, vminfo []vjailbreakv1alpha1.VMInfo) error {
	var wg sync.WaitGroup
	for i := range vminfo {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Don't panic on error
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Panic: %v\n", r)
				}
			}()
			vm := &vminfo[i] // Use a pointer
			err := CreateOrUpdateVMwareMachine(ctx, client, vmwcreds, vm)
			if err != nil {
				fmt.Printf("Error creating or updating VM '%s': %v\n", vm.Name, err)
			}
		}(i)
	}
	// Wait for all vms to be created or updated
	wg.Wait()
	return nil
}

// CreateOrUpdateVMwareMachine creates or updates a VMwareMachine object for the given VM
func CreateOrUpdateVMwareMachine(ctx context.Context, client client.Client,
	vmwcreds *vjailbreakv1alpha1.VMwareCreds, vminfo *vjailbreakv1alpha1.VMInfo) error {
	sanitizedVMName, err := ConvertToK8sName(vminfo.Name)
	if err != nil {
		return fmt.Errorf("failed to convert VM name: %w", err)
	}
	// We need this flag because, there can be multiple VMwarecreds and each will
	// trigger its own reconciliation loop,
	// so we need to know if the object is new or not. if it is new we mark the migrated
	// field to false and powerstate to the current state of the vm.
	// If the object is not new, we update the status and persist the migrated status.
	init := false

	vmwvm := &vjailbreakv1alpha1.VMwareMachine{}
	vmwvmKey := k8stypes.NamespacedName{Name: sanitizedVMName, Namespace: vmwcreds.Namespace}

	// Try to fetch existing resource
	err = client.Get(ctx, vmwvmKey, vmwvm)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get VMwareMachine: %w", err)
	}

	// Check if the object is present or not if not present create a new object and set init to true.
	if apierrors.IsNotFound(err) {
		// If not found, create a new object
		vmwvm = &vjailbreakv1alpha1.VMwareMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmwvmKey.Name,
				Namespace: vmwcreds.Namespace,
				Labels: map[string]string{
					constants.VMwareCredsLabel: vmwcreds.Name,
					constants.ESXiNameLabel:    vminfo.ESXiName,
					constants.ClusterNameLabel: vminfo.ClusterName,
				},
			},
			Spec: vjailbreakv1alpha1.VMwareMachineSpec{
				VMInfo: *vminfo,
			},
		}
		init = true
	} else {
		// Initialize labels map if needed
		label := fmt.Sprintf("%s-%s", constants.VMwareCredsLabel, vmwcreds.Name)
		currentOSFamily := vmwvm.Spec.VMInfo.OSFamily
		// Check if label already exists with same value
		if vmwvm.Labels == nil || vmwvm.Labels[label] != "true" {
			// Initialize labels map if needed
			if vmwvm.Labels == nil {
				vmwvm.Labels = make(map[string]string)
			}
			vmwvm.Labels[label] = "true"
			// Update only if we made changes
			if err = client.Update(ctx, vmwvm); err != nil {
				return fmt.Errorf("failed to update VMwareMachine label: %w", err)
			}
		}
		// Set the new label
		vmwvm.Labels[constants.VMwareCredsLabel] = vmwcreds.Name

		if !reflect.DeepEqual(vmwvm.Spec.VMInfo, *vminfo) || !reflect.DeepEqual(vmwvm.Labels[constants.ESXiNameLabel], vminfo.ESXiName) || !reflect.DeepEqual(vmwvm.Labels[constants.ClusterNameLabel], vminfo.ClusterName) {
			syncRDMDisks(vminfo, vmwvm)
			// update vminfo in case the VM has been moved by vMotion
			assignedIP := ""
			osType := ""

			if vmwvm.Spec.VMInfo.AssignedIP != "" {
				assignedIP = vmwvm.Spec.VMInfo.AssignedIP
			}
			if vmwvm.Spec.VMInfo.OSFamily != "" {
				osType = vmwvm.Spec.VMInfo.OSFamily
			}
			vmwvm.Spec.VMInfo = *vminfo
			if assignedIP != "" {
				vmwvm.Spec.VMInfo.AssignedIP = assignedIP
			}
			if osType != "" && vmwvm.Spec.VMInfo.OSFamily == "" {
				vmwvm.Spec.VMInfo.OSFamily = osType
			}
			vmwvm.Labels[constants.ESXiNameLabel] = vminfo.ESXiName
			vmwvm.Labels[constants.ClusterNameLabel] = vminfo.ClusterName

			if vmwvm.Spec.VMInfo.OSFamily == "" {
				vmwvm.Spec.VMInfo.OSFamily = currentOSFamily
			}
			// Update only if we made changes
			if err = client.Update(ctx, vmwvm); err != nil {
				return fmt.Errorf("failed to update VMwareMachine: %w", err)
			}
		}
	}
	_, err = controllerutil.CreateOrUpdate(ctx, client, vmwvm, func() error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update VMwareMachine: %w", err)
	}

	// Assumption is if init is true, the object is new and it is not migrated hence mark migrated to false.
	if init {
		vmwvm.Status = vjailbreakv1alpha1.VMwareMachineStatus{
			PowerState: vminfo.VMState,
			Migrated:   false,
		}
	} else {
		// If the object is not new, update the status and persist migrated status.
		currentMigratedStatus := vmwvm.Status.Migrated
		if vmwvm.Status.PowerState != vminfo.VMState {
			vmwvm.Status.PowerState = vminfo.VMState
		}
		vmwvm.Status.Migrated = currentMigratedStatus
	}

	// Update the status
	if err := client.Status().Update(ctx, vmwvm); err != nil {
		return fmt.Errorf("failed to update VMwareMachine status: %w", err)
	}
	return nil
}

// GetClosestFlavour gets the closest flavor for the given CPU and memory
func GetClosestFlavour(_ context.Context, cpu, memory int, computeClient *gophercloud.ServiceClient) (*flavors.Flavor, error) {
	allPages, err := flavors.ListDetail(computeClient, nil).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list flavors: %w", err)
	}

	allFlavors, err := flavors.ExtractFlavors(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract all flavors: %w", err)
	}

	bestFlavor := new(flavors.Flavor)
	bestFlavor.VCPUs = constants.MaxVCPUs
	bestFlavor.RAM = constants.MaxRAM

	// Find the smallest flavor that meets the requirements
	for _, flavor := range allFlavors {
		if flavor.VCPUs >= cpu && flavor.RAM >= memory {
			if flavor.VCPUs < bestFlavor.VCPUs ||
				(flavor.VCPUs == bestFlavor.VCPUs && flavor.RAM < bestFlavor.RAM) {
				bestFlavor = &flavor
			}
		}
	}

	if bestFlavor.VCPUs != constants.MaxVCPUs {
		return bestFlavor, nil
	}
	return nil, fmt.Errorf("no suitable flavor found for %d vCPUs and %d MB RAM", cpu, memory)
}

// CreateOrUpdateLabel creates or updates a label on a VMwareMachine resource
func CreateOrUpdateLabel(ctx context.Context, client client.Client,
	vmwvm *vjailbreakv1alpha1.VMwareMachine, key, value string) error {
	_, err := controllerutil.CreateOrUpdate(ctx, client, vmwvm, func() error {
		if vmwvm.Labels == nil {
			vmwvm.Labels = make(map[string]string)
		}
		if vmwvm.Labels[key] == value {
			return nil
		}
		vmwvm.Labels[key] = value
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update VMwareMachine labels: %w", err)
	}
	return nil
}

// FilterVMwareMachinesForCreds returns all VMwareMachine objects associated with a VMwareCreds resource
func FilterVMwareMachinesForCreds(ctx context.Context, k8sClient client.Client,
	vmwcreds *vjailbreakv1alpha1.VMwareCreds) (*vjailbreakv1alpha1.VMwareMachineList, error) {
	vmList := vjailbreakv1alpha1.VMwareMachineList{}
	if err := k8sClient.List(ctx, &vmList, client.InNamespace(constants.NamespaceMigrationSystem), client.MatchingLabels{constants.VMwareCredsLabel: vmwcreds.Name}); err != nil {
		return nil, errors.Wrap(err, "Error listing VMs")
	}
	return &vmList, nil
}

// FilterVMwareHostsForCreds filters VMwareHost objects for the given credentials
func FilterVMwareHostsForCreds(ctx context.Context, k8sClient client.Client, vmwcreds *vjailbreakv1alpha1.VMwareCreds) (*vjailbreakv1alpha1.VMwareHostList, error) {
	hostList := vjailbreakv1alpha1.VMwareHostList{}
	if err := k8sClient.List(ctx, &hostList, client.InNamespace(constants.NamespaceMigrationSystem), client.MatchingLabels{constants.VMwareCredsLabel: vmwcreds.Name}); err != nil {
		return nil, errors.Wrap(err, "Error listing VMs")
	}
	return &hostList, nil
}

// FilterVMwareClustersForCreds filters VMwareCluster objects for the given credentials
func FilterVMwareClustersForCreds(ctx context.Context, k8sClient client.Client, vmwcreds *vjailbreakv1alpha1.VMwareCreds) (*vjailbreakv1alpha1.VMwareClusterList, error) {
	clusterList := vjailbreakv1alpha1.VMwareClusterList{}
	if err := k8sClient.List(ctx, &clusterList, client.InNamespace(constants.NamespaceMigrationSystem), client.MatchingLabels{constants.VMwareCredsLabel: vmwcreds.Name}); err != nil {
		return nil, errors.Wrap(err, "Error listing VMs")
	}
	return &clusterList, nil
}

// FindVMwareMachinesNotInVcenter finds VMwareMachine objects that are not present in the vCenter
func FindVMwareMachinesNotInVcenter(ctx context.Context, client client.Client, vmwcreds *vjailbreakv1alpha1.VMwareCreds, vcenterVMs []vjailbreakv1alpha1.VMInfo) ([]vjailbreakv1alpha1.VMwareMachine, error) {
	vmList, err := FilterVMwareMachinesForCreds(ctx, client, vmwcreds)
	if err != nil {
		return nil, errors.Wrap(err, "Error filtering VMs")
	}
	var staleVMs []vjailbreakv1alpha1.VMwareMachine
	for _, vm := range vmList.Items {
		if !VMExistsInVcenter(vm.Spec.VMInfo.Name, vcenterVMs) {
			staleVMs = append(staleVMs, vm)
		}
	}
	return staleVMs, nil
}

// FindVMwareHostsNotInVcenter finds VMwareHost objects that are not present in the vCenter
func FindVMwareHostsNotInVcenter(ctx context.Context, client client.Client, vmwcreds *vjailbreakv1alpha1.VMwareCreds, clusterInfo []VMwareClusterInfo) ([]vjailbreakv1alpha1.VMwareHost, error) {
	hostList, err := FilterVMwareHostsForCreds(ctx, client, vmwcreds)
	if err != nil {
		return nil, errors.Wrap(err, "Error filtering VMs")
	}
	var staleHosts []vjailbreakv1alpha1.VMwareHost
	for _, host := range hostList.Items {
		if !HostExistsInVcenter(host.Name, clusterInfo) {
			staleHosts = append(staleHosts, host)
		}
	}
	return staleHosts, nil
}

// DeleteStaleVMwareMachines deletes VMwareMachine objects that are not present in the vCenter
func DeleteStaleVMwareMachines(ctx context.Context, client client.Client, vmwcreds *vjailbreakv1alpha1.VMwareCreds, vcenterVMs []vjailbreakv1alpha1.VMInfo) error {
	staleVMs, err := FindVMwareMachinesNotInVcenter(ctx, client, vmwcreds, vcenterVMs)
	if err != nil {
		return errors.Wrap(err, "Error finding stale VMs")
	}
	for _, vm := range staleVMs {
		if err := client.Delete(ctx, &vm); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrap(err, fmt.Sprintf("Error deleting stale VM '%s'", vm.Name))
			}
		}
	}
	return nil
}

// VMExistsInVcenter checks if a VM exists in the vCenter
func VMExistsInVcenter(vmName string, vcenterVMs []vjailbreakv1alpha1.VMInfo) bool {
	for _, vm := range vcenterVMs {
		if vm.Name == vmName {
			return true
		}
	}
	return false
}

// HostExistsInVcenter checks if a host exists in the vCenter
func HostExistsInVcenter(hostName string, clusterInfo []VMwareClusterInfo) bool {
	for _, cluster := range clusterInfo {
		for _, host := range cluster.Hosts {
			if host.Name == hostName {
				return true
			}
		}
	}
	return false
}

// DeleteDependantObjectsForVMwareCreds removes all objects dependent on a VMwareCreds resource
func DeleteDependantObjectsForVMwareCreds(ctx context.Context, scope *scope.VMwareCredsScope) error {
	log := scope.Logger
	log.Info("Deleting dependant objects for VMwareCreds", "vmwarecreds", scope.Name())
	if err := DeleteVMwareMachinesForVMwareCreds(ctx, scope); err != nil {
		return errors.Wrap(err, "Error deleting VMs")
	}
	if err := DeleteVMwareHostsForVMwareCreds(ctx, scope); err != nil {
		return errors.Wrap(err, "Error deleting hosts")
	}
	if err := DeleteVMwareClustersForVMwareCreds(ctx, scope); err != nil {
		return errors.Wrap(err, "Error deleting clusters")
	}

	if err := DeleteVMwarecredsSecret(ctx, scope); err != nil {
		return errors.Wrap(err, "Error deleting secret")
	}

	return nil
}

// DeleteVMwarecredsSecret removes the secret associated with a VMwareCreds resource
func DeleteVMwarecredsSecret(ctx context.Context, scope *scope.VMwareCredsScope) error {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scope.VMwareCreds.Spec.SecretRef.Name,
			Namespace: constants.NamespaceMigrationSystem,
		},
	}
	if err := scope.Client.Delete(ctx, &secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete associated secret")
		}
	}
	return nil
}

// DeleteVMwareMachinesForVMwareCreds removes all VMwareMachine objects associated with a VMwareCreds resource
func DeleteVMwareMachinesForVMwareCreds(ctx context.Context, scope *scope.VMwareCredsScope) error {
	vmList, err := FilterVMwareMachinesForCreds(ctx, scope.Client, scope.VMwareCreds)
	if err != nil {
		return errors.Wrap(err, "Error filtering VMs")
	}
	for _, vm := range vmList.Items {
		if err := scope.Client.Delete(ctx, &vm); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrap(err, fmt.Sprintf("error deleting VM '%s'", vm.Name))
			}
		}
	}
	return nil
}

// DeleteVMwareClustersForVMwareCreds removes all VMwareCluster objects associated with a VMwareCreds resource
func DeleteVMwareClustersForVMwareCreds(ctx context.Context, scope *scope.VMwareCredsScope) error {
	clusterList, err := FilterVMwareClustersForCreds(ctx, scope.Client, scope.VMwareCreds)
	if err != nil {
		return errors.Wrap(err, "Error filtering VMs")
	}
	for _, cluster := range clusterList.Items {
		if err := scope.Client.Delete(ctx, &cluster); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrap(err, fmt.Sprintf("error deleting VM '%s'", cluster.Name))
			}
		}
	}
	return nil
}

// DeleteVMwareHostsForVMwareCreds removes all VMwareHost objects associated with a VMwareCreds resource
func DeleteVMwareHostsForVMwareCreds(ctx context.Context, scope *scope.VMwareCredsScope) error {
	hostList, err := FilterVMwareHostsForCreds(ctx, scope.Client, scope.VMwareCreds)
	if err != nil {
		return errors.Wrap(err, "Error filtering VMs")
	}
	for _, host := range hostList.Items {
		if err := scope.Client.Delete(ctx, &host); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrap(err, fmt.Sprintf("error deleting VM '%s'", host.Name))
			}
		}
	}
	return nil
}

// containsString checks if a string exists in a slice
func containsString(slice []string, target string) bool {
	for _, item := range slice {
		if item == target {
			return true
		}
	}
	return false
}

// syncRDMDisks handles synchronization of RDM disk information between VMInfo and VMwareMachine
func syncRDMDisks(vminfo *vjailbreakv1alpha1.VMInfo, vmwvm *vjailbreakv1alpha1.VMwareMachine) {
	// Both have RDM disks - preserve OpenStack related information
	if vminfo.RDMDisks != nil && vmwvm.Spec.VMInfo.RDMDisks != nil {
		// Create a map of existing VMware Machine RDM disks by disk name
		existingDisks := make(map[string]vjailbreakv1alpha1.RDMDiskInfo)
		for _, disk := range vmwvm.Spec.VMInfo.RDMDisks {
			existingDisks[disk.DiskName] = disk
		}

		// Update VMInfo RDM disks while preserving OpenStack information
		for i, disk := range vminfo.RDMDisks {
			if existingDisk, ok := existingDisks[disk.DiskName]; ok {
				// Preserve OpenStack volume reference if new one is nil
				if reflect.DeepEqual(vminfo.RDMDisks[i].OpenstackVolumeRef, v1alpha1.OpenStackVolumeRefInfo{}) &&
					!reflect.DeepEqual(existingDisk.OpenstackVolumeRef, v1alpha1.OpenStackVolumeRefInfo{}) {
					vminfo.RDMDisks[i].OpenstackVolumeRef = existingDisk.OpenstackVolumeRef
				} else {
					// Preserve CinderBackendPool if new one is nil
					if vminfo.RDMDisks[i].OpenstackVolumeRef.CinderBackendPool == "" &&
						existingDisk.OpenstackVolumeRef.CinderBackendPool != "" {
						vminfo.RDMDisks[i].OpenstackVolumeRef.CinderBackendPool = existingDisk.OpenstackVolumeRef.CinderBackendPool
					}

					// Preserve VolumeType if new one is nil
					if vminfo.RDMDisks[i].OpenstackVolumeRef.VolumeType == "" &&
						existingDisk.OpenstackVolumeRef.VolumeType != "" {
						vminfo.RDMDisks[i].OpenstackVolumeRef.VolumeType = existingDisk.OpenstackVolumeRef.VolumeType
					}
				}
			}
		}
	}
}

// getHostStorageDeviceInfo retrieves the storage device information for the host of a given VM
func getHostStorageDeviceInfo(ctx context.Context, vm *object.VirtualMachine, hostStorageMap *sync.Map) (*govmitypes.HostStorageDeviceInfo, error) {
	hostSystem, err := vm.HostSystem(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host system: %v", err)
	}
	var hostStorageDevice *govmitypes.HostStorageDeviceInfo
	hostStorageDevicefromMap, ok := hostStorageMap.Load(hostSystem.String())
	if ok {
		hs, ok = hsfromMap.(mo.HostSystem)
		if !ok {
			return nil, fmt.Errorf("invalid type assertion for host system from map")
		}
	} else {
		var hs mo.HostSystem
		err = hostSystem.Properties(ctx, hostSystem.Reference(), []string{"config.storageDevice"}, &hs)
		if err != nil || (hs.Config == nil && hs.Config.StorageDevice == nil) {
			return nil, fmt.Errorf("failed to get host system properties: %v", err)
		}
		hostStorageMap.Store(hostSystem.String(), hs.Config.StorageDevice)
		hostStorageDevice = hs.Config.StorageDevice
	}
	return hostStorageDevice, nil
}

// populateRDMDiskInfoFromAttributes processes VM annotations and custom attributes to populate RDM disk information
// RDM disk attributes in Vmware for migration - VJB_RDM:diskName:volumeRef:value
// eg:
//
//	VJB_RDM:Hard Disk:volumeRef:"source-id"="abac111"
func populateRDMDiskInfoFromAttributes(ctx context.Context, baseRDMDisks []vjailbreakv1alpha1.RDMDiskInfo, attributes []string) ([]vjailbreakv1alpha1.RDMDiskInfo, error) {
	rdmMap := make(map[string]*vjailbreakv1alpha1.RDMDiskInfo)
	log := ctrllog.FromContext(ctx)

	// Create copies of base RDM disks to preserve existing data
	for i := range baseRDMDisks {
		diskCopy := baseRDMDisks[i] // Make a copy
		rdmMap[strings.TrimSpace(diskCopy.DiskName)] = diskCopy
	}
	// Process attributes for additional RDM information
	for _, attr := range attributes {
		if strings.HasPrefix(attr, "VJB_RDM:") {
			parts := strings.Split(attr, ":")
			if len(parts) != 4 {
				continue
			}

			diskName := strings.TrimSpace(parts[1])
			key := parts[2]
			value := parts[3]

			// Get or create RDMDiskInfo
			rdmInfo, exists := rdmMap[diskName]
			if exists {
				// Update fields only if new value is provided
				if strings.TrimSpace(key) == "volumeRef" && value != "" {
					splotVolRef := strings.Split(value, "=")
					if len(splotVolRef) != 2 {
						return nil, fmt.Errorf("invalid volume reference format: %s", rdmInfo.OpenstackVolumeRef.VolumeRef)
					}
					mp := make(map[string]string)
					mp[splotVolRef[0]] = splotVolRef[1]
					fmt.Println("Setting OpenStack Volume Ref for RDM disk:", diskName, "to", mp, rdmInfo)
					rdmInfo.OpenstackVolumeRef = vjailbreakv1alpha1.OpenStackVolumeRefInfo{
						VolumeRef: mp,
					}
					rdmMap[diskName] = rdmInfo
				}
			} else {
				log.Info("RDM attributes exist on VM but disk not found in  RDM disks")
			}
		}
	}
	// Convert map back to slice while preserving all data
	rdmDisks := make([]vjailbreakv1alpha1.RDMDiskInfo, 0, len(rdmMap))
	for _, rdmInfo := range rdmMap {
		rdmDisks = append(rdmDisks, rdmInfo)
	}
	return rdmDisks, nil
}
