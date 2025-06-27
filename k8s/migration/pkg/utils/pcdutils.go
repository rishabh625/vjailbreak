// Package utils provides utility functions for Platform9 Distributed Cloud (PCD) operations.
// It includes functions for syncing, creating, updating, and deleting PCD resources,
// managing ESXi hosts, cluster operations, and other infrastructure management tasks
// related to VMware to OpenStack migrations.
package utils

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/sdk/resmgr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

// SyncPCDInfo syncs PCD info from resmgr
func SyncPCDInfo(ctx context.Context, k8sClient client.Client, openstackCreds vjailbreakv1alpha1.OpenstackCreds) error {
	OpenStackCredentials, err := GetOpenstackCredsInfo(ctx, k8sClient, openstackCreds.Name)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack credentials")
	}
	resmgrClient, err := GetResmgrClient(OpenStackCredentials)
	if err != nil {
		return errors.Wrap(err, "failed to get resmgr client")
	}

	// Get PCDHostConfig from openstackCreds
	pcdHostConfig, err := resmgrClient.ListHostConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list host configs")
	}
	openstackCreds.Spec.PCDHostConfig = pcdHostConfig

	if err := k8sClient.Update(ctx, &openstackCreds); err != nil {
		return errors.Wrap(err, "failed to update openstack creds")
	}

	clusterList, err := resmgrClient.ListClusters(ctx)
	if err != nil && !strings.Contains(err.Error(), "404") {
		return errors.Wrap(err, "failed to list clusters")
	}

	for _, cluster := range clusterList {
		err := CreatePCDClusterFromResmgrCluster(ctx, k8sClient, cluster, &openstackCreds)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				updateErr := UpdatePCDClusterFromResmgrCluster(ctx, k8sClient, cluster, &openstackCreds)
				if updateErr != nil {
					return errors.Wrap(updateErr, "failed to update PCD cluster")
				}
				continue
			}
			return errors.Wrap(err, "failed to create PCD cluster")
		}
	}
	hostList, err := resmgrClient.ListHosts(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list hosts")
	}
	for _, host := range hostList {
		err := CreatePCDHostFromResmgrHost(ctx, k8sClient, host, &openstackCreds)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				updateErr := UpdatePCDHostFromResmgrHost(ctx, k8sClient, host, &openstackCreds)
				if updateErr != nil {
					return errors.Wrap(updateErr, "failed to update PCD host")
				}
				continue
			}
			return errors.Wrap(err, "failed to create PCD host")
		}
	}
	err = DeleteStalePCDHosts(ctx, k8sClient, openstackCreds)
	if err != nil {
		return errors.Wrap(err, "failed to delete stale PCD hosts")
	}
	err = DeleteStalePCDClusters(ctx, k8sClient, openstackCreds)
	if err != nil {
		return errors.Wrap(err, "failed to delete stale PCD clusters")
	}
	return nil
}

// CreatePCDHostFromResmgrHost creates a PCDHost from resmgr Host
func CreatePCDHostFromResmgrHost(ctx context.Context, k8sClient client.Client, host resmgr.Host, openstackCreds *vjailbreakv1alpha1.OpenstackCreds) error {
	pcdHost := generatePCDHostFromResmgrHost(openstackCreds, host)
	if err := k8sClient.Create(ctx, &pcdHost); err != nil {
		return errors.Wrap(err, "failed to create PCD host")
	}
	return nil
}

// CreatePCDClusterFromResmgrCluster creates a PCDCluster from resmgr Cluster
func CreatePCDClusterFromResmgrCluster(ctx context.Context, k8sClient client.Client, cluster resmgr.Cluster, openstackCreds *vjailbreakv1alpha1.OpenstackCreds) error {
	pcdCluster, err := generatePCDClusterFromResmgrCluster(openstackCreds, cluster)
	if err != nil {
		return errors.Wrap(err, "failed to generate PCD cluster")
	}
	if err := k8sClient.Create(ctx, pcdCluster); err != nil {
		return errors.Wrap(err, "failed to create PCD cluster")
	}
	return nil
}

