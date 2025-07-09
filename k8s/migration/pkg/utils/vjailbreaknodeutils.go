package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/scope"
	openstackutils "github.com/platform9/vjailbreak/v2v-helper/pkg/utils"
)

// CheckAndCreateMasterNodeEntry ensures a master node entry exists and creates it if needed
func CheckAndCreateMasterNodeEntry(ctx context.Context, k3sclient client.Client, local bool) error {
	var openstackuuid string

	masterNode, err := GetMasterK8sNode(ctx, k3sclient)
	if err != nil {
		return errors.Wrap(err, "failed to get master node")
	}

	err = k3sclient.Get(ctx, client.ObjectKey{Name: masterNode.Name}, &vjailbreakv1alpha1.VjailbreakNode{})
	if err == nil {
		// VjailbreakNode already exists
		return nil
	}

	if local {
		// Local mode
		openstackuuid = "fake-openstackuuid"
	} else {
		// Controller manager is always on the master node due to pod affinity
		openstackuuid, err = openstackutils.GetCurrentInstanceUUID()
		if err != nil {
			return errors.Wrap(err, "failed to get current instance uuid")
		}
	}
	vjNode := vjailbreakv1alpha1.VjailbreakNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.VjailbreakMasterNodeName,
			Namespace: constants.NamespaceMigrationSystem,
		},
		Spec: vjailbreakv1alpha1.VjailbreakNodeSpec{
			NodeRole: constants.NodeRoleMaster,
		},
	}

	err = k3sclient.Create(ctx, &vjNode)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrap(err, "failed to create vjailbreak node")
	}

	err = k3sclient.Get(ctx, types.NamespacedName{
		Namespace: constants.NamespaceMigrationSystem,
		Name:      constants.VjailbreakMasterNodeName,
	}, &vjNode)
	if err != nil {
		return errors.Wrap(err, "failed to get vjailbreak node")
	}

	vjNode.Status.VMIP = GetNodeInternalIP(masterNode)
	vjNode.Status.Phase = constants.VjailbreakNodePhaseNodeReady
	vjNode.Status.OpenstackUUID = openstackuuid

	err = k3sclient.Status().Update(ctx, &vjNode)
	if err != nil {
		return errors.Wrap(err, "failed to update vjailbreak node status")
	}

	return nil
}

// UpdateMasterNodeImageID updates the image ID of the master node
func UpdateMasterNodeImageID(ctx context.Context, k3sclient client.Client, local bool) error {
	var imageID string
	openstackcreds, err := GetOpenstackCredsForMaster(ctx, k3sclient)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack credentials for master")
	}
	// Add finalizer to openstack creds
	err = AddFinalizerToCreds(ctx, k3sclient)
	if err != nil {
		return errors.Wrap(err, "failed to add finalizer to openstack creds")
	}
	vjNode := vjailbreakv1alpha1.VjailbreakNode{}
	err = k3sclient.Get(ctx, types.NamespacedName{
		Namespace: constants.NamespaceMigrationSystem,
		Name:      constants.VjailbreakMasterNodeName,
	}, &vjNode)
	if err != nil {
		return errors.Wrap(err, "failed to get vjailbreak node")
	}

	if local {
		// Local mode
		imageID = "fake-image-id"
	} else {
		// Controller manager is always on the master node due to pod affinity
		imageID, err = GetImageIDFromVM(ctx, k3sclient, vjNode.Status.OpenstackUUID, openstackcreds)
		if err != nil {
			return errors.Wrap(err, "failed to get image id of master node")
		}
	}

	vjNode.Spec.OpenstackImageID = imageID
	vjNode.Spec.OpenstackCreds = corev1.ObjectReference{
		Name:      openstackcreds.Name,
		Namespace: openstackcreds.Namespace,
		Kind:      openstackcreds.Kind,
	}

	err = k3sclient.Update(ctx, &vjNode)
	if err != nil {
		return errors.Wrap(err, "failed to update vjailbreak node")
	}
	return nil
}

// IsMasterNode checks if the given node is a master node
func IsMasterNode(node *corev1.Node) bool {
	_, ok := node.Labels[constants.K8sMasterNodeAnnotation]
	return ok
}

// GetAllk8sNodes retrieves all Kubernetes nodes in the cluster
func GetAllk8sNodes(ctx context.Context, k3sclient client.Client) (corev1.NodeList, error) {
	nodeList := corev1.NodeList{}
	err := k3sclient.List(ctx, &nodeList)
	if err != nil {
		return corev1.NodeList{}, err
	}
	return nodeList, nil
}

