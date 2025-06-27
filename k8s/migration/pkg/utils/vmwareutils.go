// Package utils provides utility functions for handling migration-related operations.
// It includes functions for working with VMware environments, ESXi hosts, VM management,
// and integration with Platform9 components for migration workflows.
package utils

import (
	"context"

	"github.com/pkg/errors"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	constants "github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	scope "github.com/platform9/vjailbreak/k8s/migration/pkg/scope"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/mo"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VMwareHostInfo represents a host in a VMware cluster.
// It contains essential information about a VMware ESXi host.
type VMwareHostInfo struct {
	// Name is the fully qualified domain name or IP address of the host
	Name string
	// HardwareUUID is the unique identifier of the host
	HardwareUUID string
}

// VMwareClusterInfo represents a cluster in a VMware environment.
// It contains information about a VMware cluster and its associated hosts.
type VMwareClusterInfo struct {
	// Name is the unique identifier of the cluster
	Name string
	// Hosts is a list of ESXi hosts that are part of this cluster
	Hosts []VMwareHostInfo
}

// GetVMwareClustersAndHosts retrieves a list of all available VMware clusters and their hosts
func GetVMwareClustersAndHosts(ctx context.Context, k3sclient client.Client, scope *scope.VMwareCredsScope) ([]VMwareClusterInfo, error) {
	// Pre-allocate clusters slice with initial capacity
	clusters := make([]VMwareClusterInfo, 0, 4)
	vmwarecreds, err := GetVMwareCredentialsFromSecret(ctx, k3sclient, scope.VMwareCreds.Spec.SecretRef.Name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get vCenter credentials")
	}
	c, err := ValidateVMwareCreds(ctx, k3sclient, scope.VMwareCreds)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate vCenter connection")
	}
	finder := find.NewFinder(c, false)
	dc, err := finder.Datacenter(ctx, vmwarecreds.Datacenter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find datacenter")
	}
	finder.SetDatacenter(dc)
	clusterList, err := finder.ClusterComputeResourceList(ctx, "*")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cluster list")
	}

	for _, cluster := range clusterList {
		var clusterProperties mo.ClusterComputeResource
		err := cluster.Properties(ctx, cluster.Reference(), []string{"name"}, &clusterProperties)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get cluster properties")
		}

		hosts, err := cluster.Hosts(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get hosts")
		}
		var vmHosts []VMwareHostInfo
		for _, host := range hosts {
			hostSummary, err := GetESXiSummary(ctx, k3sclient, host.Name(), scope.VMwareCreds)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get ESXi summary")
			}
			vmHosts = append(vmHosts, VMwareHostInfo{Name: host.Name(), HardwareUUID: hostSummary.Summary.Hardware.Uuid})
		}
		clusters = append(clusters, VMwareClusterInfo{
			Name:  clusterProperties.Name,
			Hosts: vmHosts,
		})
	}
	return clusters, nil
}

// createVMwareHost creates a VMware host resource in Kubernetes
func createVMwareHost(ctx context.Context, k3sclient client.Client, host VMwareHostInfo, credName, clusterName, namespace string) (string, error) {
	hostk8sName, err := ConvertToK8sName(host.Name)
	if err != nil {
		return "", errors.Wrap(err, "failed to convert host name to k8s name")
	}

	vmwareHost := vjailbreakv1alpha1.VMwareHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostk8sName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.VMwareClusterLabel: clusterName,
				constants.VMwareCredsLabel:   credName,
			},
		},
		Spec: vjailbreakv1alpha1.VMwareHostSpec{
			Name:         host.Name,
			HardwareUUID: host.HardwareUUID,
			ClusterName:  clusterName,
		},
	}
	existingHost := vjailbreakv1alpha1.VMwareHost{}
	if err := k3sclient.Get(ctx, client.ObjectKey{Name: hostk8sName, Namespace: namespace}, &existingHost); err == nil {
		if existingHost.Spec.Name != host.Name || existingHost.Spec.HardwareUUID != host.HardwareUUID || existingHost.Spec.ClusterName != clusterName {
			existingHost.Spec = vmwareHost.Spec
			updateErr := k3sclient.Update(ctx, &existingHost)
			if updateErr != nil {
				return "", errors.Wrap(updateErr, "failed to update vmware host")
			}
		}
	} else {
		err = k3sclient.Create(ctx, &vmwareHost)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return "", errors.Wrap(err, "failed to create vmware host")
		}
	}

	return hostk8sName, nil
}

