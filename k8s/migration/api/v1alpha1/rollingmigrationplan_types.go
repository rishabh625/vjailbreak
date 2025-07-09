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

// RollingMigrationPlanPhase represents the current phase of a rolling migration plan execution,
// tracking the state transitions from initial waiting through running, VM migration, and final completion or failure.
type RollingMigrationPlanPhase string

// ClusterMapping defines the relationship between a VMware vCenter cluster and its corresponding
// Platform9 Distributed Cloud (PCD) cluster for migration operations. This mapping ensures that
// virtual machines are properly migrated to the appropriate target infrastructure.
type ClusterMapping struct {
	// VMwareClusterName is the name of the vCenter cluster
	VMwareClusterName string `json:"vmwareClusterName"`
	// PCDClusterName is the name of the PCD cluster
	PCDClusterName string `json:"pcdClusterName"`
}

const (
	// RollingMigrationPlanPhaseWaiting is the phase for waiting
	RollingMigrationPlanPhaseWaiting RollingMigrationPlanPhase = "Waiting"
	// RollingMigrationPlanPhaseRunning is the phase for running
	RollingMigrationPlanPhaseRunning RollingMigrationPlanPhase = "Running"
	// RollingMigrationPlanPhaseFailed is the phase for failed
	RollingMigrationPlanPhaseFailed RollingMigrationPlanPhase = "Failed"
	// RollingMigrationPlanPhaseValidating is the phase for paused
	RollingMigrationPlanPhaseValidating RollingMigrationPlanPhase = "Validating"
	// RollingMigrationPlanPhaseValidated is the phase for validated
	RollingMigrationPlanPhaseValidated RollingMigrationPlanPhase = "Validated"
	// RollingMigrationPlanPhaseValidationFailed is the phase for validation failed
	RollingMigrationPlanPhaseValidationFailed RollingMigrationPlanPhase = "ValidationFailed"
	// RollingMigrationPlanPhaseSucceeded is the phase for succeeded
	RollingMigrationPlanPhaseSucceeded RollingMigrationPlanPhase = "Succeeded"
	// RollingMigrationPlanPhaseDeleting is the phase for deleting
	RollingMigrationPlanPhaseDeleting RollingMigrationPlanPhase = "Deleting"
	// RollingMigrationPlanPhaseMigratingVMs is the phase for migrating VMs
	RollingMigrationPlanPhaseMigratingVMs RollingMigrationPlanPhase = "MigratingVMs"
)

// VMSequenceInfo defines information about a virtual machine in the migration sequence,
// including its name and the ESXi host where it is located. This information is used to
// establish the proper order and grouping of VMs during the migration process.
type VMSequenceInfo struct {
	// VMName is the name of the virtual machine to be migrated
	VMName string `json:"vmName"`
	// ESXiName is the name of the ESXi host where the virtual machine is located
	ESXiName string `json:"esxiName,omitempty"`
}

// ClusterMigrationInfo defines information about a VMware vCenter cluster migration,
// including the cluster name and the sequence of virtual machines to be migrated.
// This structure allows for coordinated migration of multiple related VMs within a cluster.
type ClusterMigrationInfo struct {
	// ClusterName is the name of the vCenter cluster to be migrated
	ClusterName string `json:"clusterName"`
	// VMSequence is the sequence of virtual machines to be migrated
	VMSequence []VMSequenceInfo `json:"vmSequence"`
	// VMMigrationBatchSize is the number of VMs in one batch for migration
	// batches will be processed sequentially, but all VMs in a batch
	// will be migrated in parallel. Default is 10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	VMMigrationBatchSize int `json:"vmMigrationBatchSize,omitempty"`
}

// RollingMigrationPlanSpec defines the desired state of RollingMigrationPlan
type RollingMigrationPlanSpec struct {
	// ClusterSequence is the sequence of vCenter clusters to be migrated
	ClusterSequence []ClusterMigrationInfo `json:"clusterSequence"`

	// BMConfigRef is the reference to the BMC credentials
	BMConfigRef corev1.LocalObjectReference `json:"bmConfigRef"`

	// CloudInitConfigRef is the reference to the cloud-init configuration
	CloudInitConfigRef *corev1.SecretReference `json:"cloudInitConfigRef,omitempty"`

	// VMMigrationPlans is the reference to the VM migration plan
	VMMigrationPlans []string `json:"vmMigrationPlans,omitempty"`

	// ClusterMapping is the mapping of vCenter clusters to PCD clusters
	ClusterMapping []ClusterMapping `json:"clusterMapping,omitempty"`

	// MigrationPlanSpecPerVM is the migration plan specification per virtual machine
	MigrationPlanSpecPerVM `json:",inline"`
}

// RollingMigrationPlanStatus defines the observed state of RollingMigrationPlan
type RollingMigrationPlanStatus struct {
	// Phase is the current phase of the migration
	Phase RollingMigrationPlanPhase `json:"phase,omitempty"`
	// VMMigrationsPhase is the list of VM migration plans
	VMMigrationsPhase string `json:"vmMigrationPhase,omitempty"`
	// CurrentESXi is the name of the current ESXi host being migrated
	CurrentESXi string `json:"currentESXi,omitempty"`
	// CurrentCluster is the name of the current vCenter cluster being migrated
	CurrentCluster string `json:"currentCluster,omitempty"`
	// CurrentVM is the name of the current virtual machine being migrated
	CurrentVM string `json:"currentVM,omitempty"`
	// Message is the message associated with the current state of the migration
	Message string `json:"message,omitempty"`
	// MigratedVMs is the list of virtual machines that have been migrated
	MigratedVMs []string `json:"migratedVMs,omitempty"`
	// FailedVMs is the list of virtual machines that have failed to migrate
	FailedVMs []string `json:"failedVMs,omitempty"`
	// MigratedESXi is the list of ESXi hosts that have been migrated
	MigratedESXi []string `json:"migratedESXi,omitempty"`
	// FailedESXi is the list of ESXi hosts that have failed to migrate
	FailedESXi []string `json:"failedESXi,omitempty"`
	// MigratedClusters is the list of vCenter clusters that have been migrated
	MigratedClusters []string `json:"migratedClusters,omitempty"`
	// FailedClusters is the list of vCenter clusters that have failed to migrate
	FailedClusters []string `json:"failedClusters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RollingMigrationPlan is the Schema for the rollingmigrationplans API that defines a coordinated
// migration of multiple VMware clusters and ESXi hosts to Platform9 Distributed Cloud (PCD).
// It supports sequenced migration of VMs across clusters with configurable batch sizes,
// cluster-to-cluster mapping, and tracking of migration progress across the entire datacenter migration.
type RollingMigrationPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RollingMigrationPlanSpec   `json:"spec,omitempty"`
	Status RollingMigrationPlanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RollingMigrationPlanList contains a list of RollingMigrationPlan
type RollingMigrationPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RollingMigrationPlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RollingMigrationPlan{}, &RollingMigrationPlanList{})
}