// GetNodeInternalIP retrieves the internal IP address of a node
func GetNodeInternalIP(node *corev1.Node) string {
	return node.Annotations[constants.InternalIPAnnotation]
}

// GetMasterK8sNode retrieves the Kubernetes master node
func GetMasterK8sNode(ctx context.Context, k3sclient client.Client) (*corev1.Node, error) {
	nodeList, err := GetAllk8sNodes(ctx, k3sclient)
	if err != nil {
		return nil, err
	}

	var masterNode *corev1.Node

	for i := range nodeList.Items {
		if IsMasterNode(&nodeList.Items[i]) {
			masterNode = &nodeList.Items[i]
			break
		}
	}
	if masterNode == nil {
		return nil, fmt.Errorf("node with required annotation not found")
	}
	return masterNode, nil
}

// CreateOpenstackVMForWorkerNode creates a new OpenStack VM for a worker node
func CreateOpenstackVMForWorkerNode(ctx context.Context, k3sclient client.Client, scope *scope.VjailbreakNodeScope) (string, error) {
	vjNode := scope.VjailbreakNode
	log := scope.Logger

	// Update the VjailbreakNode status
	err := k3sclient.Status().Update(ctx, vjNode)
	if err != nil {
		return "", errors.Wrap(err, "failed to update vjailbreak node status")
	}
	imageID, err := GetImageID(ctx, k3sclient)
	if err != nil {
		return "", errors.Wrap(err, "failed to get image id")
	}

	token, err := os.ReadFile(constants.K3sTokenFileLocation)
	if err != nil {
		return "", errors.Wrap(err, "failed to read k3s token file")
	}

	masterNode, err := GetMasterK8sNode(ctx, k3sclient)
	if err != nil {
		return "", errors.Wrap(err, "failed to get master node")
	}

	creds, err := GetOpenstackCredsForMaster(ctx, k3sclient)
	if err != nil {
		return "", errors.Wrap(err, "failed to get openstack creds")
	}

	openstackClients, err := GetOpenStackClients(ctx, k3sclient, creds)
	if err != nil {
		return "", errors.Wrap(err, "failed to get openstack clients")
	}

	networkIDs, err := GetCurrentInstanceNetworkInfo()
	if err != nil {
		return "", errors.Wrap(err, "failed to get network info")
	}

	// Define server creation parameters
	serverCreateOpts := servers.CreateOpts{
		Name:      vjNode.Name,
		FlavorRef: vjNode.Spec.OpenstackFlavorID,
		ImageRef:  imageID,
		Networks:  networkIDs,
		UserData: []byte(fmt.Sprintf(constants.K3sCloudInitScript,
			token[:12], constants.ENVFileLocation,
			"false", GetNodeInternalIP(masterNode),
			token)),
	}

	// Create the VM
	server, err := servers.Create(openstackClients.ComputeClient, serverCreateOpts).Extract()
	if err != nil {
		return "", errors.Wrap(err, "Failed to create server")
	}

	log.Info("Server created", "ID", server.ID)
	return server.ID, nil
}

// GetOpenstackCredsForMaster retrieves OpenStack credentials for the master node
func GetOpenstackCredsForMaster(ctx context.Context, k3sclient client.Client) (*vjailbreakv1alpha1.OpenstackCreds, error) {
	// Get master vjailbreakNode
	vjNode := vjailbreakv1alpha1.VjailbreakNode{}
	err := k3sclient.Get(ctx, client.ObjectKey{
		Name:      constants.VjailbreakMasterNodeName,
		Namespace: constants.NamespaceMigrationSystem,
	}, &vjNode)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, errors.Wrap(err, "failed to get vjailbreak node")
	}
	oscreds := &vjailbreakv1alpha1.OpenstackCreds{}
	err = k3sclient.Get(ctx, client.ObjectKey{
		Name:      vjNode.Spec.OpenstackCreds.Name,
		Namespace: constants.NamespaceMigrationSystem,
	}, oscreds)
	if err != nil && !apierrors.IsNotFound(err) {
		fmt.Printf("failed to get openstack creds associated with the vjailbreakNode. Using latest available creds : %v", err)
	}

	if err == nil {
		return oscreds, nil
	}
	// fetch the latest openstackcreds
	oscredsList := &vjailbreakv1alpha1.OpenstackCredsList{}
	err = k3sclient.List(ctx, oscredsList)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list openstack creds")
	}
	if len(oscredsList.Items) == 0 {
		return nil, errors.New("no openstack creds found")
	}
	oscreds = &oscredsList.Items[0]
	return oscreds, nil
}

