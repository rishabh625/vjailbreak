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

// MigrationTemplateSource defines the source environment details for the migration template
type MigrationTemplateSource struct {
	// VMwareRef is the reference to the VMware credentials to be used as the source environment
	VMwareRef string `json:"vmwareRef"`
}

// MigrationTemplateDestination defines the destination environment details for the migration template
type MigrationTemplateDestination struct {
	// OpenstackRef is the reference to the OpenStack credentials to be used as the destination environment
	OpenstackRef string `json:"openstackRef"`
}

// MigrationTemplateSpec defines the desired state of MigrationTemplate including source/destination environments and mappings
type MigrationTemplateSpec struct {
	// OSFamily is the OS type of the virtual machine
	// +kubebuilder:validation:Enum=windowsGuest;linuxGuest
	OSFamily string `json:"osFamily,omitempty"`
	// VirtioWinDriver is the driver to be used for the virtual machine
	VirtioWinDriver string `json:"virtioWinDriver,omitempty"`
	// NetworkMapping is the reference to the NetworkMapping resource that defines source to destination network mappings
	NetworkMapping string `json:"networkMapping"`
	// StorageMapping is the reference to the StorageMapping resource that defines source to destination storage mappings
	StorageMapping string `json:"storageMapping"`
	// Source is the source details for the virtual machine
	Source MigrationTemplateSource `json:"source"`
	// Destination is the destination details for the virtual machine
	Destination MigrationTemplateDestination `json:"destination"`
	// TargetPCDClusterName is the name of the PCD cluster where the virtual machine will be migrated
	TargetPCDClusterName string `json:"targetPCDClusterName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MigrationTemplate is the Schema for the migrationtemplates API that defines how VMs should be migrated
// from VMware to OpenStack including network and storage mappings. It serves as a reusable template
// that can be referenced by multiple migration plans, providing configuration for source and destination
// environments, OS-specific settings, and network/storage mappings. Migration templates enable consistent
// configuration across multiple VM migrations and simplify the definition of migration plans.
type MigrationTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MigrationTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// MigrationTemplateList contains a list of MigrationTemplate
type MigrationTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MigrationTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MigrationTemplate{}, &MigrationTemplateList{})
}
