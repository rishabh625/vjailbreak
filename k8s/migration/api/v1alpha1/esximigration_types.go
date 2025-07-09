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

// ESXIMigrationPhase represents the current phase of the ESXi host migration process
type ESXIMigrationPhase string

const (
	// ESXIMigrationPhaseWaiting indicates the ESXi migration is in initial waiting state
	ESXIMigrationPhaseWaiting ESXIMigrationPhase = "Waiting"
	// ESXIMigrationPhaseRunning indicates the ESXi migration is actively executing
	ESXIMigrationPhaseRunning ESXIMigrationPhase = "Running"
	// ESXIMigrationPhaseCordoned indicates the ESXi host has been cordoned to prevent new VM placements
	ESXIMigrationPhaseCordoned ESXIMigrationPhase = "Cordoned"
	// ESXIMigrationPhaseInMaintenanceMode indicates the ESXi host has been placed in maintenance mode
	ESXIMigrationPhaseInMaintenanceMode ESXIMigrationPhase = "InMaintenanceMode"
	// ESXIMigrationPhaseWaitingForVMsToBeMoved indicates the migration is waiting for all VMs to be moved off the host
	ESXIMigrationPhaseWaitingForVMsToBeMoved ESXIMigrationPhase = "WaitingForVMsToBeMoved"
	// ESXIMigrationPhaseConvertingToPCDHost indicates the ESXi host is being converted to a PCD host
	ESXIMigrationPhaseConvertingToPCDHost ESXIMigrationPhase = "ConvertingToPCDHost"
	// ESXIMigrationPhaseAssigningRole indicates roles are being assigned to the newly converted PCD host
	ESXIMigrationPhaseAssigningRole ESXIMigrationPhase = "AssigningRole"
	// ESXIMigrationPhaseFailed indicates the ESXi migration has failed
	ESXIMigrationPhaseFailed ESXIMigrationPhase = "Failed"
	// ESXIMigrationPhaseSucceeded indicates the ESXi migration has completed successfully
	ESXIMigrationPhaseSucceeded ESXIMigrationPhase = "Succeeded"
	// ESXIMigrationPhaseWaitingForPCDHost indicates the migration is waiting for the PCD host to be created or become available
	ESXIMigrationPhaseWaitingForPCDHost ESXIMigrationPhase = "WaitingForPCDHost"
	// ESXIMigrationPhaseConfiguringPCDHost indicates the PCD host is being configured with appropriate settings
	ESXIMigrationPhaseConfiguringPCDHost ESXIMigrationPhase = "ConfiguringPCDHost"
	// ESXIMigrationPhasePaused indicates the migration has been temporarily paused and can be resumed later
	ESXIMigrationPhasePaused ESXIMigrationPhase = "Paused"
)

// ESXIMigrationSpec defines the desired state of ESXIMigration including
// the ESXi host to migrate and the references to credentials and migration plan
type ESXIMigrationSpec struct {
	// ESXiName is the name of the ESXi host to be migrated
	ESXiName string `json:"esxiName"`
	// OpenstackCredsRef is the reference to the OpenStack credentials
	OpenstackCredsRef corev1.LocalObjectReference `json:"openstackCredsRef"`
	// VMwareCredsRef is the reference to the VMware credentials
	VMwareCredsRef corev1.LocalObjectReference `json:"vmwareCredsRef"`
	// RollingMigrationPlanRef is the reference to the RollingMigrationPlan
	RollingMigrationPlanRef corev1.LocalObjectReference `json:"rollingMigrationPlanRef"`
}

// ESXIMigrationStatus defines the observed state of ESXIMigration including
// the list of VMs on the host, current phase, and status messages
type ESXIMigrationStatus struct {
	// VMs is the list of VMs present on the ESXi host
	VMs []string `json:"vms,omitempty"`
	// Phase is the current phase of the migration lifecycle
	// The final phases include 'Succeeded' when the ESXi host has been successfully
	// removed from vCenter inventory after migration is complete
	Phase ESXIMigrationPhase `json:"phase,omitempty"`
	// Message is the message associated with the current state of the migration
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ESXIMigration is the Schema for the esximigrations API that defines
// the process of migrating an ESXi host to PCD, including putting it in maintenance mode,
// migrating all VMs, and finally removing it from vCenter inventory after completion
type ESXIMigration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ESXIMigrationSpec   `json:"spec,omitempty"`
	Status ESXIMigrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ESXIMigrationList contains a list of ESXIMigration
type ESXIMigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ESXIMigration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ESXIMigration{}, &ESXIMigrationList{})
}