// CreateEntryForNoPCDCluster creates a PCDCluster for no cluster
func CreateEntryForNoPCDCluster(ctx context.Context, k8sClient client.Client, openstackCreds *vjailbreakv1alpha1.OpenstackCreds) error {
	k8sClusterName, err := getK8sClusterObjectName(constants.PCDClusterNameNoCluster, openstackCreds.Name)
	if err != nil {
		return errors.Wrap(err, "failed to convert cluster name to k8s name")
	}
	pcdCluster := vjailbreakv1alpha1.PCDCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sClusterName,
			Namespace: constants.NamespaceMigrationSystem,
			Labels: map[string]string{
				constants.OpenstackCredsLabel: openstackCreds.Name,
			},
		},
		Spec: vjailbreakv1alpha1.PCDClusterSpec{
			ClusterName:                   constants.PCDClusterNameNoCluster,
			Description:                   "",
			Hosts:                         []string{},
			VMHighAvailability:            false,
			EnableAutoResourceRebalancing: false,
			RebalancingFrequencyMins:      0,
		},
		Status: vjailbreakv1alpha1.PCDClusterStatus{
			AggregateID: 0,
			CreatedAt:   "",
			UpdatedAt:   "",
		},
	}
	if err := k8sClient.Create(ctx, &pcdCluster); err != nil {
		return errors.Wrap(err, "failed to create PCD cluster")
	}
	return nil
}

// DeleteEntryForNoPCDCluster deletes the PCDCluster for null cluster
func DeleteEntryForNoPCDCluster(ctx context.Context, k8sClient client.Client, openstackCreds *vjailbreakv1alpha1.OpenstackCreds) error {
	k8sClusterName, err := getK8sClusterObjectName(constants.PCDClusterNameNoCluster, openstackCreds.Name)
	if err != nil {
		return errors.Wrap(err, "failed to convert cluster name to k8s name")
	}
	pcdCluster := vjailbreakv1alpha1.PCDCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sClusterName,
			Namespace: constants.NamespaceMigrationSystem,
			Labels: map[string]string{
				constants.OpenstackCredsLabel: openstackCreds.Name,
			},
		},
	}
	if err := k8sClient.Delete(ctx, &pcdCluster); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to delete PCD cluster")
	}
	return nil
}

// UpdatePCDHostFromResmgrHost updates an existing PCDHost with data from resmgr Host
func UpdatePCDHostFromResmgrHost(ctx context.Context, k8sClient client.Client, host resmgr.Host, openstackCreds *vjailbreakv1alpha1.OpenstackCreds) error {
	pcdHost := generatePCDHostFromResmgrHost(openstackCreds, host)
	oldPCDHost := vjailbreakv1alpha1.PCDHost{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: host.ID, Namespace: constants.NamespaceMigrationSystem}, &oldPCDHost); err != nil {
		return errors.Wrap(err, "failed to get PCD host")
	}
	oldPCDHost.Spec = pcdHost.Spec
	oldPCDHost.Status = pcdHost.Status
	if err := k8sClient.Update(ctx, &oldPCDHost); err != nil {
		return errors.Wrap(err, "failed to update PCD host")
	}
	if err := k8sClient.Status().Update(ctx, &oldPCDHost); err != nil {
		return errors.Wrap(err, "failed to update PCD host status")
	}
	return nil
}

// UpdatePCDClusterFromResmgrCluster updates an existing PCDCluster with data from resmgr Cluster
func UpdatePCDClusterFromResmgrCluster(ctx context.Context, k8sClient client.Client, cluster resmgr.Cluster, openstackCreds *vjailbreakv1alpha1.OpenstackCreds) error {
	oldPCDCluster := vjailbreakv1alpha1.PCDCluster{}

	k8sClusterName, err := getK8sClusterObjectName(cluster.Name, openstackCreds.Name)
	if err != nil {
		return errors.Wrap(err, "failed to convert cluster name to k8s name")
	}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: k8sClusterName, Namespace: constants.NamespaceMigrationSystem}, &oldPCDCluster); err != nil {
		return errors.Wrap(err, "failed to get PCD cluster")
	}

	pcdCluster, err := generatePCDClusterFromResmgrCluster(openstackCreds, cluster)
	if err != nil {
		return errors.Wrap(err, "failed to generate PCD cluster")
	}
	oldPCDCluster.Spec = pcdCluster.Spec
	oldPCDCluster.Status = pcdCluster.Status
	if err := k8sClient.Update(ctx, &oldPCDCluster); err != nil {
		return errors.Wrap(err, "failed to update PCD cluster")
	}
	if err := k8sClient.Status().Update(ctx, &oldPCDCluster); err != nil {
		return errors.Wrap(err, "failed to update PCD cluster status")
	}
	return nil
}

