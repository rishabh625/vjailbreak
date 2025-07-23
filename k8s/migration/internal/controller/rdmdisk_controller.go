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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	v2vhelperutils "github.com/platform9/vjailbreak/v2v-helper/pkg/utils"
	"github.com/platform9/vjailbreak/v2v-helper/vm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	constants "github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	utils "github.com/platform9/vjailbreak/k8s/migration/pkg/utils"
)

// RDMDiskReconciler reconciles a RDMDisk object
type RDMDiskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rdmdisks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rdmdisks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rdmdisks/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RDMDisk object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *RDMDiskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get the RDMDisk resource
	rdmDisk := &vjailbreakv1alpha1.RDMDisk{}
	if err := r.Get(ctx, req.NamespacedName, rdmDisk); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch RDMDisk")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Updated logic for handling phases and conditions
	switch rdmDisk.Status.Phase {
	case "":
		// Initial phase, validate the RDM disk specifications
		if err := ValidateRDMDiskFields(rdmDisk); err != nil {
			log.Error(err, "validation failed")
			rdmDisk.Status.Phase = "Error"
			meta.SetStatusCondition(&rdmDisk.Status.Conditions, metav1.Condition{
				Type:    "ValidationFailed",
				Status:  metav1.ConditionTrue,
				Reason:  "RequiredFieldsMissing",
				Message: err.Error(),
			})
			if err := r.Status().Update(ctx, rdmDisk); err != nil {
				log.Error(err, "unable to update RDMDisk status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		// Validation passed, move to Pending phase
		rdmDisk.Status.Phase = "Pending"
		meta.SetStatusCondition(&rdmDisk.Status.Conditions, metav1.Condition{
			Type:    "Validated",
			Status:  metav1.ConditionTrue,
			Reason:  "ValidationPassed",
			Message: "All required fields validated",
		})
		if err := r.Status().Update(ctx, rdmDisk); err != nil {
			log.Error(err, "unable to update RDMDisk status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil

	case "Pending":
		// Check if ImportToCinder is true, move to Managing phase
		if rdmDisk.Spec.ImportToCinder {
			rdmDisk.Status.Phase = "Managing"
			meta.SetStatusCondition(&rdmDisk.Status.Conditions, metav1.Condition{
				Type:    "MigrationStarted",
				Status:  metav1.ConditionTrue,
				Reason:  "ImportToCinderEnabled",
				Message: "Starting migration to Cinder Importing LUN",
			})
			if err := r.Status().Update(ctx, rdmDisk); err != nil {
				log.Error(err, "unable to update RDMDisk status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, nil

	case "Managing":
		ctxlog := log.WithName(constants.RDMDiskControllerName)

		if rdmDisk.Spec.ImportToCinder && rdmDisk.Status.CinderVolumeID == "" {
			// Create the RDM disk object with required fields
			rdmDiskObj := vm.RDMDisk{
				DiskName:          rdmDisk.Name,
				VolumeRef:         rdmDisk.Spec.OpenstackVolumeRef.Source,
				CinderBackendPool: rdmDisk.Spec.OpenstackVolumeRef.CinderBackendPool,
				VolumeType:        rdmDisk.Spec.OpenstackVolumeRef.VolumeType,
			}
			openstackcreds := &vjailbreakv1alpha1.OpenstackCreds{}
			openstackCredsName := client.ObjectKey{
				Namespace: req.Namespace,
				Name:      rdmDisk.Spec.OpenstackVolumeRef.OpenstackCreds,
			}
			if err := r.Get(ctx, openstackCredsName, openstackcreds); err != nil {
				if apierrors.IsNotFound(err) {
					ctxlog.Info("Resource not found, likely deleted", "openstackcreds", openstackCredsName)
					return ctrl.Result{}, nil
				}
				ctxlog.Error(err, "Failed to get OpenstackCreds resource", "openstackcreds", openstackCredsName)
				return ctrl.Result{}, err
			}
			ctxlog.V(1).Info("Retrieved OpenstackCreds resource", "openstackcreds", openstackCredsName, "resourceVersion", openstackcreds.ResourceVersion)
			openstackClient, err := utils.GetOpenStackClients(ctx, r.Client, openstackcreds)
			if err != nil {
				return ctrl.Result{}, handleError(ctx, r.Client, rdmDisk, "Error", "OpenStackClientCreationFailed", "Failed to create OpenStack client from options", err)
			}
			osclient := v2vhelperutils.OpenStackClients{
				BlockStorageClient: openstackClient.BlockStorageClient,
				ComputeClient:      openstackClient.ComputeClient,
				NetworkingClient:   openstackClient.NetworkingClient,
			}
			volumeID, err := ImportLUNToCinder(ctx, &osclient, rdmDiskObj)
			if err != nil {
				return ctrl.Result{}, handleError(ctx, r.Client, rdmDisk, "Error", "CinderManageFailed", "Failed to manage RDM disk in Cinder", err)
			}
			// Update status with the volume ID in CinderReference
			rdmDisk.Status.Phase = "Managed"
			rdmDisk.Status.CinderVolumeID = volumeID
			meta.SetStatusCondition(&rdmDisk.Status.Conditions, metav1.Condition{
				Type:    "MigrationSucceeded",
				Status:  metav1.ConditionTrue,
				Reason:  "CinderManageSucceeded",
				Message: "Successfully imported RDM disk to Cinder",
			})
			if err := r.Status().Update(ctx, rdmDisk); err != nil {
				log.Error(err, "unable to update RDMDisk status with volume ID")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil

	default:
		log.Info("Unknown phase", "phase", rdmDisk.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *RDMDiskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vjailbreakv1alpha1.RDMDisk{}).
		Named("rdmdisk").
		Complete(r)
}

// ValidateRDMDiskFields validates all required fields for migration
func ValidateRDMDiskFields(rdmDisk *vjailbreakv1alpha1.RDMDisk) error {
	if len(rdmDisk.Spec.OpenstackVolumeRef.Source) == 0 {
		return fmt.Errorf("OpenstackVolumeRef.source is required")
	}

	if rdmDisk.Spec.OpenstackVolumeRef.CinderBackendPool == "" {
		return fmt.Errorf("OpenstackVolumeRef.cinderBackendPool is required")
	}

	if rdmDisk.Spec.OpenstackVolumeRef.VolumeType == "" {
		return fmt.Errorf("OpenstackVolumeRef.volumeType is required")
	}
	return nil
}

// handleError updates the RDMDisk status with the provided error details and logs the error.
func handleError(ctx context.Context, r client.Client, rdmDisk *vjailbreakv1alpha1.RDMDisk, phase string, conditionType string, reason string, err error) error {
	log := logf.FromContext(ctx)
	log.Error(err, fmt.Sprintf("Failed during phase: %s", phase))
	rdmDisk.Status.Phase = phase
	failureCondition := metav1.Condition{
		Type:    conditionType,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: err.Error(),
	}
	meta.SetStatusCondition(&rdmDisk.Status.Conditions, failureCondition)
	if updateErr := r.Status().Update(ctx, rdmDisk); updateErr != nil {
		log.Error(updateErr, "unable to update RDMDisk status")
		return updateErr
	}
	return err
}

// ImportLUNToCinder imports a LUN into OpenStack Cinder and returns the volume ID.
func ImportLUNToCinder(ctx context.Context, openstackClient *v2vhelperutils.OpenStackClients, rdmDisk vm.RDMDisk) (string, error) {
	ctxlog := logf.FromContext(ctx)
	ctxlog.Info(fmt.Sprintf("Importing LUN: %s", rdmDisk.DiskName))
	volume, err := ExecuteVolumeManageRequest(ctx, rdmDisk, openstackClient, "volume 3.8")
	if err != nil || volume == nil {
		return "", fmt.Errorf("failed to import LUN: %s", err)
	} else if volume.ID == "" {
		return "", fmt.Errorf("failed to import LUN: received empty volume ID")
	}
	ctxlog.Info(fmt.Sprintf("LUN imported successfully, waiting for volume %s to become available", volume.ID))
	// Wait for the volume to become available
	err = openstackClient.WaitForVolume(volume.ID)
	if err != nil {
		return "", fmt.Errorf("failed to wait for volume to become available: %s", err)
	}
	ctxlog.Info(fmt.Sprintf("Volume %s is now available", volume.ID))
	return volume.ID, nil
}

// BuildVolumeManagePayload builds the request payload for manage volume.
func BuildVolumeManagePayload(rdmDisk vm.RDMDisk) (map[string]interface{}, error) {
	// Validate required fields
	if rdmDisk.DiskName == "" {
		return nil, fmt.Errorf("disk name cannot be empty")
	}
	if rdmDisk.CinderBackendPool == "" {
		return nil, fmt.Errorf("cinder backend pool cannot be empty")
	}
	if rdmDisk.VolumeType == "" {
		return nil, fmt.Errorf("volume type cannot be empty")
	}
	if len(rdmDisk.VolumeRef) == 0 {
		return nil, fmt.Errorf("volume reference cannot be empty")
	}

	var key, value string
	for k, rm := range rdmDisk.VolumeRef {
		key = k
		value = rm
	}
	payload := map[string]interface{}{
		"volume": map[string]interface{}{
			"host": rdmDisk.CinderBackendPool,
			"ref": map[string]string{
				key: value,
			},
			"name":              rdmDisk.DiskName,
			"volume_type":       rdmDisk.VolumeType,
			"description":       fmt.Sprintf("Volume for %s", rdmDisk.DiskName),
			"bootable":          false,
			"availability_zone": nil,
		},
	}
	return payload, nil
}

// ExecuteVolumeManageRequest triggers the volume manage request and returns volume.
func ExecuteVolumeManageRequest(ctx context.Context, rdmDisk vm.RDMDisk, osclient *v2vhelperutils.OpenStackClients, openstackAPIVersion string) (*volumes.Volume, error) {
	body, err := BuildVolumeManagePayload(rdmDisk)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}

	response, err := osclient.BlockStorageClient.Post(osclient.BlockStorageClient.ServiceURL("manageable_volumes"), body, &result, &gophercloud.RequestOpts{
		OkCodes:     []int{http.StatusAccepted},
		MoreHeaders: map[string]string{"OpenStack-API-Version": openstackAPIVersion},
	})
	if err != nil {
		return nil, err
	}
	// Add error handling for response.Body.Close()
	if response != nil && response.Body != nil {
		defer func() {
			if err := response.Body.Close(); err != nil {
				logf.FromContext(ctx).Error(err, "failed to close response body")
			}
		}()
	}
	// Add error handling for type assertion
	volumeMap, ok := result["volume"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to assert type for volume map")
	}
	// Convert volume map to JSON
	volumeJSON, err := json.Marshal(volumeMap)
	if err != nil {
		return nil, err
	}
	// Unmarshal JSON into your struct
	var v volumes.Volume
	if err := json.Unmarshal(volumeJSON, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
