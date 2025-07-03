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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RdmDiskSpec defines the desired state of RdmDisk.
type RdmDiskSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	DiskName        string        `json:"diskName"`
	DiskSize        int           `json:"diskSize"`
	UUID            string        `json:"uuid"`
	DisplayName     string        `json:"displayName"`
	OwnerVMs        []string      `json:"ownerVMs"`
	VolumeRef       VolumeRefInfo `json:"volumeRef"`
	CinderReference string        `json:"cinderReference"`
}

// RdmDiskStatus defines the observed state of RdmDisk.
type RdmDiskStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +kubebuilder:validation:Enum=Pending;Managed;Error
	Phase     string `json:"phase,omitempty"` // Pending | Managed | Error
	Validated bool   `json:"validated,omitempty"`
	Error     string `json:"error,omitempty"` // Error message if any
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RdmDisk is the Schema for the rdmdisks API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type RdmDisk struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RdmDiskSpec   `json:"spec,omitempty"`
	Status RdmDiskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RdmDiskList contains a list of RdmDisk.
type RdmDiskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RdmDisk `json:"items"`
}

type VolumeRefInfo struct {
	Source            map[string]string `json:"source"`
	CinderBackendPool string            `json:"cinderBackendPool"`
	VolumeType        string            `json:"volumeType"`
}

func init() {
	SchemeBuilder.Register(&RdmDisk{}, &RdmDiskList{})
}
