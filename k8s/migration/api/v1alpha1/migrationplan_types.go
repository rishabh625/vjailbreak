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

// MigrationPlanStrategy defines the strategy for executing a migration plan including
// scheduling options and migration type (hot or cold)
type MigrationPlanStrategy struct {
	// +kubebuilder:validation:Enum=hot;cold
	Type string `json:"type"`
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format:=date-time
	DataCopyStart metav1.Time `json:"dataCopyStart,omitempty"`
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format:=date-time
	VMCutoverStart metav1.Time `json:"vmCutoverStart,omitempty"`
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format:=date-time
	VMCutoverEnd metav1.Time `json:"vmCutoverEnd,omitempty"`
	// +kubebuilder:default:=false
	AdminInitiatedCutOver bool `json:"adminInitiatedCutOver,omitempty"`
	// +kubebuilder:default:=false
	PerformHealthChecks bool `json:"performHealthChecks,omitempty"`
	// +kubebuilder:default:="443"
	HealthCheckPort string `json:"healthCheckPort,omitempty"`
}

// AdvancedOptions defines advanced configuration options for the migration process
// including granular selection of volumes, networks, and ports
type AdvancedOptions struct {
	// GranularVolumeTypes is a list of volume types to be migrated
	GranularVolumeTypes []string `json:"granularVolumeTypes,omitempty"`
	// GranularNetworks is a list of networks to be migrated
	GranularNetworks []string `json:"granularNetworks,omitempty"`
	// GranularPorts is a list of ports to be migrated
	GranularPorts []string `json:"granularPorts,omitempty"`
}

// PostMigrationAction defines the post migration action for the virtual machine
type PostMigrationAction struct {
	RenameVM     *bool  `json:"renameVm,omitempty"`
	Suffix       string `json:"suffix,omitempty"`
	MoveToFolder *bool  `json:"moveToFolder,omitempty"`
	FolderName   string `json:"folderName,omitempty"`
}

// MigrationPlanSpec defines the desired state of MigrationPlan including
// the migration template, strategy, and the list of virtual machines to migrate
type MigrationPlanSpec struct {
	// MigrationPlanSpecPerVM is the migration plan specification per virtual machine
	MigrationPlanSpecPerVM `json:",inline"`
	// VirtualMachines is a list of virtual machines to be migrated
	VirtualMachines [][]string `json:"virtualMachines"`
}

// MigrationPlanSpecPerVM defines the configuration that applies to each VM in the migration plan
type MigrationPlanSpecPerVM struct {
	// MigrationTemplate is the template to be used for the migration
	MigrationTemplate string `json:"migrationTemplate"`
	// MigrationStrategy is the strategy to be used for the migration
	MigrationStrategy MigrationPlanStrategy `json:"migrationStrategy"`
	// Retry the migration if it fails
	Retry bool `json:"retry,omitempty"`
	// AdvancedOptions is a list of advanced options for the migration
	AdvancedOptions AdvancedOptions `json:"advancedOptions,omitempty"`
	// +kubebuilder:default:="echo \"Add your startup script here!\""
	FirstBootScript     string               `json:"firstBootScript,omitempty"`
	PostMigrationAction *PostMigrationAction `json:"postMigrationAction,omitempty"`
}

// MigrationPlanStatus defines the observed state of MigrationPlan including
// the current status and progress of the migration
type MigrationPlanStatus struct {
	// MigrationStatus is the status of the migration using Kubernetes PodPhase states
	// (Pending, Running, Succeeded, Failed, Unknown)
	MigrationStatus corev1.PodPhase `json:"migrationStatus"`
	// MigrationMessage is the message associated with the migration
	MigrationMessage string `json:"migrationMessage"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=`.status.migrationStatus`,name=Status,type=string

// MigrationPlan is the Schema for the migrationplans API that defines
// how to migrate virtual machines from VMware to OpenStack including migration strategy and scheduling.
// It allows administrators to configure migration parameters such as timing, health checks,
// and VM-specific settings for bulk VM migration operations between environments.
type MigrationPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MigrationPlanSpec   `json:"spec,omitempty"`
	Status MigrationPlanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MigrationPlanList contains a list of MigrationPlan
type MigrationPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MigrationPlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MigrationPlan{}, &MigrationPlanList{})
}
