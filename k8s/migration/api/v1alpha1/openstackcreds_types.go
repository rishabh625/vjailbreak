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
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HostConfig defines the configuration for a Platform9 Distributed Cloud host
type HostConfig struct {
	ID                    string            `json:"id,omitempty"`
	Name                  string            `json:"name,omitempty"`
	MgmtInterface         string            `json:"mgmtInterface,omitempty"`
	VMConsoleInterface    string            `json:"vmConsoleInterface,omitempty"`
	HostLivenessInterface string            `json:"hostLivenessInterface,omitempty"`
	TunnelingInterface    string            `json:"tunnelingInterface,omitempty"`
	ImagelibInterface     string            `json:"imagelibInterface,omitempty"`
	NetworkLabels         map[string]string `json:"networkLabels,omitempty"`
	ClusterName           string            `json:"clusterName,omitempty"`
}

// OpenStackCredsInfo holds the actual credentials after decoding
type OpenStackCredsInfo struct {
	// AuthURL is the OpenStack authentication URL
	AuthURL string
	// Username is the OpenStack username
	Username string
	// Password is the OpenStack password
	Password string
	// RegionName is the OpenStack region
	RegionName string
	// TenantName is the OpenStack tenant
	TenantName string
	// Insecure is whether to skip certificate verification
	Insecure bool
	// DomainName is the OpenStack domain
	DomainName string
}

// OpenstackInfo contains information about OpenStack environment resources including available volume types and networks
type OpenstackInfo struct {
	VolumeTypes []string `json:"volumeTypes,omitempty"`
	Networks    []string `json:"networks,omitempty"`
}

// OpenstackCredsSpec defines the desired state of OpenstackCreds
type OpenstackCredsSpec struct {
	// SecretRef is the reference to the Kubernetes secret holding OpenStack credentials
	SecretRef corev1.ObjectReference `json:"secretRef,omitempty"`

	// Flavors is the list of available flavors in openstack
	Flavors []flavors.Flavor `json:"flavors,omitempty"`

	// PCDHostConfig is the list of available clusters in openstack
	PCDHostConfig []HostConfig `json:"pcdHostConfig,omitempty"`
}

// OpenstackCredsStatus defines the observed state of OpenstackCreds
type OpenstackCredsStatus struct {
	// Openstack is the OpenStack configuration for the openstackcreds
	Openstack OpenstackInfo `json:"openstack,omitempty"`
	// OpenStackValidationStatus is the status of the OpenStack validation
	OpenStackValidationStatus string `json:"openstackValidationStatus,omitempty"`
	// OpenStackValidationMessage is the message associated with the OpenStack validation
	OpenStackValidationMessage string `json:"openstackValidationMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=`.status.openstackValidationStatus`,name=Status,type=string

// OpenstackCreds is the Schema for the OpenStack credentials API that defines authentication
// and connection details for OpenStack environments. It provides a secure way to store and validate
// OpenStack credentials for use in migration operations, including authentication information,
// available compute flavors, volume types, networks, and Platform9 Distributed Cloud host configurations.
type OpenstackCreds struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenstackCredsSpec   `json:"spec,omitempty"`
	Status OpenstackCredsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenstackCredsList contains a list of OpenstackCreds
type OpenstackCredsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenstackCreds `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenstackCreds{}, &OpenstackCredsList{})
}
