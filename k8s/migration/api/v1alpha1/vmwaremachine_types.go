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

type OpenStackVolumeRefInfo struct {
	// VolumeRef is the reference to the OpenStack volume
	VolumeRef string `json:"volumeRef,omitempty"`
	// CinderBackendPool is the cinder backend pool of the disk
	CinderBackendPool string `json:"cinderBackendPool,omitempty"`
	// VolumeType is the volume type of the disk
	VolumeType string `json:"volumeType,omitempty"`
}

type RDMDiskInfo struct {
	// DiskName is the name of the disk
	DiskName string `json:"diskName,omitempty"`
	// DiskSize is the size of the disk in GB
	DiskSize int64 `json:"diskSize,omitempty"`
	// UUID is the unique identifier of the disk
	UUID string `json:"uuid,omitempty"`
	// DisplayName is the display name of the disk
	DisplayName string `json:"displayName,omitempty"`
	// OpenstackVolumeRef contains OpenStack volume reference information
	OpenstackVolumeRef *OpenStackVolumeRefInfo `json:"openstackVolumeRef,omitempty"`
}

type VMInfo struct {
	Name       string        `json:"name"`
	Datastores []string      `json:"datastores,omitempty"`
	Disks      []string      `json:"disks,omitempty"`
	Networks   []string      `json:"networks,omitempty"`
	IPAddress  string        `json:"ipAddress,omitempty"`
	VMState    string        `json:"vmState,omitempty"`
	OSType     string        `json:"osType,omitempty"`
	CPU        int           `json:"cpu,omitempty"`
	Memory     int           `json:"memory,omitempty"`
	RDMDisks   []RDMDiskInfo `json:"rdmDisks,omitempty"`
}

// VMwareMachineSpec defines the desired state of VMwareMachine
type VMwareMachineSpec struct {
	// VMInfo is the info of the VMs in the VMwareMachine
	VMs VMInfo `json:"vms,omitempty"`

	// TargetFlavorId is the flavor to be used to create the target VM on openstack
	TargetFlavorID string `json:"targetFlavorId,omitempty"`
}

// VMwareMachineStatus defines the observed state of VMwareMachine
type VMwareMachineStatus struct {
	// PowerState is the state of the VMs in the VMware
	PowerState string `json:"powerState,omitempty"`

	// Migrated flag to indicate if the VMs have been migrated
	// +kubebuilder:default=false
	// +kubebuilder:validation:Required
	Migrated bool `json:"migrated,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// VMwareMachine is the Schema for the vmwaremachines API
type VMwareMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VMwareMachineSpec   `json:"spec,omitempty"`
	Status VMwareMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VMwareMachineList contains a list of VMwareMachine
type VMwareMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VMwareMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VMwareMachine{}, &VMwareMachineList{})
}
