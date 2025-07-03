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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  This is scaffolding for you to own.
// NOTE: json tags are required.  Any new fields you add must have json:"-" or json:"fieldName" tags for the fields to be serialized.

// RDMDiskInfo represents information about a Raw Device Mapping disk
type RDMDiskInfo struct {
	// DeviceName is the name of the RDM device
	// +kubebuilder:validation:Required
	DeviceName string `json:"deviceName"`

	// LunUUID is the unique identifier for the LUN
	// +kubebuilder:validation:Required
	LunUUID string `json:"lunUUID"`

	// CapacityInKB is the capacity of the RDM disk in kilobytes
	// +kubebuilder:validation:Minimum=0
	CapacityInKB int64 `json:"capacityInKB"`

	// CompatibilityMode indicates the RDM compatibility mode (physical or virtual)
	// +kubebuilder:validation:Enum=physical;virtual
	CompatibilityMode string `json:"compatibilityMode,omitempty"`

	// IsShared indicates if this is a shared RDM disk that can be attached to multiple VMs
	// +kubebuilder:default=false
	IsShared bool `json:"isShared"`

	// SharedRDMRef is the reference to the SharedRDM CR name if this is a shared disk
	// +kubebuilder:validation:Optional
	SharedRDMRef string `json:"sharedRDMRef,omitempty"`

	// MigrationStatus indicates the current status of RDM migration
	// +kubebuilder:validation:Enum=Pending;InProgress;Completed;Failed
	// +kubebuilder:default=Pending
	MigrationStatus string `json:"migrationStatus"`

	// DataSource indicates the source of the RDM data for migration
	// +kubebuilder:validation:Enum=vmware-lun;existing-volume;manual
	// +kubebuilder:default=vmware-lun
	DataSource string `json:"dataSource"`
}

// OpenStackVolumeRefInfo represents reference information for an OpenStack volume
type OpenStackVolumeRefInfo struct {
	// VolumeID is the OpenStack volume ID
	VolumeID string `json:"volumeID,omitempty"`

	// VolumeName is the OpenStack volume name
	VolumeName string `json:"volumeName,omitempty"`

	// VolumeType is the OpenStack volume type
	VolumeType string `json:"volumeType,omitempty"`

	// BackendPool is the Cinder backend pool
	BackendPool string `json:"backendPool,omitempty"`
}

// VMInfo represents information about a VMware virtual machine
type VMInfo struct {
	// Name is the name of the VM
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// UUID is the unique identifier of the VM
	// +kubebuilder:validation:Required
	UUID string `json:"uuid"`

	// PowerState indicates the current power state of the VM
	PowerState string `json:"powerState,omitempty"`

	// GuestOS is the guest operating system
	GuestOS string `json:"guestOS,omitempty"`

	// CPUCount is the number of CPUs allocated to the VM
	// +kubebuilder:validation:Minimum=1
	CPUCount int32 `json:"cpuCount,omitempty"`

	// MemoryMB is the amount of memory in megabytes
	// +kubebuilder:validation:Minimum=0
	MemoryMB int64 `json:"memoryMB,omitempty"`

	// DiskInfo contains information about VM disks
	DiskInfo []DiskInfo `json:"diskInfo,omitempty"`

	// RDMDiskInfo contains information about RDM disks
	RDMDiskInfo []RDMDiskInfo `json:"rdmDiskInfo,omitempty"`

	// HasRDMDisks is a quick flag to indicate presence of RDM disks
	// +kubebuilder:default=false
	HasRDMDisks bool `json:"hasRDMDisks"`

	// RDMMigrationRequired indicates if RDM migration is needed for this VM
	// +kubebuilder:default=false
	RDMMigrationRequired bool `json:"rdmMigrationRequired"`

	// NetworkInfo contains information about VM networks
	NetworkInfo []NetworkInfo `json:"networkInfo,omitempty"`

	// Annotations contains additional metadata
	Annotations map[string]string `json:"annotations,omitempty"`
}

// DiskInfo represents information about a regular VM disk
type DiskInfo struct {
	// DeviceName is the name of the disk device
	DeviceName string `json:"deviceName"`

	// CapacityInKB is the capacity of the disk in kilobytes
	// +kubebuilder:validation:Minimum=0
	CapacityInKB int64 `json:"capacityInKB"`

	// DiskMode indicates the disk mode (persistent, independent, etc.)
	DiskMode string `json:"diskMode,omitempty"`

	// ThinProvisioned indicates if the disk is thin provisioned
	ThinProvisioned bool `json:"thinProvisioned,omitempty"`
}

// NetworkInfo represents information about VM network interfaces
type NetworkInfo struct {
	// NetworkName is the name of the network
	NetworkName string `json:"networkName"`

	// MACAddress is the MAC address of the network interface
	MACAddress string `json:"macAddress,omitempty"`

	// IPAddress is the IP address assigned to the interface
	IPAddress string `json:"ipAddress,omitempty"`

	// Connected indicates if the network interface is connected
	Connected bool `json:"connected,omitempty"`
}

// VMwareMachineSpec defines the desired state of VMwareMachine
type VMwareMachineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// VMInfo contains the VMware virtual machine information
	// +kubebuilder:validation:Required
	VMInfo VMInfo `json:"vmInfo"`

	// CredentialsRef is a reference to the secret containing VMware credentials
	// +kubebuilder:validation:Required
	CredentialsRef string `json:"credentialsRef"`

	// VCenterURL is the URL of the vCenter server
	// +kubebuilder:validation:Required
	VCenterURL string `json:"vcenterURL"`

	// Datacenter is the VMware datacenter name
	// +kubebuilder:validation:Required
	Datacenter string `json:"datacenter"`

	// Cluster is the VMware cluster name
	Cluster string `json:"cluster,omitempty"`

	// ResourcePool is the VMware resource pool name
	ResourcePool string `json:"resourcePool,omitempty"`

	// Folder is the VMware folder path
	Folder string `json:"folder,omitempty"`
}

// VMwareMachineStatus defines the observed state of VMwareMachine
type VMwareMachineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase represents the current phase of the VMware machine
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the VMware machine's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncTime is the last time the VM information was synchronized
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ErrorMessage contains any error message from the last operation
	ErrorMessage string `json:"errorMessage,omitempty"`

	// RDMSyncStatus indicates the status of RDM disk synchronization
	RDMSyncStatus string `json:"rdmSyncStatus,omitempty"`

	// SharedRDMRefs contains references to SharedRDM resources used by this VM
	SharedRDMRefs []string `json:"sharedRDMRefs,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:printcolumn:name="VM Name",type=string,JSONPath=`.spec.vmInfo.name`
//+kubebuilder:printcolumn:name="UUID",type=string,JSONPath=`.spec.vmInfo.uuid`
//+kubebuilder:printcolumn:name="Power State",type=string,JSONPath=`.spec.vmInfo.powerState`
//+kubebuilder:printcolumn:name="Has RDM",type=boolean,JSONPath=`.spec.vmInfo.hasRDMDisks`
//+kubebuilder:printcolumn:name="RDM Migration Required",type=boolean,JSONPath=`.spec.vmInfo.rdmMigrationRequired`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VMwareMachine is the Schema for the vmwaremachines API
type VMwareMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VMwareMachineSpec   `json:"spec,omitempty"`
	Status VMwareMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VMwareMachineList contains a list of VMwareMachine
type VMwareMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VMwareMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VMwareMachine{}, &VMwareMachineList{})
}
