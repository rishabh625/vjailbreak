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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	constants "github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	scope "github.com/platform9/vjailbreak/k8s/migration/pkg/scope"
	utils "github.com/platform9/vjailbreak/k8s/migration/pkg/utils"
)

// RollingMigrationPlanReconciler reconciles a RollingMigrationPlan object
type RollingMigrationPlanReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Logger logr.Logger
	Local  bool
}

// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rollingmigrationplans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rollingmigrationplans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=rollingmigrationplans/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RollingMigrationPlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctxlog := log.FromContext(ctx).WithName(constants.RollingMigrationPlanControllerName)

	rollingmigrationplan := &vjailbreakv1alpha1.RollingMigrationPlan{}
	if err := r.Get(ctx, req.NamespacedName, rollingmigrationplan); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	scope, err := scope.NewRollingMigrationPlanScope(scope.RollingMigrationPlanScopeParams{
		Logger:               ctxlog,
		Client:               r.Client,
		RollingMigrationPlan: rollingmigrationplan,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always close the scope when exiting this function such that we can persist any RollingMigrationPlan changes.
	defer func() {
		if err := scope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	if !rollingmigrationplan.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, scope)
	}
	return r.reconcileNormal(ctx, scope)
}

func (r *RollingMigrationPlanReconciler) reconcileNormal(ctx context.Context, scope *scope.RollingMigrationPlanScope) (ctrl.Result, error) {
	log := scope.Logger
	migrationPlan := scope.RollingMigrationPlan
	log.Info(fmt.Sprintf("Reconciling RollingMigrationPlan '%s'", migrationPlan.Name))

	controllerutil.AddFinalizer(migrationPlan, constants.RollingMigrationPlanFinalizer)
	if err := scope.Close(); err != nil {
		log.Error(err, "Failed to close RollingMigrationPlanScope")
		return ctrl.Result{}, errors.Wrap(err, "failed to close rolling migration plan scope")
	}

	// validate environment is PCD
	if isPCD, err := utils.ValidateOpenstackIsPCD(ctx, r.Client, migrationPlan); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to validate environment")
	} else if !isPCD {
		log.Info("OpenStack environment is not PCD, skipping rolling migration plan")
		return ctrl.Result{}, nil
	}

	switch migrationPlan.Status.Phase {
	case "":
		migrationPlan.Status.Phase = vjailbreakv1alpha1.RollingMigrationPlanPhaseWaiting
	case vjailbreakv1alpha1.RollingMigrationPlanPhaseSucceeded:
		log.Info("RollingMigrationPlan already succeeded")
		return ctrl.Result{}, nil
	case vjailbreakv1alpha1.RollingMigrationPlanPhaseFailed:
		log.Info("RollingMigrationPlan already failed")
		return ctrl.Result{}, nil
	}

	if utils.IsRollingMigrationPlanPaused(ctx, migrationPlan.Name, r.Client) {
		if err := utils.PauseRollingMigrationPlan(ctx, scope); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to pause rolling migration plan")
		}
		return ctrl.Result{}, nil
	}

	// check and create default validation configmap
	configMap, err := utils.GetValidationConfigMapForRollingMigrationPlan(ctx, r.Client, migrationPlan)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Validation configmap not found, creating default validation configmap")
			if _, err := utils.CreateDefaultValidationConfigMapForRollingMigrationPlan(ctx, r.Client, migrationPlan); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to create default validation configmap")
			}
		}
	}
	if configMap == nil {
		configMap, err = utils.GetValidationConfigMapForRollingMigrationPlan(ctx, r.Client, migrationPlan)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to get validation configmap for rolling migration plan")
		}
	}

	// Do not validate if the rolling migration plan has already started running
	if migrationPlan.Status.Phase != vjailbreakv1alpha1.RollingMigrationPlanPhaseRunning {
		valid, message, err := utils.ValidateRollingMigrationPlan(ctx, scope, configMap)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to validate rolling migration plan")
		}

		if !valid {
			log.Info(fmt.Sprintf("RollingMigrationPlan %s is not valid: %s, requeueing after 1 minute", migrationPlan.Name, message))
			migrationPlan.Status.Message = message
			migrationPlan.Status.Phase = vjailbreakv1alpha1.RollingMigrationPlanPhaseValidationFailed
			if err := r.Status().Update(ctx, migrationPlan); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to update rolling migration plan")
			}
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}
	}

	if err := utils.ResumeRollingMigrationPlan(ctx, scope); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to resume rolling migration plan")
	}

	if migrationPlan.Spec.CloudInitConfigRef == nil {
		log.Info("CloudInitConfigRef is not set")
		err := utils.MergeCloudInitAndCreateSecret(ctx, scope, r.Local)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to merge cloud-init config and create secret")
		}
	}

	if err := utils.UpdateESXiNamesInRollingMigrationPlan(ctx, scope); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to update ESXi in rolling migration plan")
	}

	// execute rolling migration plan
	if requeue, err := r.ExecuteRollingMigrationPlan(ctx, scope); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to execute rolling migration plan")
	} else if requeue {
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Aggregate MigrationPlan statuses and update RollingMigrationPlan status
	updated, err := r.aggregateAndUpdateMigrationPlanStatuses(ctx, scope)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to aggregate migration plan statuses")
	}
	if updated {
		// Requeue to check status updates
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

func (r *RollingMigrationPlanReconciler) reconcileDelete(ctx context.Context, scope *scope.RollingMigrationPlanScope) (ctrl.Result, error) {
	log := scope.Logger
	log.Info("Reconciling deletion", "rollingmigrationplan", scope.RollingMigrationPlan.Name, "namespace", scope.RollingMigrationPlan.Namespace)

	// Delete all ClusterMigrations
	for _, cluster := range scope.RollingMigrationPlan.Spec.ClusterSequence {
		clusterMigration, err := utils.GetClusterMigration(ctx, r.Client, cluster.ClusterName, scope.RollingMigrationPlan)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return ctrl.Result{}, errors.Wrap(err, "failed to get cluster migration")
		}
		if err := r.Delete(ctx, clusterMigration); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to delete cluster migration")
		}
	}

	// Delete all MigrationPlans
	for _, vm := range scope.RollingMigrationPlan.Spec.VMMigrationPlans {
		migrationPlan, err := utils.GetMigrationPlan(ctx, r.Client, vm)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return ctrl.Result{}, errors.Wrap(err, "failed to get migration plan")
		}
		if err := r.Delete(ctx, migrationPlan); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to delete migration plan")
		}
	}

	// Wait for all ClusterMigrations to be deleted
	for _, cluster := range scope.RollingMigrationPlan.Spec.ClusterSequence {
		_, err := utils.GetClusterMigration(ctx, r.Client, cluster.ClusterName, scope.RollingMigrationPlan)
		if err == nil {
			// ClusterMigration still exists, requeue
			log.Info("ClusterMigration still exists, requeuing", "cluster", cluster)
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}
	}

	// Wait for all MigrationPlans to be deleted
	for _, vm := range scope.RollingMigrationPlan.Spec.VMMigrationPlans {
		_, err := utils.GetMigrationPlan(ctx, r.Client, vm)
		if err == nil {
			// MigrationPlan still exists, requeue
			log.Info("MigrationPlan still exists, requeuing", "vm", vm)
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}
	}

	// Delete MigrationTemplates
	migrationTemplate, err := utils.GetMigrationTemplate(ctx, r.Client, scope.RollingMigrationPlan.Spec.MigrationTemplate, scope.RollingMigrationPlan)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get MigrationTemplate", "vm", scope.RollingMigrationPlan.Spec.MigrationTemplate)
			return ctrl.Result{}, errors.Wrap(err, "failed to get migration template")
		}
	} else {
		if delErr := r.Delete(ctx, migrationTemplate); delErr != nil {
			log.Error(delErr, "Failed to delete MigrationTemplate", "vm", scope.RollingMigrationPlan.Spec.MigrationTemplate)
			return ctrl.Result{}, errors.Wrap(delErr, "failed to delete migration template")
		}
	}

	// Delete validation configmap
	validationConfigMap, err := utils.GetValidationConfigMapForRollingMigrationPlan(ctx, r.Client, scope.RollingMigrationPlan)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get validation configmap", "rollingmigrationplan", scope.RollingMigrationPlan.Name)
			return ctrl.Result{}, errors.Wrap(err, "failed to get validation configmap")
		}
	} else {
		if delErr := r.Delete(ctx, validationConfigMap); delErr != nil {
			log.Error(delErr, "Failed to delete validation configmap", "rollingmigrationplan", scope.RollingMigrationPlan.Name)
			return ctrl.Result{}, errors.Wrap(delErr, "failed to delete validation configmap")
		}
	}

	controllerutil.RemoveFinalizer(scope.RollingMigrationPlan, constants.RollingMigrationPlanFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
// aggregateAndUpdateMigrationPlanStatuses collects statuses from all MigrationPlans and updates the RollingMigrationPlan status
func (r *RollingMigrationPlanReconciler) aggregateAndUpdateMigrationPlanStatuses(ctx context.Context, scope *scope.RollingMigrationPlanScope) (bool, error) {
	log := scope.Logger
	log.Info("Aggregating MigrationPlan statuses", "rollingmigrationplan", scope.RollingMigrationPlan.Name)

	var totalPlans, succeededPlans, failedPlans, runningPlans, waitingPlans int
	var statusMessages []string

	// Get all MigrationPlans associated with this RollingMigrationPlan
	for _, planName := range scope.RollingMigrationPlan.Spec.VMMigrationPlans {
		migrationPlan, err := utils.GetMigrationPlan(ctx, r.Client, planName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// force update requeue, by sending true
				return true, nil
			}
			return false, errors.Wrap(err, "failed to get migration plan")
		}
		totalPlans++

		// Count statuses based on PodPhase
		switch migrationPlan.Status.MigrationStatus {
		case corev1.PodSucceeded:
			succeededPlans++
		case corev1.PodFailed:
			failedPlans++
			if migrationPlan.Status.MigrationMessage != "" {
				statusMessages = append(statusMessages, fmt.Sprintf("VM %s: %s", planName, migrationPlan.Status.MigrationMessage))
			}
		case corev1.PodRunning:
			runningPlans++
		default: // PodPending or other states
			waitingPlans++
		}
	}

	if totalPlans == 0 {
		log.Info("No MigrationPlans found for this RollingMigrationPlan")
		return false, nil
	}

	// Determine the overall status based on aggregated statuses
	var currentPhase vjailbreakv1alpha1.RollingMigrationPlanPhase
	var message string

	// Determine the phase based on the migration plan statuses
	switch {
	case failedPlans > 0:
		currentPhase = vjailbreakv1alpha1.RollingMigrationPlanPhaseFailed
		message = fmt.Sprintf("Failed to complete migration: %d/%d plans failed. %s",
			failedPlans, totalPlans, strings.Join(statusMessages, "; "))
	case runningPlans > 0:
		currentPhase = vjailbreakv1alpha1.RollingMigrationPlanPhaseRunning
		message = fmt.Sprintf("Migration in progress: %d/%d plans succeeded, %d running, %d waiting",
			succeededPlans, totalPlans, runningPlans, waitingPlans)
	case waitingPlans > 0:
		currentPhase = vjailbreakv1alpha1.RollingMigrationPlanPhaseWaiting
		message = fmt.Sprintf("Waiting for migration to start: %d/%d plans succeeded, %d waiting",
			succeededPlans, totalPlans, waitingPlans)
	case succeededPlans == totalPlans:
		currentPhase = vjailbreakv1alpha1.RollingMigrationPlanPhaseSucceeded
		message = fmt.Sprintf("Migration completed successfully: all %d plans succeeded", totalPlans)
	default:
		currentPhase = vjailbreakv1alpha1.RollingMigrationPlanPhaseWaiting
		message = "Preparing for migration"
	}

	// Update the status if it has changed
	// Build new migrated and failed VMs lists
	newMigratedVMs := []string{}
	newFailedVMs := []string{}

	for _, cluster := range scope.RollingMigrationPlan.Spec.ClusterSequence {
		for _, vmName := range cluster.VMSequence {
			vmMigration, err := utils.GetVMMigration(ctx, r.Client, vmName.VMName, scope.RollingMigrationPlan)
			if err != nil {
				if apierrors.IsNotFound(err) {
					log.Info("VMMigration not found, skipping", "vm", vmName.VMName, "error", err)
					continue
				}
				return false, errors.Wrap(err, "failed to get VMMigration")
			}
			if corev1.PodPhase(vmMigration.Status.Phase) == corev1.PodSucceeded {
				newMigratedVMs = append(newMigratedVMs, vmName.VMName)
			} else if corev1.PodPhase(vmMigration.Status.Phase) == corev1.PodFailed {
				newFailedVMs = append(newFailedVMs, vmName.VMName)
			}
		}
	}

	// Check if any status fields have changed
	migratedVMsChanged := !utils.StringSlicesEqual(scope.RollingMigrationPlan.Status.MigratedVMs, newMigratedVMs)
	failedVMsChanged := !utils.StringSlicesEqual(scope.RollingMigrationPlan.Status.FailedVMs, newFailedVMs)
	phaseChanged := scope.RollingMigrationPlan.Status.VMMigrationsPhase != string(currentPhase)
	messageChanged := scope.RollingMigrationPlan.Status.Message != message

	if phaseChanged || messageChanged || migratedVMsChanged || failedVMsChanged {
		// Update status fields only if there are changes
		scope.RollingMigrationPlan.Status.VMMigrationsPhase = string(currentPhase)
		scope.RollingMigrationPlan.Status.Message = message
		scope.RollingMigrationPlan.Status.MigratedVMs = newMigratedVMs
		scope.RollingMigrationPlan.Status.FailedVMs = newFailedVMs

		if err := r.Status().Update(ctx, scope.RollingMigrationPlan); err != nil {
			return false, errors.Wrap(err, "failed to update RollingMigrationPlan status")
		}
		log.Info("Updated RollingMigrationPlan status", "phase", currentPhase, "message", message,
			"migratedVMsChanged", migratedVMsChanged, "failedVMsChanged", failedVMsChanged)
		return true, nil
	}

	return false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RollingMigrationPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vjailbreakv1alpha1.RollingMigrationPlan{}).
		Complete(r)
}

