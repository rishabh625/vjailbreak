// Copyright © 2024 The vjailbreak authors

package openstack

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/utils/migrateutils"
	"github.com/platform9/vjailbreak/v2v-helper/vm"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
)

//go:generate mockgen -source=../openstack/openstackops.go -destination=../openstack/openstackops_mock.go -package=openstack

type OpenstackOperations interface {
	CreateVolume(name string, size int64, ostype string, uefi bool, volumetype string) (*volumes.Volume, error)
	WaitForVolume(volumeID string) error
	AttachVolumeToVM(volumeID string) error
	WaitForVolumeAttachment(volumeID string) error
	DetachVolumeFromVM(volumeID string) error
	SetVolumeUEFI(volume *volumes.Volume) error
	EnableQGA(volume *volumes.Volume) error
	SetVolumeImageMetadata(volume *volumes.Volume) error
	SetVolumeBootable(volume *volumes.Volume) error
	GetClosestFlavour(cpu int32, memory int32) (*flavors.Flavor, error)
	GetFlavor(flavorId string) (*flavors.Flavor, error)
	GetNetwork(networkname string) (*networks.Network, error)
	GetPort(portID string) (*ports.Port, error)
	CreatePort(networkid *networks.Network, mac, ip, vmname string) (*ports.Port, error)
	CreateVM(flavor *flavors.Flavor, networkIDs, portIDs []string, vminfo vm.VMInfo, availabilityZone string) (*servers.Server, error)
	DeleteVolume(volumeID string) error
	FindDevice(volumeID string) (string, error)
	WaitUntilVMActive(vmID string) (bool, error)
	CinderManage(rdmDisk vm.RDMDisk) (*volumes.Volume, error)
}

func getCert(endpoint string) (*x509.Certificate, error) {
	conf := &tls.Config{
		InsecureSkipVerify: true,
	}
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("error parsing URL: %w", err)
	}
	hostname := parsedURL.Hostname()
	conn, err := tls.Dial("tcp", hostname+":443", conf)
	if err != nil {
		return nil, fmt.Errorf("error connecting to %s: %w", hostname, err)
	}
	defer conn.Close()
	cert := conn.ConnectionState().PeerCertificates[0]
	return cert, nil
}

func validateOpenStack(insecure bool) (*migrateutils.OpenStackClients, error) {
	opts, err := openstack.AuthOptionsFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to get OpenStack auth options: %s", err)
	}
	opts.AllowReauth = true
	providerClient, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider client: %s", err)
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if insecure {
		tlsConfig.InsecureSkipVerify = true
	} else {
		// Get the certificate for the Openstack endpoint
		caCert, err := getCert(opts.IdentityEndpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to get certificate: %s", err)
		}
		caCertPool, _ := x509.SystemCertPool()
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

	// Connection Retry Block
	for i := 0; i < constants.MaxIntervalCount; i++ {
		err = openstack.Authenticate(providerClient, opts)
		if err == nil {
			break
		}
		time.Sleep(5 * time.Second) // Wait for 5 seconds before checking again
	}
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate OpenStack client: %s", err)
	}

	endpoint := gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	}

	blockStorageClient, err := openstack.NewBlockStorageV3(providerClient, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create block storage client: %s", err)
	}

	computeClient, err := openstack.NewComputeV2(providerClient, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %s", err)
	}

	networkingClient, err := openstack.NewNetworkV2(providerClient, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create networking client: %s", err)
	}

	return &migrateutils.OpenStackClients{
		BlockStorageClient: blockStorageClient,
		ComputeClient:      computeClient,
		NetworkingClient:   networkingClient,
	}, nil
}

func NewOpenStackClients(insecure bool) (*migrateutils.OpenStackClients, error) {
	ostackclients, err := validateOpenStack(insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to validate OpenStack connection: %s", err)
	}
	return ostackclients, nil
}
