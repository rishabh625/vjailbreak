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

	"github.com/pkg/errors"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/scope"
	utils "github.com/platform9/vjailbreak/k8s/migration/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	constants "github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// ClusterMigrationReconciler reconciles a ClusterMigration object
type ClusterMigrationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=clustermigrations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=clustermigrations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=clustermigrations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ClusterMigrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctxlog := log.FromContext(ctx).WithName(constants.ClusterMigrationControllerName)
	ctxlog.Info("Starting reconciliation", "clustermigration", req.NamespacedName)

	clusterMigration := &vjailbreakv1alpha1.ClusterMigration{}
	if err := r.Get(ctx, req.NamespacedName, clusterMigration); err != nil {
		if apierrors.IsNotFound(err) {
			ctxlog.Info("Resource not found, likely deleted", "clustermigration", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		ctxlog.Error(err, "Failed to get ClusterMigration resource", "clustermigration", req.NamespacedName)
		return ctrl.Result{}, err
	}
	ctxlog.V(1).Info("Retrieved ClusterMigration resource", "clustermigration", req.NamespacedName, "resourceVersion", clusterMigration.ResourceVersion)

	rollingMigrationPlan := &vjailbreakv1alpha1.RollingMigrationPlan{}
	rollingMigrationPlanKey := client.ObjectKey{Namespace: clusterMigration.Namespace, Name: clusterMigration.Spec.RollingMigrationPlanRef.Name}
	ctxlog.V(1).Info("Fetching referenced RollingMigrationPlan", "rollingMigrationPlan", rollingMigrationPlanKey)
	if err := r.Get(ctx, rollingMigrationPlanKey, rollingMigrationPlan); err != nil {
		if apierrors.IsNotFound(err) {
			ctxlog.Info("Referenced RollingMigrationPlan not found", "rollingMigrationPlan", rollingMigrationPlanKey)
			return ctrl.Result{}, nil
		}
		ctxlog.Error(err, "Failed to get RollingMigrationPlan", "rollingMigrationPlan", rollingMigrationPlanKey)
		return ctrl.Result{}, err
	}

	scope, err := scope.NewClusterMigrationScope(scope.ClusterMigrationScopeParams{
		Logger:               ctxlog,
		Client:               r.Client,
		ClusterMigration:     clusterMigration,
		RollingMigrationPlan: rollingMigrationPlan,
	})
	if err != nil {
		ctxlog.Error(err, "Failed to create ClusterMigrationScope")
		return ctrl.Result{}, err
	}

	// Always close the scope when exiting this function such that we can persist any ClusterMigration changes.
	defer func() {
		if err := scope.Close(); err != nil && reterr == nil {
			ctxlog.Error(err, "Failed to close ClusterMigrationScope")
			reterr = err
		}
	}()

	if !clusterMigration.DeletionTimestamp.IsZero() {
		ctxlog.Info("Resource is being deleted, reconciling deletion", "clustermigration", req.NamespacedName)
		return r.reconcileDelete(ctx, scope)
	}

	ctxlog.Info("Reconciling normal state", "clustermigration", req.NamespacedName)
	return r.reconcileNormal(ctx, scope)
}

func (r *ClusterMigrationReconciler) reconcileNormal(ctx context.Context, scope *scope.ClusterMigrationScope) (ctrl.Result, error) {
	log := scope.Logger
	clusterMigration := scope.ClusterMigration
	log.Info("Starting normal reconciliation", "clustermigration", clusterMigration.Name, "namespace", clusterMigration.Namespace)

	controllerutil.AddFinalizer(clusterMigration, constants.ClusterMigrationFinalizer)
	if err := scope.Close(); err != nil {
		log.Error(err, "Failed to close ClusterMigrationScope")
		return ctrl.Result{}, errors.Wrap(err, "failed to close cluster migration scope")
	}
	switch clusterMigration.Status.Phase {
	case "":
		log.Info("Initializing ClusterMigration phase", "newPhase", vjailbreakv1alpha1.ClusterMigrationPhasePending)
		clusterMigration.Status.Phase = vjailbreakv1alpha1.ClusterMigrationPhasePending
	case vjailbreakv1alpha1.ClusterMigrationPhaseSucceeded:
		log.Info("Cluster migration already succeeded")
		return ctrl.Result{}, nil
	case vjailbreakv1alpha1.ClusterMigrationPhaseFailed:
		log.Info("Cluster migration already failed")
		return ctrl.Result{}, nil
	}

	if utils.IsClusterMigrationPaused(ctx, clusterMigration.Name, scope.Client) {
		clusterMigration.Status.Phase = vjailbreakv1alpha1.ClusterMigrationPhasePaused
		if err := scope.Client.Status().Update(ctx, clusterMigration); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update cluster migration status")
		}
		log.Info(fmt.Sprintf("Cluster migration %s is paused, skipping reconciliation", clusterMigration.Name))
		return ctrl.Result{}, nil
	}

	// count successful esxiMigrations, we want to trigger vm migrations
	// only if one or more esxi migrations are successful
	successfulESXiMigrations, err := countSuccessfulESXIMigrations(ctx, scope)
	if err != nil {
		log.Error(err, "Failed to count successful ESXi migrations")
		return ctrl.Result{}, errors.Wrap(err, "failed to count successful esxi migrations")
	}

	log.Info("Counted successful ESXi migrations", "count", successfulESXiMigrations)
	if successfulESXiMigrations >= 1 {
		err = handleVMMigrations(ctx, scope)
		if err != nil {
			log.Error(err, "Failed to handle VM migrations")
			return ctrl.Result{}, errors.Wrap(err, "failed to handle vm migrations")
		}
	}

	for i, esxi := range clusterMigration.Spec.ESXIMigrationSequence {
		esxiMigration, err := utils.GetESXIMigration(ctx, scope.Client, esxi, scope.RollingMigrationPlan)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("ESXIMigration not found, creating new one", "esxiName", esxi)
				if esxiMigration, err = utils.CreateESXIMigration(ctx, scope, esxi); err != nil {
					log.Error(err, "Failed to create ESXIMigration", "esxiName", esxi)
					return ctrl.Result{}, errors.Wrap(err, "failed to create esxi migration")
				}
				log.Info("Successfully created ESXIMigration", "esxiName", esxi, "esximigration", esxiMigration.Name)
			} else {
				log.Error(err, "Failed to get ESXIMigration", "esxiName", esxi)
				return ctrl.Result{}, errors.Wrap(err, "failed to get esxi migration")
			}
		}
		log.Info("Retrieved ESXIMigration", "esxiName", esxi, "esximigration", esxiMigration.Name, "phase", esxiMigration.Status.Phase)

		switch esxiMigration.Status.Phase {
		case vjailbreakv1alpha1.ESXIMigrationPhaseFailed:
			log.Info("ESXIMigration failed, updating ClusterMigration status", "esxiName", esxi, "message", esxiMigration.Status.Message)
			err = r.UpdateClusterMigrationStatus(ctx, scope, vjailbreakv1alpha1.ClusterMigrationPhaseFailed, esxiMigration.Status.Message, esxi)
			if err != nil {
				log.Error(err, "Failed to update ClusterMigration status", "desiredPhase", vjailbreakv1alpha1.ClusterMigrationPhaseFailed)
				return ctrl.Result{}, errors.Wrap(err, "failed to update cluster migration status")
			}
			log.Info("Successfully updated ClusterMigration status to failed")
			return ctrl.Result{}, nil
		case vjailbreakv1alpha1.ESXIMigrationPhaseSucceeded:
			if i == len(clusterMigration.Spec.ESXIMigrationSequence)-1 {
				if i == 0 {
					err = handleVMMigrations(ctx, scope)
					if err != nil {
						log.Error(err, "Failed to handle VM migrations")
						return ctrl.Result{}, errors.Wrap(err, "failed to handle vm migrations")
					}
				}
				log.Info("All ESXIMigrations succeeded, updating ClusterMigration status to succeeded")
				err = r.UpdateClusterMigrationStatus(ctx, scope, vjailbreakv1alpha1.ClusterMigrationPhaseSucceeded, "All ESXIMigrations succeeded", "")
				if err != nil {
					log.Error(err, "Failed to update ClusterMigration status", "desiredPhase", vjailbreakv1alpha1.ClusterMigrationPhaseSucceeded)
					return ctrl.Result{}, errors.Wrap(err, "failed to update cluster migration status")
				}
				log.Info("Successfully updated ClusterMigration status to succeeded")
				return ctrl.Result{}, nil
			}
			log.Info("ESXIMigration succeeded, continuing to next ESXi", "esxiName", esxi)
			continue
		case vjailbreakv1alpha1.ESXIMigrationPhaseWaiting,
			vjailbreakv1alpha1.ESXIMigrationPhaseWaitingForVMsToBeMoved,
			vjailbreakv1alpha1.ESXIMigrationPhaseAssigningRole,
			vjailbreakv1alpha1.ESXIMigrationPhaseConvertingToPCDHost,
			vjailbreakv1alpha1.ESXIMigrationPhaseCordoned,
			vjailbreakv1alpha1.ESXIMigrationPhaseInMaintenanceMode:
			log.Info("ESXIMigration is running, updating ClusterMigration status", "esxiName", esxi)
			err = r.UpdateClusterMigrationStatus(ctx, scope, vjailbreakv1alpha1.ClusterMigrationPhaseRunning, esxiMigration.Status.Message, esxi)
			if err != nil {
				log.Error(err, "Failed to update ClusterMigration status", "desiredPhase", vjailbreakv1alpha1.ClusterMigrationPhaseRunning)
				return ctrl.Result{}, errors.Wrap(err, "failed to update cluster migration status")
			}
			log.Info("Successfully updated ClusterMigration status to running")
			log.Info("Requeuing ClusterMigration for further processing", "requeueAfter", "1m")
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}

		log.Info("ESXIMigration is in another state, updating ClusterMigration status to pending", "esxiName", esxi, "esxiPhase", esxiMigration.Status.Phase)
		err = r.UpdateClusterMigrationStatus(ctx, scope, vjailbreakv1alpha1.ClusterMigrationPhasePending, esxiMigration.Status.Message, esxi)
		if err != nil {
			log.Error(err, "Failed to update ClusterMigration status", "desiredPhase", vjailbreakv1alpha1.ClusterMigrationPhasePending)
			return ctrl.Result{}, errors.Wrap(err, "failed to update cluster migration status")
		}
		log.Info("Successfully updated ClusterMigration status to pending")
		log.Info("Requeuing ClusterMigration for further processing", "requeueAfter", "1m")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}
	return ctrl.Result{}, nil
}