// UpdateRollingMigrationPlanStatus updates the status fields of a RollingMigrationPlan resource including phase, message, and current targets.
// It also updates statistics about migrated and failed VMs by checking the status of related VMMigration resources.
func (r *RollingMigrationPlanReconciler) UpdateRollingMigrationPlanStatus(ctx context.Context, scope *scope.RollingMigrationPlanScope, status vjailbreakv1alpha1.RollingMigrationPlanPhase, message, currentCluster, currentESXi string) error {
	scope.RollingMigrationPlan.Status.Phase = status
	scope.RollingMigrationPlan.Status.Message = message
	scope.RollingMigrationPlan.Status.CurrentCluster = currentCluster
	scope.RollingMigrationPlan.Status.CurrentESXi = currentESXi

	// update migrated and failed VMs
	for _, vm := range scope.RollingMigrationPlan.Spec.VMMigrationPlans {
		migration, err := utils.GetVMMigration(ctx, scope.Client, vm, scope.RollingMigrationPlan)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return errors.Wrap(err, "failed to get VMMigration")
		}
		switch migration.Status.Phase {
		case vjailbreakv1alpha1.VMMigrationPhaseFailed:
			scope.RollingMigrationPlan.Status.FailedVMs = append(scope.RollingMigrationPlan.Status.FailedVMs, vm)
		case vjailbreakv1alpha1.VMMigrationPhaseSucceeded:
			scope.RollingMigrationPlan.Status.MigratedVMs = append(scope.RollingMigrationPlan.Status.MigratedVMs, vm)
		}
	}
	return r.Status().Update(ctx, scope.RollingMigrationPlan)
}