// GetCurrentInstanceNetworkInfo retrieves network information for the current instance
func GetCurrentInstanceNetworkInfo() ([]servers.Network, error) {
	client := retryablehttp.NewClient()
	client.RetryMax = 5
	client.Logger = nil
	networks := []servers.Network{}
	req, err := retryablehttp.NewRequestWithContext(context.Background(), "GET",
		"http://169.254.169.254/openstack/latest/network_data.json", http.NoBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get response")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("Error closing response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	var metadata OpenStackMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	for _, Network := range metadata.Networks {
		networks = append(networks, servers.Network{
			UUID: Network.NetworkID,
		})
	}
	return networks, nil
}

// GetOpenstackVMIP retrieves the IP address of an OpenStack VM
func GetOpenstackVMIP(ctx context.Context, uuid string, k3sclient client.Client) (string, error) {
	creds, err := GetOpenstackCredsForMaster(ctx, k3sclient)
	if err != nil {
		return "", errors.Wrap(err, "failed to get openstack creds")
	}
	openstackClients, err := GetOpenStackClients(ctx, k3sclient, creds)
	if err != nil {
		return "", errors.Wrap(err, "failed to get openstack clients")
	}

	// Fetch the VM details
	server, err := servers.Get(openstackClients.ComputeClient, uuid).Extract()
	if err != nil {
		return "", errors.Wrap(err, "Failed to get server details")
	}

	// Extract IP addresses
	for _, addresses := range server.Addresses {
		addrs, ok := addresses.([]any)
		if !ok {
			return "", fmt.Errorf("addresses is not of type []any")
		}
		for _, addr := range addrs {
			ipInfo, ok := addr.(map[string]any)
			if !ok {
				continue
			}
			addrStr, ok := ipInfo["addr"].(string)
			if !ok {
				continue
			}
			return addrStr, nil
		}
	}
	return "", errors.New("failed to get vm ip")
}

// GetImageIDFromVM retrieves the image ID from a virtual machine using its UUID
func GetImageIDFromVM(ctx context.Context, k3sclient client.Client, uuid string, openstackcreds *vjailbreakv1alpha1.OpenstackCreds) (string, error) {
	openstackClients, err := GetOpenStackClients(ctx, k3sclient, openstackcreds)
	if err != nil {
		return "", errors.Wrap(err, "failed to get openstack clients")
	}

	// Fetch the VM details
	server, err := servers.Get(openstackClients.ComputeClient, uuid).Extract()
	if err != nil {
		return "", errors.Wrap(err, "Failed to get server details")
	}

	if server.Image["id"] != nil {
		fmt.Println("Image ID found", "Image ID", server.Image["id"])
	} else {
		imageID, err := GetImageIDOfVMBootFromVolume(ctx, uuid, k3sclient, openstackcreds)
		if err != nil {
			return "", errors.Wrap(err, "Failed to get image ID from VM or volume")
		}
		return imageID, nil
	}

	if imageID, ok := server.Image["id"].(string); ok {
		return imageID, nil
	}
	return "", fmt.Errorf("failed to assert image ID as string")
}

// GetImageIDOfVMBootFromVolume returns the ID of the image used to create the volume
func GetImageIDOfVMBootFromVolume(ctx context.Context, uuid string, k3sclient client.Client, openstackcreds *vjailbreakv1alpha1.OpenstackCreds) (string, error) {
	openstackClients, err := GetOpenStackClients(ctx, k3sclient, openstackcreds)
	if err != nil {
		return "", errors.Wrap(err, "failed to get OpenStack clients")
	}

	// Fetch the VM details
	server, err := servers.Get(openstackClients.ComputeClient, uuid).Extract()
	if err != nil {
		return "", errors.Wrap(err, "failed to get server details")
	}

	// Get attached volumes on that server
	attachedVolumes := server.AttachedVolumes

	// Check if the root volume is an image
	for _, volume := range attachedVolumes {
		// Get volume details
		volume, err := volumes.Get(openstackClients.BlockStorageClient, volume.ID).Extract()
		if err != nil {
			return "", errors.Wrap(err, "failed to get volume details")
		}
		fmt.Println("Volume metadata", volume.VolumeImageMetadata)
		imageID := volume.VolumeImageMetadata["image_id"]
		if imageID != "" {
			return imageID, nil
		}
	}
	return "", fmt.Errorf("no image found for the volume")
}

// ListAllFlavors retrieves a list of all available OpenStack flavors
func ListAllFlavors(ctx context.Context, k3sclient client.Client, openstackcreds *vjailbreakv1alpha1.OpenstackCreds) ([]flavors.Flavor, error) {
	openstackClients, err := GetOpenStackClients(ctx, k3sclient, openstackcreds)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get openstack clients")
	}

	// List flavors
	allPages, err := flavors.ListDetail(openstackClients.ComputeClient, nil).AllPages()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list flavors")
	}

	return flavors.ExtractFlavors(allPages)
}