func (r *ClusterMigrationReconciler) reconcileDelete(ctx context.Context, scope *scope.ClusterMigrationScope) (ctrl.Result, error) {
	log := scope.Logger
	log.Info("Reconciling deletion", "clustermigration", scope.ClusterMigration.Name, "namespace", scope.ClusterMigration.Namespace)

	// Delete all ESXIMigrations
	for _, esxi := range scope.ClusterMigration.Spec.ESXIMigrationSequence {
		esxiMigration, err := utils.GetESXIMigration(ctx, r.Client, esxi, scope.RollingMigrationPlan)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			log.Error(err, "Failed to get ESXIMigration", "esxiName", esxi)
			return ctrl.Result{}, errors.Wrap(err, "failed to get esxi migration")
		}
		if err := r.Delete(ctx, esxiMigration); err != nil {
			log.Error(err, "Failed to delete ESXIMigration", "esxiName", esxi)
			return ctrl.Result{}, errors.Wrap(err, "failed to delete esxi migration")
		}
	}

	// Wait for all ESXIMigrations to be deleted
	for _, esxi := range scope.ClusterMigration.Spec.ESXIMigrationSequence {
		_, err := utils.GetESXIMigration(ctx, r.Client, esxi, scope.RollingMigrationPlan)
		if err == nil {
			// ESXIMigration still exists, requeue
			log.Info("ESXIMigration still exists, requeuing", "esxiName", esxi)
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}
	}

	controllerutil.RemoveFinalizer(scope.ClusterMigration, constants.ClusterMigrationFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterMigrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vjailbreakv1alpha1.ClusterMigration{}).
		Complete(r)
}