func generatePCDHostFromResmgrHost(openstackCreds *vjailbreakv1alpha1.OpenstackCreds, host resmgr.Host) vjailbreakv1alpha1.PCDHost {
	// Create a new PCDHost
	interfaces := []vjailbreakv1alpha1.PCDHostInterface{}
	for name, itface := range host.Extensions.Interfaces.Data.IfaceInfo {
		// Collect all IP addresses from the interface
		ipAddresses := []string{}
		for _, iface := range itface.Ifaces {
			ipAddresses = append(ipAddresses, iface.Addr)
		}

		// Create the interface with all IPs and the MAC address
		interfaces = append(interfaces, vjailbreakv1alpha1.PCDHostInterface{
			IPAddresses: ipAddresses,
			MACAddress:  itface.MAC,
			Name:        name,
		})
	}
	pcdHost := vjailbreakv1alpha1.PCDHost{
		ObjectMeta: metav1.ObjectMeta{
			// Use the host ID as the name to ensure uniqueness
			Name:      host.ID,
			Namespace: constants.NamespaceMigrationSystem,
			// Add labels if needed
			Labels: map[string]string{
				constants.OpenstackCredsLabel: openstackCreds.Name,
			},
		},
		Spec: vjailbreakv1alpha1.PCDHostSpec{
			HostName:      host.Info.Hostname,
			HostID:        host.ID,
			HostState:     host.RoleStatus,
			RolesAssigned: host.Roles,
			OSFamily:      host.Info.OSFamily,
			Arch:          host.Info.Arch,
			OSInfo:        host.Info.OSInfo,
			Interfaces:    interfaces,
		},
		Status: vjailbreakv1alpha1.PCDHostStatus{
			Responding: host.Info.Responding,
			RoleStatus: host.RoleStatus,
		},
	}
	return pcdHost
}

func generatePCDClusterFromResmgrCluster(openstackCreds *vjailbreakv1alpha1.OpenstackCreds, cluster resmgr.Cluster) (*vjailbreakv1alpha1.PCDCluster, error) {
	k8sClusterName, err := getK8sClusterObjectName(cluster.Name, openstackCreds.Name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert cluster name to k8s name")
	}
	return &vjailbreakv1alpha1.PCDCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sClusterName,
			Namespace: constants.NamespaceMigrationSystem,
			Labels: map[string]string{
				constants.OpenstackCredsLabel: openstackCreds.Name,
			},
		},
		Spec: vjailbreakv1alpha1.PCDClusterSpec{
			ClusterName:                   cluster.Name,
			Description:                   cluster.Description,
			Hosts:                         cluster.Hostlist,
			VMHighAvailability:            cluster.VMHighAvailability.Enabled,
			EnableAutoResourceRebalancing: cluster.AutoResourceRebalancing.Enabled,
			RebalancingFrequencyMins:      cluster.AutoResourceRebalancing.RebalancingFrequencyMins,
		},
		Status: vjailbreakv1alpha1.PCDClusterStatus{
			AggregateID: cluster.AggregateID,
			CreatedAt:   cluster.CreatedAt,
			UpdatedAt:   cluster.UpdatedAt,
		},
	}, nil
}

