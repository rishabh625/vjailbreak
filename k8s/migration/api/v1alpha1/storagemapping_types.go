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

// Storage defines the storage mapping configuration for VMware to OpenStack migration
type Storage struct {
	// VMware storage identifier (datastore name or ID)
	// +kubebuilder:validation:Required
	VMwareStorage string `json:"vmwareStorage"`

	// OpenStack storage backend or volume type
	// +kubebuilder:validation:Required
	OpenstackStorage string `json:"openstackStorage"`

	// Storage class for persistent volumes
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// RDM-specific volume type for OpenStack
	// +optional
	RDMVolumeType string `json:"rdmVolumeType,omitempty"`

	// Specific Cinder backend pool for RDM volumes
	// +optional
	RDMBackendPool string `json:"rdmBackendPool,omitempty"`

	// Whether the target storage supports multi-attach for shared RDM
	// +optional
	SupportsMultiAttach bool `json:"supportsMultiAttach,omitempty"`

	// Default strategy for RDM migration (copy, reuse, skip)
	// +kubebuilder:validation:Enum=copy;reuse;skip
	// +kubebuilder:default="copy"
	// +optional
	RDMigrationStrategy string `json:"rdmMigrationStrategy,omitempty"`
}

// RDMStorageMapping defines specific storage mapping for RDM disks
type RDMStorageMapping struct {
	// Type of VMware RDM (physical, virtual)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=physical;virtual
	SourceRDMType string `json:"sourceRDMType"`

	// Target OpenStack volume type
	// +kubebuilder:validation:Required
	TargetVolumeType string `json:"targetVolumeType"`

	// How to migrate the data (copy, reuse, manual)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=copy;reuse;manual
	MigrationMethod string `json:"migrationMethod"`

	// Whether multi-attach is required for this mapping
	// +optional
	RequiresMultiAttach bool `json:"requiresMultiAttach,omitempty"`

	// Specific backend pool for this RDM type
	// +optional
	BackendPool string `json:"backendPool,omitempty"`

	// Additional volume properties for RDM volumes
	// +optional
	VolumeProperties map[string]string `json:"volumeProperties,omitempty"`
}

// StorageMappingSpec defines the desired state of StorageMapping
type StorageMappingSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// List of storage mappings from VMware to OpenStack
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Storages []Storage `json:"storages"`

	// RDM-specific storage mappings that override default storage mappings for RDM disks
	// +optional
	RDMMappings []RDMStorageMapping `json:"rdmMappings,omitempty"`

	// Default RDM migration strategy if not specified in storage mapping
	// +kubebuilder:validation:Enum=copy;reuse;skip
	// +kubebuilder:default="copy"
	// +optional
	DefaultRDMStrategy string `json:"defaultRDMStrategy,omitempty"`

	// Whether to enable automatic RDM detection and mapping
	// +kubebuilder:default=true
	// +optional
	EnableRDMAutoMapping bool `json:"enableRDMAutoMapping,omitempty"`
}

// StorageMappingStatus defines the observed state of StorageMapping
type StorageMappingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Current phase of the storage mapping
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the storage mapping's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Number of storage mappings validated
	// +optional
	ValidatedMappings int `json:"validatedMappings,omitempty"`

	// Number of RDM mappings validated
	// +optional
	ValidatedRDMMappings int `json:"validatedRDMMappings,omitempty"`

	// Last validation time
	// +optional
	LastValidated *metav1.Time `json:"lastValidated,omitempty"`

	// Validation errors if any
	// +optional
	ValidationErrors []string `json:"validationErrors,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Mappings",type="integer",JSONPath=".status.validatedMappings"
//+kubebuilder:printcolumn:name="RDM Mappings",type="integer",JSONPath=".status.validatedRDMMappings"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// StorageMapping is the Schema for the storagemappings API
type StorageMapping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StorageMappingSpec   `json:"spec,omitempty"`
	Status StorageMappingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StorageMappingList contains a list of StorageMapping
type StorageMappingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageMapping `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageMapping{}, &StorageMappingList{})
}