// UpdateClusterMigrationStatus updates the status, message, and current ESXi host for a ClusterMigration resource.
// It logs the status change and persists the update to the Kubernetes API server.
func (r *ClusterMigrationReconciler) UpdateClusterMigrationStatus(ctx context.Context, scope *scope.ClusterMigrationScope, status vjailbreakv1alpha1.ClusterMigrationPhase, message, currentESXi string) error {
	log := scope.Logger
	log.V(1).Info("Updating ClusterMigration status",
		"previousPhase", scope.ClusterMigration.Status.Phase,
		"newPhase", status,
		"message", message,
		"currentESXi", currentESXi)
	scope.ClusterMigration.Status.Phase = status
	scope.ClusterMigration.Status.Message = message
	scope.ClusterMigration.Status.CurrentESXi = currentESXi
	return r.Status().Update(ctx, scope.ClusterMigration)
}

// CheckAndUpdateClusterMigrationStatus examines all related ESXIMigration resources and updates the ClusterMigration status accordingly.
// It aggregates the state of all ESXi migrations to determine the overall cluster migration status.
func (r *ClusterMigrationReconciler) CheckAndUpdateClusterMigrationStatus(ctx context.Context, scope *scope.ClusterMigrationScope) error {
	log := scope.Logger
	esxiMigrationList := &vjailbreakv1alpha1.ESXIMigrationList{}
	if err := r.List(ctx, esxiMigrationList, client.InNamespace(scope.ClusterMigration.Namespace),
		client.MatchingLabels{constants.ClusterMigrationLabel: scope.ClusterMigration.Name}); err != nil {
		return err
	}

	// for _, esxiMigration := range esxiMigrationList.Items {
	// 	switch esxiMigration.Status.Phase {
	// 	case vjailbreakv1alpha1.ESXIMigrationPhasePending:

	// 	}
	// }

	log.V(1).Info("Retrieved ESXIMigrations", "count", len(esxiMigrationList.Items))
	return nil
}