// DeleteOpenstackVM deletes an OpenStack virtual machine by its UUID
func DeleteOpenstackVM(ctx context.Context, uuid string, k3sclient client.Client) error {
	creds, err := GetOpenstackCredsForMaster(ctx, k3sclient)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack creds")
	}
	openstackClients, err := GetOpenStackClients(ctx, k3sclient, creds)
	if err != nil {
		return errors.Wrap(err, "failed to get openstack clients")
	}

	// delete the VM
	err = servers.Delete(openstackClients.ComputeClient, uuid).ExtractErr()
	if err != nil && !strings.Contains(err.Error(), "404") {
		return errors.Wrap(err, "Failed to delete server")
	}
	return nil
}

// GetImageID retrieves the image ID from the Kubernetes client
func GetImageID(ctx context.Context, k3sclient client.Client) (string, error) {
	vjNode := vjailbreakv1alpha1.VjailbreakNode{}
	// Get the image ID from the vjailbreak master node
	err := k3sclient.Get(ctx, types.NamespacedName{
		Namespace: constants.NamespaceMigrationSystem,
		Name:      constants.VjailbreakMasterNodeName,
	}, &vjNode)
	if err != nil {
		return "", errors.Wrap(err, "failed to get vjailbreak node")
	}
	return vjNode.Spec.OpenstackImageID, nil
}

// GetOpenstackVMByName retrieves an OpenStack VM's UUID by its name
func GetOpenstackVMByName(ctx context.Context, name string, k3sclient client.Client) (string, error) {
	creds, err := GetOpenstackCredsForMaster(ctx, k3sclient)
	if err != nil {
		return "", errors.Wrap(err, "failed to get openstack creds")
	}
	openstackClients, err := GetOpenStackClients(ctx, k3sclient, creds)
	if err != nil {
		return "", errors.Wrap(err, "failed to get openstack clients")
	}

	listOpts := servers.ListOpts{Name: name}
	allPages, err := servers.List(openstackClients.ComputeClient, listOpts).AllPages()
	if err != nil {
		return "", errors.Wrap(err, "failed to list servers")
	}

	allServers, err := servers.ExtractServers(allPages)
	if err != nil || len(allServers) == 0 {
		return "", errors.Wrap(err, "failed to extract servers")
	}

	vmID := allServers[0].ID
	return vmID, nil
}

// ReadFileContent reads and returns the content of a file at the given path
func ReadFileContent(filePath string) ([]byte, error) {
	// Validate file path
	cleanPath := filepath.Clean(filePath)
	if !strings.HasPrefix(cleanPath, "/") {
		return nil, fmt.Errorf("invalid file path: must be absolute path")
	}
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read file")
	}

	return data, nil
}

// GetActiveMigrations retrieves a list of active migrations for a given node
func GetActiveMigrations(ctx context.Context, nodeName string, k3sclient client.Client) ([]string, error) {
	migrationList := &vjailbreakv1alpha1.MigrationList{}
	err := k3sclient.List(ctx, migrationList)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list migrations")
	}

	ignorePhases := []vjailbreakv1alpha1.VMMigrationPhase{vjailbreakv1alpha1.VMMigrationPhasePending,
		vjailbreakv1alpha1.VMMigrationPhaseFailed,
		vjailbreakv1alpha1.VMMigrationPhaseSucceeded,
		vjailbreakv1alpha1.VMMigrationPhaseUnknown,
	}

	var activeMigrations []string
	for i := range migrationList.Items {
		migration := &migrationList.Items[i]
		if migration.Status.AgentName == nodeName && !slices.Contains(ignorePhases, migration.Status.Phase) {
			activeMigrations = append(activeMigrations,
				migration.Name)
		}
	}
	return activeMigrations, nil
}