// createVMwareCluster creates a VMware cluster resource in Kubernetes
func createVMwareCluster(ctx context.Context, k3sclient client.Client, cluster VMwareClusterInfo, scope *scope.VMwareCredsScope) error {
	clusterk8sName, err := ConvertToK8sName(cluster.Name)
	if err != nil {
		return errors.Wrap(err, "failed to convert cluster name to k8s name")
	}

	vmwareCluster := vjailbreakv1alpha1.VMwareCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterk8sName,
			Namespace: scope.Namespace(),
			Labels: map[string]string{
				constants.VMwareCredsLabel: scope.Name(),
			},
		},
		Spec: vjailbreakv1alpha1.VMwareClusterSpec{
			Name: cluster.Name,
		},
	}

	// Create hosts and collect their k8s names
	for _, host := range cluster.Hosts {
		hostk8sName, err := createVMwareHost(ctx, k3sclient, host, scope.Name(), cluster.Name, scope.Namespace())
		if err != nil {
			return err
		}
		vmwareCluster.Spec.Hosts = append(vmwareCluster.Spec.Hosts, hostk8sName)
	}

	// Create the cluster
	existingCluster := vjailbreakv1alpha1.VMwareCluster{}
	if err := k3sclient.Get(ctx, client.ObjectKey{Name: clusterk8sName, Namespace: scope.Namespace()}, &existingCluster); err == nil {
		if existingCluster.Spec.Name != cluster.Name {
			existingCluster.Spec = vmwareCluster.Spec
			updateErr := k3sclient.Update(ctx, &existingCluster)
			if updateErr != nil {
				return errors.Wrap(updateErr, "failed to update vmware cluster")
			}
		}
	} else {
		createErr := k3sclient.Create(ctx, &vmwareCluster)
		if createErr != nil && !apierrors.IsAlreadyExists(createErr) {
			return errors.Wrap(createErr, "failed to create vmware cluster")
		}
	}

	return nil
}

// CreateVMwareClustersAndHosts creates VMware clusters and hosts
func CreateVMwareClustersAndHosts(ctx context.Context, k3sclient client.Client, scope *scope.VMwareCredsScope) error {
	clusters, err := GetVMwareClustersAndHosts(ctx, k3sclient, scope)
	if err != nil {
		return errors.Wrap(err, "failed to get clusters and hosts")
	}

	for _, cluster := range clusters {
		if err := createVMwareCluster(ctx, k3sclient, cluster, scope); err != nil {
			return err
		}
	}
	return nil
}

// DeleteStaleVMwareClustersAndHosts removes VMware cluster and host resources that no longer exist in vCenter.
// This helps maintain synchronization between Kubernetes resources and the actual infrastructure state.
func DeleteStaleVMwareClustersAndHosts(ctx context.Context, k3sclient client.Client, scope *scope.VMwareCredsScope) error {
	clusters, err := GetVMwareClustersAndHosts(ctx, k3sclient, scope)
	if err != nil {
		return errors.Wrap(err, "failed to get clusters and hosts")
	}
	hosts := []VMwareHostInfo{}
	for _, cluster := range clusters {
		hosts = append(hosts, cluster.Hosts...)
	}
	existingClusters := vjailbreakv1alpha1.VMwareClusterList{}
	if err := k3sclient.List(ctx, &existingClusters, client.MatchingLabels{constants.VMwareCredsLabel: scope.Name()}); err != nil {
		return errors.Wrap(err, "failed to list vmware clusters")
	}
	existingHosts := vjailbreakv1alpha1.VMwareHostList{}
	if err := k3sclient.List(ctx, &existingHosts, client.MatchingLabels{constants.VMwareCredsLabel: scope.Name()}); err != nil {
		return errors.Wrap(err, "failed to list vmware hosts")
	}
	// Create a map of valid cluster names for O(1) lookups
	clusterNames := make(map[string]bool)
	for _, cluster := range clusters {
		cname, err := ConvertToK8sName(cluster.Name)
		if err != nil {
			return errors.Wrap(err, "failed to convert cluster name to k8s name")
		}
		clusterNames[cname] = true
	}

	// Delete only clusters that don't exist in vSphere anymore
	for _, existingCluster := range existingClusters.Items {
		if !clusterNames[existingCluster.Name] {
			if err := k3sclient.Delete(ctx, &existingCluster); err != nil {
				return errors.Wrap(err, "failed to delete stale vmware cluster")
			}
		}
	}

	// Create a map of valid host names for O(1) lookups
	hostNames := make(map[string]bool)
	for _, host := range hosts {
		hname, err := ConvertToK8sName(host.Name)
		if err != nil {
			return errors.Wrap(err, "failed to convert host name to k8s name")
		}
		hostNames[hname] = true
	}

	// Delete only hosts that don't exist in vSphere anymore
	for _, existingHost := range existingHosts.Items {
		if !hostNames[existingHost.Name] {
			if err := k3sclient.Delete(ctx, &existingHost); err != nil {
				return errors.Wrap(err, "failed to delete stale vmware host")
			}
		}
	}
	return nil
}
