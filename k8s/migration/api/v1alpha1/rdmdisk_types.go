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

// RDMDiskSpec defines the desired state of RDMDisk.
type RDMDiskSpec struct {
	// Important: Run "make" to regenerate code after modifying this file
	DiskName           string             `json:"diskName"`
	DiskSize           int                `json:"diskSize"`
	UUID               string             `json:"uuid"`
	DisplayName        string             `json:"displayName"`
	OwnerVMs           []string           `json:"ownerVMs"`
	OpenstackVolumeRef OpenstackVolumeRef `json:"openstackVolumeRef"`       // OpenStack volume reference information
	ImportToCinder     bool               `json:"importToCinder,omitempty"` // Indicates whether the RDM disk should be imported to Cinder and is set by MigrationPlan Controller
}

// RDMDiskStatus defines the observed state of RDMDisk.
type RDMDiskStatus struct {
	// +kubebuilder:validation:Enum=Pending;Managing;Managed;Error
	Phase          string             `json:"phase,omitempty"` //  Available | Managing | Managed | Error
	CinderVolumeID string             `json:"cinderVolumeID,omitempty"`
	Conditions     []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RDMDisk is the Schema for the rdmdisks API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type RDMDisk struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the desired state of RDMDisk
	Spec   RDMDiskSpec   `json:"spec,omitempty"`
	Status RDMDiskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RDMDiskList contains a list of RDMDisk.
type RDMDiskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RDMDisk `json:"items"`
}

// OpenstackVolumeRef ... contains information about the OpenStack volume reference.
type OpenstackVolumeRef struct {
	VolumeRef         map[string]string `json:"source"` // volumeRef contains the OpenStack volume reference information - obtained by query - openstack block storage volume manageable list
	CinderBackendPool string            `json:"cinderBackendPool"`
	VolumeType        string            `json:"volumeType"`
	OpenstackCreds    string            `json:"openstackCreds,omitempty"` // Optional: OpenStack credentials to use for the volume

}

func init() {
	SchemeBuilder.Register(&RDMDisk{}, &RDMDiskList{})
}