// GetInclusterClient creates and returns a Kubernetes in-cluster client
func GetInclusterClient() (client.Client, error) {
	// Create a direct Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get in-cluster config")
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vjailbreakv1alpha1.AddToScheme(scheme))
	clientset, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get in-cluster config")
	}

	return clientset, err
}

// GetNodeByName returns the node object by name
func GetNodeByName(ctx context.Context, k3sclient client.Client, nodeName string) (*corev1.Node, error) {
	node := &corev1.Node{}
	err := k3sclient.Get(ctx, client.ObjectKey{
		Name: nodeName,
	}, node)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get node")
	}
	return node, nil
}

// DeleteNodeByName deletes a Kubernetes node by its name
func DeleteNodeByName(ctx context.Context, k3sclient client.Client, nodeName string) error {
	node := &corev1.Node{}
	err := k3sclient.Get(ctx, client.ObjectKey{
		Name: nodeName,
	}, node)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to get node")
	}
	err = k3sclient.Delete(ctx, node)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to delete node")
	}
	return nil
}

// AddFinalizerToCreds adds finalizers to credentials and their associated secret
// nolint:dupl // separation of logic
func AddFinalizerToCreds(ctx context.Context, k3sclient client.Client) error {
	creds, err := GetOpenstackCredsForMaster(ctx, k3sclient)
	if err != nil {
		return errors.Wrap(err, "failed to get credentials")
	}

	secret := &corev1.Secret{}
	err = k3sclient.Get(ctx, types.NamespacedName{
		Name:      creds.Spec.SecretRef.Name,
		Namespace: constants.NamespaceMigrationSystem,
	}, secret)
	if err != nil {
		return errors.Wrap(err, "failed to get credentials from secret")
	}

	controllerutil.AddFinalizer(creds, constants.VjailbreakNodeFinalizer)
	controllerutil.AddFinalizer(secret, constants.VjailbreakNodeFinalizer)

	if err := k3sclient.Update(ctx, creds); err != nil {
		return errors.Wrap(err, "failed to update credentials with finalizer")
	}
	if err := k3sclient.Update(ctx, secret); err != nil {
		return errors.Wrap(err, "failed to update secret with finalizer")
	}
	return nil
}

// DeleteFinalizerFromCreds removes finalizers from credentials and their associated secret
// nolint:dupl // separation of logic
func DeleteFinalizerFromCreds(ctx context.Context, k3sclient client.Client) error {
	creds, err := GetOpenstackCredsForMaster(ctx, k3sclient)
	if err != nil {
		return errors.Wrap(err, "failed to get credentials")
	}

	secret := &corev1.Secret{}
	err = k3sclient.Get(ctx, types.NamespacedName{
		Name:      creds.Spec.SecretRef.Name,
		Namespace: constants.NamespaceMigrationSystem,
	}, secret)
	if err != nil {
		return errors.Wrap(err, "failed to get credentials from secret")
	}

	controllerutil.RemoveFinalizer(creds, constants.VjailbreakNodeFinalizer)
	controllerutil.RemoveFinalizer(secret, constants.VjailbreakNodeFinalizer)

	if err := k3sclient.Update(ctx, creds); err != nil {
		return errors.Wrap(err, "failed to update credentials with finalizer")
	}
	if err := k3sclient.Update(ctx, secret); err != nil {
		return errors.Wrap(err, "failed to update secret with finalizer")
	}
	return nil
}

// GetVMMigration retrieves a Migration resource for a specific VM in a rolling migration plan.
// It returns the Migration resource associated with the VM or an error if not found.
func GetVMMigration(ctx context.Context, k3sclient client.Client, vmName string, rollingMigrationPlan *vjailbreakv1alpha1.RollingMigrationPlan) (*vjailbreakv1alpha1.Migration, error) {
	vmk8sName, err := ConvertToK8sName(vmName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert vm name to k8s name")
	}
	migration := &vjailbreakv1alpha1.Migration{}
	err = k3sclient.Get(ctx, client.ObjectKey{
		Name:      MigrationNameFromVMName(vmk8sName),
		Namespace: rollingMigrationPlan.Namespace,
	}, migration)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get vm migration")
	}
	return migration, nil
}
