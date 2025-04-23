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

type DiskInfo struct {
	Label      string `json:"label,omitempty"`
	DiskPath   string `json:"diskpath,omitempty"`
	SizeBytes  int64  `json:"sizebytes,omitempty"`
	DiskType   string `json:"disktype,omitempty"`
	UnitNumber int32  `json:"unitnumber,omitempty"`
	IsRDM      bool   `json:"isrdm,omitempty"`
	RDMType    string `json:"rdmtype,omitempty"` // 'physical' or 'virtual' for RDM disks
	UUID       string `json:"uuid,omitempty"`
}

type VMInfo struct {
	Name       string      `json:"name"`
	Datastores []string    `json:"datastores,omitempty"`
	Disks      []*DiskInfo `json:"disks,omitempty"`
	Networks   []string    `json:"networks,omitempty"`
	IPAddress  string      `json:"ipAddress,omitempty"`
	VMState    string      `json:"vmState,omitempty"`
	OSType     string      `json:"osType,omitempty"`
	CPU        int         `json:"cpu,omitempty"`
	Memory     int         `json:"memory,omitempty"`
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
