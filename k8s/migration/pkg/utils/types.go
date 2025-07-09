package utils

import gophercloud "github.com/gophercloud/gophercloud"

// CloudInitParams holds OpenStack authentication parameters for cloud-init configuration.
// These parameters are used when generating cloud-init configurations for bare metal nodes.
type CloudInitParams struct {
	AuthURL     string
	Username    string
	Password    string
	RegionName  string
	TenantName  string
	Insecure    bool
	DomainName  string
	FQDN        string
	KeystoneURL string
}

// OpenStackClients holds clients for interacting with OpenStack services
type OpenStackClients struct {
	// BlockStorageClient is the client for interacting with OpenStack Block Storage
	BlockStorageClient *gophercloud.ServiceClient
	// ComputeClient is the client for interacting with OpenStack Compute
	ComputeClient *gophercloud.ServiceClient
	// NetworkingClient is the client for interacting with OpenStack Networking
	NetworkingClient *gophercloud.ServiceClient
}

// Network represents network configuration for OpenStack VMs
type Network struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Link      string `json:"link"`
	NetworkID string `json:"network_id"`
}

// OpenStackMetadata represents metadata for OpenStack VMs
type OpenStackMetadata struct {
	Networks []Network `json:"networks"`
}

// VMwareHostInfo represents a host in a VMware cluster.
// It contains essential information about a VMware ESXi host.
type VMwareHostInfo struct {
	// Name is the fully qualified domain name or IP address of the host
	Name string
	// HardwareUUID is the unique identifier of the host
	HardwareUUID string
}

// VMwareClusterInfo represents a cluster in a VMware environment.
// It contains information about a VMware cluster and its associated hosts.
type VMwareClusterInfo struct {
	// Name is the unique identifier of the cluster
	Name string
	// Hosts is a list of ESXi hosts that are part of this cluster
	Hosts []VMwareHostInfo
}

// RollingMigartionValidationConfig defines the validation configuration for rolling migration
type RollingMigartionValidationConfig struct {
	// CheckDRSEnabled checks if DRS is enabled
	CheckDRSEnabled bool `json:"checkDRSEnabled"` // Check if DRS is enabled
	// CheckDRSIsFullyAutomated checks if DRS is in fully automated mode
	CheckDRSIsFullyAutomated bool `json:"checkDRSIsFullyAutomated"` // Check if DRS is in fully automated mode
	// CheckIfThereAreMoreThanOneHostInCluster checks if there are more than one host in the cluster
	CheckIfThereAreMoreThanOneHostInCluster bool `json:"checkIfThereAreMoreThanOneHostInCluster"` // Check if there are more than one host in the cluster
	// CheckClusterRemainingHostCapacity checks if the cluster has enough remaining capacity
	CheckClusterRemainingHostCapacity bool `json:"checkClusterRemainingHostCapacity"` // Check if the cluster has enough remaining capacity
	// CheckVMsAreNotBlockedForMigration checks if the VMs are not blocked for migration
	CheckVMsAreNotBlockedForMigration bool `json:"checkVMsAreNotBlockedForMigration"` // Check if the VMs are not blocked for migration
	// CheckESXiInMAAS checks if the ESXi host is in MAAS
	CheckESXiInMAAS bool `json:"checkESXiInMAAS"` // Check if the ESXi host is in MAAS
	// CheckPCDHasClusterConfigured checks if the PCD has at-least one Cluster configured
	CheckPCDHasClusterConfigured bool `json:"checkPCDHasClusterConfigured"` // Check if the PCD has at-least one Cluster configured
}