// ExecuteRollingMigrationPlan handles the execution of the rolling migration plan by creating and managing ClusterMigration resources.
// It processes one cluster at a time based on the order defined in the migration plan and tracks the progress of each cluster migration.
func (r *RollingMigrationPlanReconciler) ExecuteRollingMigrationPlan(ctx context.Context, scope *scope.RollingMigrationPlanScope) (bool, error) {
	log := scope.Logger
	for _, cluster := range scope.RollingMigrationPlan.Spec.ClusterSequence {
		// TODO(vPwned): VALIDATE: poweroff vms cannot be moved by the vmware vcenter
		// TODO(vPwned): VALIDATE: DRS needs to be enabled and on fully automated mode
		clusterMigration, err := utils.GetClusterMigration(ctx, scope.Client, cluster.ClusterName, scope.RollingMigrationPlan)
		if err != nil {
			if apierrors.IsNotFound(err) {
				if _, createErr := utils.CreateClusterMigration(ctx, scope.Client, cluster, scope.RollingMigrationPlan); createErr != nil {
					return false, errors.Wrap(createErr, "failed to create cluster migration")
				}
				return true, nil
			}

			return false, errors.Wrap(err, "failed to get cluster migration")
		}
		switch clusterMigration.Status.Phase {
		case vjailbreakv1alpha1.ClusterMigrationPhaseFailed:
			log.Info("Cluster migration is in failed state, aborting rolling migration plan", "cluster", cluster, "message", clusterMigration.Status.Message)
			err = r.UpdateRollingMigrationPlanStatus(ctx, scope, vjailbreakv1alpha1.RollingMigrationPlanPhaseFailed, clusterMigration.Status.Message, cluster.ClusterName, clusterMigration.Status.CurrentESXi)
			if err != nil {
				return false, errors.Wrap(err, "failed to update rolling migration plan status")
			}
			return false, nil
		case vjailbreakv1alpha1.ClusterMigrationPhaseSucceeded:
			continue
		case vjailbreakv1alpha1.ClusterMigrationPhaseRunning:
			err = r.UpdateRollingMigrationPlanStatus(ctx, scope, vjailbreakv1alpha1.RollingMigrationPlanPhaseRunning, clusterMigration.Status.Message, cluster.ClusterName, clusterMigration.Status.CurrentESXi)
			if err != nil {
				return false, errors.Wrap(err, "failed to update rolling migration plan status")
			}
			return false, nil
		}

		err = r.UpdateRollingMigrationPlanStatus(ctx, scope, vjailbreakv1alpha1.RollingMigrationPlanPhaseWaiting, clusterMigration.Status.Message, cluster.ClusterName, clusterMigration.Status.CurrentESXi)
		if err != nil {
			return false, errors.Wrap(err, "failed to update rolling migration plan status")
		}
		return false, nil
	}
	return false, nil
}
