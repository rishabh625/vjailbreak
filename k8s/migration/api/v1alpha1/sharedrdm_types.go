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

// SharedRDMSpec defines the desired state of SharedRDM
type SharedRDMSpec struct {
	// SourceUUID is the VMware LUN UUID that identifies the source RDM device
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	SourceUUID string `json:"sourceUUID"`

	// DisplayName is a human-readable name for the shared RDM
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	DisplayName string `json:"displayName"`

	// DiskSize is the size of the RDM disk in bytes
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	DiskSize int64 `json:"diskSize"`

	// VMwareRefs is a list of VMware VM names that are using this shared RDM
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	VMwareRefs []string `json:"vmwareRefs"`

	// OpenstackVolumeRef contains reference information for the created Cinder volume
	// +kubebuilder:validation:Optional
	OpenstackVolumeRef OpenStackVolumeRefInfo `json:"openstackVolumeRef,omitempty"`

	// MigrationStrategy defines the strategy for data migration
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=copy;reuse;manual
	// +kubebuilder:default=copy
	MigrationStrategy string `json:"migrationStrategy"`

	// CinderBackendPool specifies the target Cinder backend pool for volume creation
	// +kubebuilder:validation:Optional
	CinderBackendPool string `json:"cinderBackendPool,omitempty"`

	// VolumeType specifies the target OpenStack volume type
	// +kubebuilder:validation:Optional
	VolumeType string `json:"volumeType,omitempty"`
}

// SharedRDMStatus defines the observed state of SharedRDM
type SharedRDMStatus struct {
	// Phase represents the current phase of the SharedRDM resource
	// +kubebuilder:validation:Enum=Discovering;Creating;Ready;Failed
	// +kubebuilder:default=Discovering
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the SharedRDM's state
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// VolumeID is the ID of the created OpenStack volume
	// +kubebuilder:validation:Optional
	VolumeID string `json:"volumeID,omitempty"`

	// AttachedVMs is a list of VM names currently using this shared volume
	// +kubebuilder:validation:Optional
	AttachedVMs []string `json:"attachedVMs,omitempty"`

	// CreationTime is the time when the SharedRDM resource was created
	// +kubebuilder:validation:Optional
	CreationTime *metav1.Time `json:"creationTime,omitempty"`

	// LastUpdateTime is the time when the SharedRDM resource was last updated
	// +kubebuilder:validation:Optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// ErrorMessage contains any error message from the last operation
	// +kubebuilder:validation:Optional
	ErrorMessage string `json:"errorMessage,omitempty"`

	// MigrationProgress indicates the progress of data migration (0-100)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=0
	MigrationProgress int32 `json:"migrationProgress,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced,shortName=srdm
//+kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
//+kubebuilder:printcolumn:name="Source UUID",type=string,JSONPath=`.spec.sourceUUID`
//+kubebuilder:printcolumn:name="Disk Size",type=string,JSONPath=`.spec.diskSize`
//+kubebuilder:printcolumn:name="Strategy",type=string,JSONPath=`.spec.migrationStrategy`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Volume ID",type=string,JSONPath=`.status.volumeID`
//+kubebuilder:printcolumn:name="Progress",type=string,JSONPath=`.status.migrationProgress`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SharedRDM is the Schema for the sharedrdms API
// +kubebuilder:resource:path=sharedrdms,scope=Namespaced
type SharedRDM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SharedRDMSpec   `json:"spec,omitempty"`
	Status SharedRDMStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SharedRDMList contains a list of SharedRDM
type SharedRDMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SharedRDM `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SharedRDM{}, &SharedRDMList{})
}