// DeleteStalePCDHosts removes PCDHost resources that no longer exist in the upstream resmgr
func DeleteStalePCDHosts(ctx context.Context, k8sClient client.Client, openstackCreds vjailbreakv1alpha1.OpenstackCreds) error {
	OpenStackCredentials, err := GetOpenstackCredsInfo(ctx, k8sClient, openstackCreds.Name)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack credentials")
	}
	resmgrClient, err := GetResmgrClient(OpenStackCredentials)
	if err != nil {
		return errors.Wrap(err, "failed to get resmgr client")
	}
	upstreamHostList, err := resmgrClient.ListHosts(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list hosts")
	}
	upstreamHostNames := []string{}
	for _, host := range upstreamHostList {
		upstreamHostNames = append(upstreamHostNames, host.ID)
	}
	downstreamHostList, err := filterPCDHostsOnOpenstackCreds(ctx, k8sClient, openstackCreds)
	if err != nil {
		return errors.Wrap(err, "failed to filter PCD hosts")
	}
	for _, host := range downstreamHostList {
		if !containsString(upstreamHostNames, host.Spec.HostID) {
			if err := k8sClient.Delete(ctx, &vjailbreakv1alpha1.PCDHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      host.Name,
					Namespace: constants.NamespaceMigrationSystem,
				},
			}); err != nil {
				return errors.Wrap(err, "failed to delete stale PCD host")
			}
		}
	}
	return nil
}

// DeleteStalePCDClusters removes PCDCluster resources that no longer exist in the upstream resmgr
func DeleteStalePCDClusters(ctx context.Context, k8sClient client.Client, openstackCreds vjailbreakv1alpha1.OpenstackCreds) error {
	OpenStackCredentials, err := GetOpenstackCredsInfo(ctx, k8sClient, openstackCreds.Name)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack credentials")
	}
	resmgrClient, err := GetResmgrClient(OpenStackCredentials)
	if err != nil {
		return errors.Wrap(err, "failed to get resmgr client")
	}
	upstreamClusterList, err := resmgrClient.ListClusters(ctx)
	if err != nil && !strings.Contains(err.Error(), "404") {
		return errors.Wrap(err, "failed to list clusters")
	}

	upstreamClusterNames := []string{}
	for _, cluster := range upstreamClusterList {
		upstreamClusterNames = append(upstreamClusterNames, cluster.Name)
	}

	upstreamClusterNames = append(upstreamClusterNames, constants.PCDClusterNameNoCluster)

	downstreamClusterList, err := filterPCDClustersOnOpenstackCreds(ctx, k8sClient, openstackCreds)
	if err != nil {
		return errors.Wrap(err, "failed to filter PCD clusters")
	}
	for _, cluster := range downstreamClusterList {
		if !containsString(upstreamClusterNames, cluster.Spec.ClusterName) {
			if err := k8sClient.Delete(ctx, &vjailbreakv1alpha1.PCDCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cluster.Name,
					Namespace: constants.NamespaceMigrationSystem,
				},
			}); err != nil && !apierrors.IsNotFound(err) {
				return errors.Wrap(err, "failed to delete stale PCD cluster")
			}
		}
	}
	return nil
}

// nolint:unparam
func filterPCDClustersOnOpenstackCreds(ctx context.Context, k8sClient client.Client, openstackCreds vjailbreakv1alpha1.OpenstackCreds) ([]vjailbreakv1alpha1.PCDCluster, error) {
	clusterList := vjailbreakv1alpha1.PCDClusterList{}
	err := k8sClient.List(ctx, &clusterList, &client.ListOptions{
		Namespace: constants.NamespaceMigrationSystem,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			constants.OpenstackCredsLabel: openstackCreds.Name,
		}),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list PCD clusters")
	}
	return clusterList.Items, nil
}

// nolint:unparam
func filterPCDHostsOnOpenstackCreds(ctx context.Context, k8sClient client.Client, openstackCreds vjailbreakv1alpha1.OpenstackCreds) ([]vjailbreakv1alpha1.PCDHost, error) {
	hostList := vjailbreakv1alpha1.PCDHostList{}
	err := k8sClient.List(ctx, &hostList, &client.ListOptions{
		Namespace: constants.NamespaceMigrationSystem,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			constants.OpenstackCredsLabel: openstackCreds.Name,
		}),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list PCD hosts")
	}
	return hostList.Items, nil
}

