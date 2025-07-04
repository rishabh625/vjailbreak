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

	"github.com/platform9/vjailbreak/v2v-helper/migrate"
	"github.com/platform9/vjailbreak/v2v-helper/vm"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
)

// RdmDiskReconciler reconciles a RdmDisk object
type RdmDiskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rdmdisks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rdmdisks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rdmdisks/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RdmDisk object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *RdmDiskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get the RdmDisk resource
	rdmDisk := &vjailbreakv1alpha1.RdmDisk{}
	if err := r.Get(ctx, req.NamespacedName, rdmDisk); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch RdmDisk")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Handle different phases
	switch rdmDisk.Status.Phase {
	case "Created":
		// Validate the RDM disk specifications
		if err := validateRdmDiskFields(rdmDisk); err != nil {
			log.Error(err, "validation failed")
			rdmDisk.Status.Phase = "Error"
			startCondition := metav1.Condition{
				Type:    "ValidationFailed",
				Status:  metav1.ConditionTrue,
				Reason:  "FieldValidationFailed",
				Message: err.Error(),
			}
			meta.SetStatusCondition(&rdmDisk.Status.Conditions, startCondition)
			if err := r.Status().Update(ctx, rdmDisk); err != nil {
				log.Error(err, "unable to update RdmDisk status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if rdmDisk.Spec.ImportToCinder {
			// All validations passed, move to Managing phase
			rdmDisk.Status.Phase = "Managing"
			startCondition := metav1.Condition{
				Type:    "MigrationStarted",
				Status:  metav1.ConditionTrue,
				Reason:  "ValidationPassed",
				Message: "All required fields validated, starting migration",
			}
			meta.SetStatusCondition(&rdmDisk.Status.Conditions, startCondition)
			if err := r.Status().Update(ctx, rdmDisk); err != nil {
				log.Error(err, "unable to update RdmDisk status")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{Requeue: true}, nil

	case "Managing":
		if rdmDisk.Spec.ImportToCinder && rdmDisk.Status.CinderVolumeID == "" {

			// Create the RDM disk object with required fields
			rdmDiskObj := vm.RDMDisk{
				VolumeRef:         rdmDisk.Spec.VolumeRef.Source,
				CinderBackendPool: rdmDisk.Spec.VolumeRef.CinderBackendPool,
				VolumeType:        rdmDisk.Spec.VolumeRef.VolumeType,
			}

			migrationObj := &migrate.Migrate{}
			volumeID, err := migrationObj.CinderManage(rdmDiskObj)
			if err != nil {
				log.Error(err, "Failed to manage RDM disk in Cinder")
				rdmDisk.Status.Phase = "Error"
				failureCondition := metav1.Condition{
					Type:    "MigrationFailed",
					Status:  metav1.ConditionTrue,
					Reason:  "CinderManageFailed",
					Message: err.Error(),
				}
				meta.SetStatusCondition(&rdmDisk.Status.Conditions, failureCondition)
				if updateErr := r.Status().Update(ctx, rdmDisk); updateErr != nil {
					log.Error(updateErr, "unable to update RdmDisk status")
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{}, err
			}

			// Update status with the volume ID in CinderReference
			rdmDisk.Status.Phase = "Managed"
			rdmDisk.Status.CinderVolumeID = volumeID
			successCondition := metav1.Condition{
				Type:    "MigrationSucceeded",
				Status:  metav1.ConditionTrue,
				Reason:  "CinderManageSucceeded",
				Message: "Successfully imported RDM disk to Cinder",
			}
			meta.SetStatusCondition(&rdmDisk.Status.Conditions, successCondition)
			if err := r.Status().Update(ctx, rdmDisk); err != nil {
				log.Error(err, "unable to update RdmDisk status with volume ID")
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
func (r *RdmDiskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vjailbreakv1alpha1.RdmDisk{}).
		Named("rdmdisk").
		Complete(r)
}

// validateRdmDiskFields validates all required fields for migration
func validateRdmDiskFields(rdmDisk *vjailbreakv1alpha1.RdmDisk) error {
	if len(rdmDisk.Spec.VolumeRef.Source) == 0 {
		return fmt.Errorf("volumeRef.source is required")
	}

	if rdmDisk.Spec.VolumeRef.CinderBackendPool == "" {
		return fmt.Errorf("volumeRef.cinderBackendPool is required")
	}

	if rdmDisk.Spec.VolumeRef.VolumeType == "" {
		return fmt.Errorf("volumeRef.volumeType is required")
	}

	return nil
}
