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
	DiskName           string        `json:"diskName"`
	DiskSize           int           `json:"diskSize"`
	UUID               string        `json:"uuid"`
	DisplayName        string        `json:"displayName"`
	OwnerVMs           []string      `json:"ownerVMs"`
	OpenstackVolumeRef VolumeRefInfo `json:"openstackVolumeRef"`
	ImportToCinder     bool          `json:"importToCinder,omitempty"` // Indicates if the RDM disk is being imported
}

// RdmDiskStatus defines the observed state of RdmDisk.
type RdmDiskStatus struct {
	// +kubebuilder:validation:Enum=Created;Migrate;Managing;Managed;Error
	Phase          string             `json:"phase,omitempty"` // Created | Migrate | Managing | Managed | Error
	CinderVolumeID string             `json:"cinderVolumeID,omitempty"`
	Conditions     []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RdmDisk is the Schema for the rdmdisks API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type RdmDisk struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the desired state of RdmDisk
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
