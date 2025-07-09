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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VjailbreakNodePhase represents the lifecycle phase of a vjailbreak node
// including provisioning, ready, and error states
type VjailbreakNodePhase string

// VjailbreakNodeSpec defines the desired state of VjailbreakNode including
// node configuration, resource limits, and credentials for provisioning
type VjailbreakNodeSpec struct {
	// NodeRole is the role assigned to the node (e.g., "worker", "controller")
	NodeRole string `json:"nodeRole"`

	// OpenstackCreds is the reference to the credentials for the OpenStack environment
	// where the node will be provisioned
	OpenstackCreds corev1.ObjectReference `json:"openstackCreds"`

	// OpenstackFlavorID is the flavor of the VM
	OpenstackFlavorID string `json:"openstackFlavorID"`

	// OpenstackImageID is the image of the VM
	OpenstackImageID string `json:"openstackImageID"`
}

// VjailbreakNodeStatus defines the observed state of VjailbreakNode including
// migration statistics, health status, and current workload
type VjailbreakNodeStatus struct {
	// OpenstackUUID is the UUID of the VM in OpenStack
	OpenstackUUID string `json:"openstackUUID,omitempty"`

	// VMIP is the IP address of the VM
	VMIP string `json:"vmIP"`

	// Phase is the current lifecycle phase of the node
	// (e.g., Provisioning, Ready, Error, Decommissioning)
	Phase VjailbreakNodePhase `json:"phase,omitempty"`

	// ActiveMigrations is the list of active migrations currently being processed on this node,
	// containing references to MigrationPlan resources
	ActiveMigrations []string `json:"activeMigrations,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=`.status.phase`,name=Phase,type=string
// +kubebuilder:printcolumn:JSONPath=`.status.vmIP`,name=VMIP,type=string

// VjailbreakNode is the Schema for the vjailbreaknodes API that represents
// a node in the migration infrastructure with configuration, resource limits,
// and statistics for monitoring migration progress
type VjailbreakNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of VjailbreakNode
	Spec VjailbreakNodeSpec `json:"spec,omitempty"`

	// Status defines the observed state of VjailbreakNode
	Status VjailbreakNodeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VjailbreakNodeList contains a list of VjailbreakNode
type VjailbreakNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VjailbreakNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VjailbreakNode{}, &VjailbreakNodeList{})
}