// GetVMwareHostFromESXiName retrieves a VMwareHost resource based on the ESXi host name
func GetVMwareHostFromESXiName(ctx context.Context, k8sClient client.Client, esxiName string) (*vjailbreakv1alpha1.VMwareHost, error) {
	vmwareHost := &vjailbreakv1alpha1.VMwareHost{}
	esxiK8sName, err := ConvertToK8sName(esxiName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert ESXi name to k8s name")
	}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: esxiK8sName, Namespace: constants.NamespaceMigrationSystem}, vmwareHost)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get VMwareHost")
	}
	return vmwareHost, nil
}

// AssignHypervisorRoleToHost assigns the hypervisor role to a PCD host in the specified cluster
func AssignHypervisorRoleToHost(ctx context.Context, k8sClient client.Client, openstackCredsName, pcdHost, clusterName string) error {
	OpenStackCredentials, err := GetOpenstackCredsInfo(ctx, k8sClient, openstackCredsName)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack credentials")
	}
	resmgrClient, err := GetResmgrClient(OpenStackCredentials)
	if err != nil {
		return errors.Wrap(err, "failed to get resmgr client")
	}
	return resmgrClient.AssignHypervisor(ctx, pcdHost, clusterName)
}

// AssignHostConfigToHost assigns a host configuration to a PCD host
func AssignHostConfigToHost(ctx context.Context, k8sClient client.Client, openstackCredsName string, pcdHost string, hostConfigID string) error {
	OpenStackCredentials, err := GetOpenstackCredsInfo(ctx, k8sClient, openstackCredsName)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack credentials")
	}
	resmgrClient, err := GetResmgrClient(OpenStackCredentials)
	if err != nil {
		return errors.Wrap(err, "failed to get resmgr client")
	}
	return resmgrClient.AssignHostConfig(ctx, pcdHost, hostConfigID)
}

// WaitForHypervisorRoleAssignment checks if hypervisor role assignment has completed for a PCD host
func WaitForHypervisorRoleAssignment(ctx context.Context, k8sClient client.Client, openstackCredsName string, pcdHostID string) (bool, error) {
	OpenStackCredentials, err := GetOpenstackCredsInfo(ctx, k8sClient, openstackCredsName)
	if err != nil {
		return false, errors.Wrap(err, "failed to get openstack credentials")
	}
	resmgrClient, err := GetResmgrClient(OpenStackCredentials)
	if err != nil {
		return false, errors.Wrap(err, "failed to get resmgr client")
	}
	pcdHost, err := resmgrClient.GetHost(ctx, pcdHostID)
	if err != nil {
		return false, errors.Wrap(err, "failed to get host")
	}
	if !containsString(pcdHost.Roles, "pf9-ostackhost-neutron") || (pcdHost.RoleStatus != "ok" && pcdHost.RoleStatus != "converging") {
		return false, nil
	}
	return true, nil
}

// WaitforHostToShowUpOnPCD checks if a host has appeared in the PCD system
func WaitforHostToShowUpOnPCD(ctx context.Context, k8sClient client.Client, openstackCredsName string, pcdHostID string) (bool, error) {
	OpenStackCredentials, err := GetOpenstackCredsInfo(ctx, k8sClient, openstackCredsName)
	if err != nil {
		return false, errors.Wrap(err, "failed to get openstack credentials")
	}
	resmgrClient, err := GetResmgrClient(OpenStackCredentials)
	if err != nil {
		return false, errors.Wrap(err, "failed to get resmgr client")
	}
	hostList, err := resmgrClient.ListHosts(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get host")
	}
	for _, host := range hostList {
		if host.ID == pcdHostID {
			return true, nil
		}
	}
	return false, nil
}

func getK8sClusterObjectName(clusterName, openstackCredsName string) (string, error) {
	k8sClusterName, err := ConvertToK8sName(clusterName)
	if err != nil {
		return "", errors.Wrap(err, "failed to convert cluster name to k8s name")
	}
	name := fmt.Sprintf("%s-%s", k8sClusterName, openstackCredsName)
	return name[:min(len(name), 63)], nil
}