func handleVMMigrations(ctx context.Context, scope *scope.ClusterMigrationScope) error {
	log := scope.Logger
	targetClusterName := ""
	for _, mapping := range scope.RollingMigrationPlan.Spec.ClusterMapping {
		if mapping.VMwareClusterName == scope.ClusterMigration.Spec.ClusterName {
			targetClusterName = mapping.PCDClusterName
			break
		}
	}
	if targetClusterName == "" {
		log.Info("Target cluster name not found, using default cluster for VM migrations")
	} else {
		// update migrationtemplate with target cluster name.
		// This is possible as all the VMs are migrated to the same cluster.
		// We disable selection of other VMs from the UI to prevent this.
		// TODO(vPwned): Add backend validation to prevent this.
		// This is potentially a problem when we support multiple clusters.
		migrationTemplate := &vjailbreakv1alpha1.MigrationTemplate{}
		if err := scope.Client.Get(ctx, k8stypes.NamespacedName{
			Name:      scope.RollingMigrationPlan.Spec.MigrationTemplate,
			Namespace: constants.NamespaceMigrationSystem},
			migrationTemplate); err != nil {
			return errors.Wrap(err, "failed to get migration template")
		}

		migrationTemplate.Spec.TargetPCDClusterName = targetClusterName
		if err := scope.Client.Update(ctx, migrationTemplate); err != nil {
			return errors.Wrap(err, "failed to update migration template")
		}
	}

	// execute VM Migrations
	err := utils.ConvertVMSequenceToMigrationPlans(ctx, scope, 10)
	if err != nil {
		return errors.Wrap(err, "failed to convert VM sequence to migration plans")
	}
	return nil
}

func countSuccessfulESXIMigrations(ctx context.Context, scope *scope.ClusterMigrationScope) (int, error) {
	log := scope.Logger
	count := 0
	for _, esxi := range scope.ClusterMigration.Spec.ESXIMigrationSequence {
		esxiMigration, err := utils.GetESXIMigration(ctx, scope.Client, esxi, scope.RollingMigrationPlan)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			log.Error(err, "Failed to get ESXIMigration", "esxiName", esxi)
			return 0, errors.Wrap(err, "failed to get esxi migration")
		}
		if esxiMigration.Status.Phase == vjailbreakv1alpha1.ESXIMigrationPhaseSucceeded {
			count++
		}
	}
	return count, nil
}
