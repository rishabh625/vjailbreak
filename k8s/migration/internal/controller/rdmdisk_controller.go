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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/utils/migrateutils"
	"github.com/platform9/vjailbreak/v2v-helper/vm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	constants "github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	utils "github.com/platform9/vjailbreak/k8s/migration/pkg/utils"
)

// RDMDiskReconciler reconciles a RDMDisk object
type RDMDiskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	// RetryInterval a constant for the validation retry interval
	RetryInterval = 2 * time.Minute
	// ConditionValidationFailed is the condition type for migration to cinder validation failure
	ConditionValidationFailed = "RDMDiskValidationFailed"
	// ConditionValidationPassed is the condition type for migration to cinder validation passed
	ConditionValidationPassed = "RDMDiskValidationPassed" // #nosec G101
	// ReasonRequiredFieldsMissing ConditionMigrationStarted is the condition type for migration to cinder
	ReasonRequiredFieldsMissing = "RDMDiskRequiredFieldsMissing"
	// ReasonValidatedSpecs ConditionMigrationStarted is the condition type for migration to cinder
	ReasonValidatedSpecs = "ValidatedRDMDiskSpecs"
	// MigrationStarted ConditionMigrationStarted is the condition type for migration to cinder
	MigrationStarted = "RDMDiskMigrationStarted"
	// MigrationSucceeded ConditionMigrationStarted is the condition type for migration to cinder
	MigrationSucceeded = "RDMDiskMigrationSucceeded"
	// MigrationFailed ConditionMigrationStarted is the condition type for migration to cinder
	MigrationFailed = "RDMDiskMigrationFailed"
	// blockStorageAPIVersion is the version of the OpenStack Block Storage API to use
	blockStorageAPIVersion = "3.8"
	// RDMPhaseAvailable is the phase for RDMDisk when it is available to migrate
	RDMPhaseAvailable = "Available"
	// RDMPhaseManaging is the phase for RDMDisk when it is being managed
	RDMPhaseManaging = "Managing"
	// RDMPhaseManaged is the phase for RDMDisk when it has been successfully managed
	RDMPhaseManaged = "Managed"
	// RDMPhaseError is the phase for RDMDisk when there is an error
	RDMPhaseError = "Error"
)

// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rdmdisks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rdmdisks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rdmdisks/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Here it is specific to RDMDisk objects.
func (r *RDMDiskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctxlog := log.FromContext(ctx).WithName(constants.RDMDiskControllerName)

	// Get the RDMDisk resource
	rdmDisk := &vjailbreakv1alpha1.RDMDisk{}
	if err := r.Get(ctx, req.NamespacedName, rdmDisk); err != nil {
		if client.IgnoreNotFound(err) != nil {
			ctxlog.Error(err, "unable to fetch RDMDisk")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	switch rdmDisk.Status.Phase {
	case "":
		return r.handleInitialPhase(ctx, rdmDisk, ctxlog)

	case RDMPhaseAvailable:
		return r.handleAvailablePhase(ctx, rdmDisk, ctxlog)

	case RDMPhaseManaging:
		return r.handleManagingPhase(ctx, req, rdmDisk, ctxlog)
	case RDMPhaseManaged:
		ctxlog.Info("RDMDisk is already managed", "CinderVolumeID", rdmDisk.Status.CinderVolumeID)
		return ctrl.Result{}, nil
	case RDMPhaseError:
		ctxlog.Info("RDMDisk is in error state, skipping reconciliation, to trigger a reconciliation re create rdm disk custom resource", "CinderVolumeID", rdmDisk.Status.CinderVolumeID)
		return ctrl.Result{}, nil
	default:
		ctxlog.Info("Unknown phase", "phase", rdmDisk.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// handleInitialPhase handles the initial phase of RDMDisk reconciliation
func (r *RDMDiskReconciler) handleInitialPhase(ctx context.Context, rdmDisk *vjailbreakv1alpha1.RDMDisk, log logr.Logger) (ctrl.Result, error) {
	mostRecentValidationFailedCondition := getMostRecentValidationFailedCondition(rdmDisk.Status.Conditions)
	if mostRecentValidationFailedCondition != nil {
		lastTransitionTime := mostRecentValidationFailedCondition.LastTransitionTime.Time
		timeSinceLastTransition := time.Since(lastTransitionTime)
		if timeSinceLastTransition < RetryInterval {
			log.Info("Skipping validation as a ValidationFailed condition was set less than 2 minutes ago",
				"LastReason", mostRecentValidationFailedCondition.Reason)
			requeueAfter := RetryInterval - timeSinceLastTransition
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}
	if err := ValidateRDMDiskFields(rdmDisk); err != nil {
		log.Error(err, "validation failed")
		updateStatusCondition(rdmDisk, metav1.Condition{
			Type:    ConditionValidationFailed,
			Status:  metav1.ConditionTrue,
			Reason:  ReasonRequiredFieldsMissing,
			Message: err.Error(),
		})
		if err := r.Status().Update(ctx, rdmDisk); err != nil {
			log.Error(err, "unable to update RDMDisk status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	rdmDisk.Status.Phase = RDMPhaseAvailable
	updateStatusCondition(rdmDisk, metav1.Condition{
		Type:    ConditionValidationPassed,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonValidatedSpecs,
		Message: "All required fields are present and valid",
	})
	if err := r.Status().Update(ctx, rdmDisk); err != nil {
		log.Error(err, "unable to update RDMDisk status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: RetryInterval}, nil
}

// handleAvailablePhase handles the Available phase of RDMDisk reconciliation
func (r *RDMDiskReconciler) handleAvailablePhase(ctx context.Context, rdmDisk *vjailbreakv1alpha1.RDMDisk, log logr.Logger) (ctrl.Result, error) {
	if rdmDisk.Spec.ImportToCinder {
		rdmDisk.Status.Phase = RDMPhaseManaging
		updateStatusCondition(rdmDisk, metav1.Condition{
			Type:    MigrationStarted,
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
}

// handleManagingPhase handles the managing phase of RDMDisk reconciliation
func (r *RDMDiskReconciler) handleManagingPhase(ctx context.Context, req ctrl.Request, rdmDisk *vjailbreakv1alpha1.RDMDisk, log logr.Logger) (ctrl.Result, error) {
	if rdmDisk.Spec.ImportToCinder && rdmDisk.Status.CinderVolumeID == "" {
		rdmDiskObj := vm.RDMDisk{
			DiskName:          rdmDisk.Name,
			VolumeRef:         rdmDisk.Spec.OpenstackVolumeRef.VolumeRef,
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
				log.Info("Resource not found, likely deleted", "openstackcreds", openstackCredsName)
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to get OpenstackCreds resource", "openstackcreds", openstackCredsName)
			return ctrl.Result{}, err
		}
		log.V(1).Info("Retrieved OpenstackCreds resource", "openstackcreds", openstackCredsName, "resourceVersion", openstackcreds.ResourceVersion)
		openstackClient, err := utils.GetOpenStackClients(ctx, r.Client, openstackcreds)
		if err != nil {
			return ctrl.Result{}, handleError(ctx, r.Client, rdmDisk, "Error", "OpenStackClientCreationFailed", "Failed to create OpenStack client from options", err)
		}
		osclient := migrateutils.OpenStackClients{
			BlockStorageClient: openstackClient.BlockStorageClient,
			ComputeClient:      openstackClient.ComputeClient,
			NetworkingClient:   openstackClient.NetworkingClient,
		}
		volumeID, err := utils.ImportLUNToCinder(ctx, &osclient, rdmDiskObj, blockStorageAPIVersion)
		if err != nil {
			return ctrl.Result{}, handleError(ctx, r.Client, rdmDisk, "Error", MigrationFailed, "FailedToImportLUNToCinder", err)
		}
		rdmDisk.Status.Phase = RDMPhaseManaged
		rdmDisk.Status.CinderVolumeID = volumeID
		updateStatusCondition(rdmDisk, metav1.Condition{
			Type:    MigrationSucceeded,
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
	return ctrl.Result{}, nil
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
	if len(rdmDisk.Spec.OpenstackVolumeRef.VolumeRef) == 0 {
		return fmt.Errorf("OpenstackVolumeRef.source is required")
	}

	if rdmDisk.Spec.OpenstackVolumeRef.CinderBackendPool == "" {
		return fmt.Errorf("OpenstackVolumeRef.cinderBackendPool is required")
	}

	if rdmDisk.Spec.OpenstackVolumeRef.VolumeType == "" {
		return fmt.Errorf("OpenstackVolumeRef.volumeType is required")
	}
	if rdmDisk.Spec.DiskName == "" {
		return fmt.Errorf("DiskName is required")
	}
	return nil
}

// handleError updates the RDMDisk status with the provided error details and logs the error.
func handleError(ctx context.Context, r client.Client, rdmDisk *vjailbreakv1alpha1.RDMDisk, phase string, conditionType string, reason string, err error) error {
	log := log.FromContext(ctx)
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

func getMostRecentValidationFailedCondition(conditions []metav1.Condition) *metav1.Condition {
	var mostRecent *metav1.Condition
	for i := range conditions {
		cond := conditions[i]
		if cond.Type == ConditionValidationFailed && cond.Status == metav1.ConditionTrue {
			if mostRecent == nil || cond.LastTransitionTime.After(mostRecent.LastTransitionTime.Time) {
				mostRecent = &cond
			}
		}
	}
	return mostRecent
}

// Refactor status condition updates into a helper function
func updateStatusCondition(rdmDisk *vjailbreakv1alpha1.RDMDisk, condition metav1.Condition) {
	meta.SetStatusCondition(&rdmDisk.Status.Conditions, condition)
}
