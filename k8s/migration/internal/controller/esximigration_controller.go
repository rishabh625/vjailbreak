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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pkg/errors"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/scope"
	utils "github.com/platform9/vjailbreak/k8s/migration/pkg/utils"
	providers "github.com/platform9/vjailbreak/pkg/vpwned/sdk/providers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ESXIMigrationReconciler reconciles a ESXIMigration object
type ESXIMigrationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=esximigrations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=esximigrations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=esximigrations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ESXIMigrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctxlog := log.FromContext(ctx).WithName(constants.ESXIMigrationControllerName)
	ctxlog.Info("Starting reconciliation", "esximigration", req.NamespacedName)

	esxiMigration := &vjailbreakv1alpha1.ESXIMigration{}
	if err := r.Get(ctx, req.NamespacedName, esxiMigration); err != nil {
		if apierrors.IsNotFound(err) {
			ctxlog.Info("Resource not found, likely deleted", "esximigration", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		ctxlog.Error(err, "Failed to get ESXIMigration resource", "esximigration", req.NamespacedName)
		return ctrl.Result{}, err
	}

	scope, err := scope.NewESXIMigrationScope(scope.ESXIMigrationScopeParams{
		Logger:        ctxlog,
		Client:        r.Client,
		ESXIMigration: esxiMigration,
	})
	if err != nil {
		ctxlog.Error(err, "Failed to create ESXIMigrationScope")
		return ctrl.Result{}, errors.Wrap(err, "failed to create ESXIMigrationScope")
	}

	rollingMigrationPlan := &vjailbreakv1alpha1.RollingMigrationPlan{}
	rollingMigrationPlanKey := client.ObjectKey{Namespace: esxiMigration.Namespace, Name: esxiMigration.Spec.RollingMigrationPlanRef.Name}
	if err := r.Get(ctx, rollingMigrationPlanKey, rollingMigrationPlan); err != nil {
		if apierrors.IsNotFound(err) {
			if !esxiMigration.DeletionTimestamp.IsZero() {
				ctxlog.Info("Resource is being deleted, reconciling deletion", "esximigration", req.NamespacedName)
				return r.reconcileDelete(ctx, scope)
			}
			return ctrl.Result{}, errors.Wrap(err, "failed to get RollingMigrationPlan")
		}
		return ctrl.Result{}, errors.Wrap(err, "failed to get RollingMigrationPlan")
	}

	scope.RollingMigrationPlan = rollingMigrationPlan

	// Always close the scope when exiting this function such that we can persist any ESXIMigration changes.
	defer func() {
		if err := scope.Close(); err != nil && reterr == nil {
			ctxlog.Error(err, "Failed to close ESXIMigrationScope")
			reterr = err
		}
	}()

	if !esxiMigration.DeletionTimestamp.IsZero() {
		ctxlog.Info("Resource is being deleted, reconciling deletion", "esximigration", req.NamespacedName)
		return r.reconcileDelete(ctx, scope)
	}

	return r.reconcileNormal(ctx, scope)
}

func (r *ESXIMigrationReconciler) reconcileNormal(ctx context.Context, scope *scope.ESXIMigrationScope) (ctrl.Result, error) {
	log := scope.Logger
	log.Info("Starting normal reconciliation", "esximigration", scope.ESXIMigration.Name, "namespace", scope.ESXIMigration.Namespace)
	controllerutil.AddFinalizer(scope.ESXIMigration, constants.ESXIMigrationFinalizer)
	switch scope.ESXIMigration.Status.Phase {
	case "":
		scope.ESXIMigration.Status.Phase = vjailbreakv1alpha1.ESXIMigrationPhaseWaiting
		err := r.Status().Update(ctx, scope.ESXIMigration)
		if err != nil {
			log.Error(err, "Failed to update ESXIMigration status")
			return ctrl.Result{}, errors.Wrap(err, "failed to update ESXi migration status")
		}
	case vjailbreakv1alpha1.ESXIMigrationPhaseSucceeded:
		log.Info("ESXIMigration already succeeded")
		return ctrl.Result{}, nil
	case vjailbreakv1alpha1.ESXIMigrationPhaseFailed:
		log.Info("ESXIMigration already failed")
		return ctrl.Result{}, nil
	}

	if utils.IsESXIMigrationPaused(ctx, scope.ESXIMigration.Name, scope.Client) {
		scope.ESXIMigration.Status.Phase = vjailbreakv1alpha1.ESXIMigrationPhasePaused
		if err := scope.Client.Status().Update(ctx, scope.ESXIMigration); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update ESXi migration status")
		}
		log.Info(fmt.Sprintf("ESXi migration %s is paused, skipping reconciliation", scope.ESXIMigration.Name))
		return ctrl.Result{}, nil
	}

	bmConfig := &vjailbreakv1alpha1.BMConfig{}
	bmConfigKey := client.ObjectKey{Namespace: scope.ESXIMigration.Namespace, Name: scope.RollingMigrationPlan.Spec.BMConfigRef.Name}
	if err := r.Get(ctx, bmConfigKey, bmConfig); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrap(err, "failed to get BMConfig")
		}
		log.Error(err, "Failed to get BMConfig", "bmconfig", bmConfigKey)
		return ctrl.Result{}, errors.Wrap(err, "failed to get BMConfig")
	}

	if scope.ESXIMigration.Status.Phase == vjailbreakv1alpha1.ESXIMigrationPhaseCordoned {
		return r.handleESXiCordoned(ctx, scope, bmConfig)
	}

	if scope.ESXIMigration.Status.Phase == vjailbreakv1alpha1.ESXIMigrationPhaseWaitingForPCDHost {
		return r.handleESXiWaitingForPCDHost(ctx, scope)
	}

	if scope.ESXIMigration.Status.Phase == vjailbreakv1alpha1.ESXIMigrationPhaseConfiguringPCDHost {
		return r.handleESXiConfiguringPCDHost(ctx, scope)
	}

	inMaintenance, err := utils.CheckESXiInMaintenanceMode(ctx, r.Client, scope)
	if err != nil {
		log.Error(err, "Failed to check ESXi maintenance mode", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		return ctrl.Result{}, errors.Wrap(err, "failed to check ESXi maintenance mode")
	}
	if inMaintenance {
		return r.handleESXiInMaintenanceMode(ctx, scope)
	}

	log.Info("Putting ESXi in maintenance mode", "esxiName", scope.ESXIMigration.Spec.ESXiName)
	err = utils.PutESXiInMaintenanceMode(ctx, r.Client, scope)
	if err != nil {
		log.Error(err, "Failed to put ESXi in maintenance mode", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		scope.ESXIMigration.Status.Phase = vjailbreakv1alpha1.ESXIMigrationPhaseFailed
		updateErr := scope.Client.Status().Update(ctx, scope.ESXIMigration)
		if updateErr != nil {
			return ctrl.Result{}, errors.Wrap(updateErr, "failed to update ESXi migration status to failed")
		}
		return ctrl.Result{}, errors.Wrap(err, "failed to put ESXi in maintenance mode")
	}
	log.Info("Successfully updated ESXIMigration status to maintenance")
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// nolint:unparam
func (r *ESXIMigrationReconciler) reconcileDelete(_ context.Context, scope *scope.ESXIMigrationScope) (ctrl.Result, error) {
	log := scope.Logger
	log.Info("Reconciling deletion", "esximigration", scope.ESXIMigration.Name, "namespace", scope.ESXIMigration.Namespace)

	controllerutil.RemoveFinalizer(scope.ESXIMigration, constants.ESXIMigrationFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ESXIMigrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vjailbreakv1alpha1.ESXIMigration{}).
		Complete(r)
}

func (r *ESXIMigrationReconciler) handleESXiCordoned(ctx context.Context, scope *scope.ESXIMigrationScope, bmConfig *vjailbreakv1alpha1.BMConfig) (ctrl.Result, error) {
	log := scope.Logger
	log.Info("ESXi is cordoned", "esxiName", scope.ESXIMigration.Spec.ESXiName)
	provider, err := providers.GetProvider(string(bmConfig.Spec.ProviderType))
	if err != nil {
		return ctrl.Result{}, err
	}
	err = utils.ConvertESXiToPCDHost(ctx, scope, provider)
	if err != nil {
		return ctrl.Result{}, err
	}

	scope.ESXIMigration.Status.Phase = vjailbreakv1alpha1.ESXIMigrationPhaseWaitingForPCDHost
	err = r.Status().Update(ctx, scope.ESXIMigration)
	if err != nil {
		log.Error(err, "Failed to update ESXIMigration status", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		return ctrl.Result{}, errors.Wrap(err, "failed to update ESXi migration status")
	}
	return ctrl.Result{}, nil
}

func (r *ESXIMigrationReconciler) handleESXiWaitingForPCDHost(ctx context.Context, scope *scope.ESXIMigrationScope) (ctrl.Result, error) {
	log := scope.Logger
	log.Info("ESXi is waiting for PCD host", "esxiName", scope.ESXIMigration.Spec.ESXiName)
	destOpenstackCreds, err := utils.GetDestinationOpenstackCredsFromRollingMigrationPlan(ctx, r.Client, scope.RollingMigrationPlan)
	if err != nil {
		log.Error(err, "Failed to get destination openstack credentials", "openstackCreds", destOpenstackCreds)
		return ctrl.Result{}, errors.Wrap(err, "failed to get destination openstack credentials")
	}
	vmwareHost, err := utils.GetVMwareHostFromESXiName(ctx, r.Client, scope.ESXIMigration.Spec.ESXiName)
	if err != nil {
		log.Error(err, "Failed to get VMware host", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		return ctrl.Result{}, errors.Wrap(err, "failed to get VMware host")
	}
	showedUp, err := utils.WaitforHostToShowUpOnPCD(ctx, r.Client, destOpenstackCreds.Name, vmwareHost.Spec.HardwareUUID)
	if err != nil {
		log.Error(err, "Failed to wait for host to show up on PCD", "hostID", vmwareHost.Spec.HardwareUUID)
		return ctrl.Result{}, errors.Wrap(err, "failed to wait for host to show up on PCD")
	}
	if !showedUp {
		log.Info("Host did not show up on PCD, waiting for it to show up", "hostID", vmwareHost.Spec.HardwareUUID)
		return ctrl.Result{RequeueAfter: constants.CredsRequeueAfter}, nil
	}
	log.Info("Host showed up on PCD", "hostName", vmwareHost.Spec.Name)
	scope.ESXIMigration.Status.Phase = vjailbreakv1alpha1.ESXIMigrationPhaseConfiguringPCDHost
	err = r.Status().Update(ctx, scope.ESXIMigration)
	if err != nil {
		log.Error(err, "Failed to update ESXIMigration status", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		return ctrl.Result{}, errors.Wrap(err, "failed to update ESXi migration status")
	}
	return ctrl.Result{}, nil
}

func (r *ESXIMigrationReconciler) handleESXiConfiguringPCDHost(ctx context.Context, scope *scope.ESXIMigrationScope) (ctrl.Result, error) {
	log := scope.Logger
	log.Info("ESXi is configuring PCD host", "esxiName", scope.ESXIMigration.Spec.ESXiName)
	var pcdClusterName string

	destOpenstackCreds, err := utils.GetDestinationOpenstackCredsFromRollingMigrationPlan(ctx, r.Client, scope.RollingMigrationPlan)
	if err != nil {
		log.Error(err, "Failed to get destination openstack credentials", "openstackCreds", destOpenstackCreds)
		return ctrl.Result{}, errors.Wrap(err, "failed to get destination openstack credentials")
	}
	vmwareHost, err := utils.GetVMwareHostFromESXiName(ctx, r.Client, scope.ESXIMigration.Spec.ESXiName)
	if err != nil {
		log.Error(err, "Failed to get VMware host", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		return ctrl.Result{}, errors.Wrap(err, "failed to get VMware host")
	}
	if vmwareHost.Spec.HostConfigID == "" {
		log.Info("Host config ID is empty, pausing ESXi migration. please assign host config to ESXi to continue", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		scope.RollingMigrationPlan.Labels[constants.PauseMigrationLabel] = constants.TrueString
		err = r.Update(ctx, scope.RollingMigrationPlan)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update RollingMigrationPlan")
		}
		return ctrl.Result{RequeueAfter: constants.CredsRequeueAfter}, nil
	}
	if len(scope.RollingMigrationPlan.Spec.ClusterMapping) > 0 {
		for _, mapping := range scope.RollingMigrationPlan.Spec.ClusterMapping {
			if mapping.VMwareClusterName == vmwareHost.Spec.ClusterName {
				pcdClusterName = mapping.PCDClusterName
				break
			}
		}
	}
	if pcdClusterName == "" {
		log.Info("PCD cluster name is empty, pausing ESXi migration. please assign PCD cluster to ESXi to continue", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		pcdClusterList := &vjailbreakv1alpha1.PCDClusterList{}
		err = r.List(ctx, pcdClusterList)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to list PCD clusters")
		}
		if len(pcdClusterList.Items) == 0 {
			log.Info("No PCD clusters found, pausing ESXi migration. please create PCD cluster to continue", "esxiName", scope.ESXIMigration.Spec.ESXiName)
			return ctrl.Result{RequeueAfter: constants.CredsRequeueAfter}, nil
		}
		pcdClusterName = pcdClusterList.Items[0].Name
	}

	if err := utils.AssignHostConfigToHost(ctx, r.Client, destOpenstackCreds.Name, vmwareHost.Spec.HardwareUUID, vmwareHost.Spec.HostConfigID); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to assign host config to PCD host")
	}
	if err := utils.AssignHypervisorRoleToHost(ctx, r.Client, destOpenstackCreds.Name, vmwareHost.Spec.HardwareUUID, pcdClusterName); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to assign hypervisor role to PCD host")
	}
	assigned, err := utils.WaitForHypervisorRoleAssignment(ctx, r.Client, destOpenstackCreds.Name, vmwareHost.Spec.HardwareUUID)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to wait for hypervisor role assignment")
	}
	if !assigned {
		return ctrl.Result{RequeueAfter: constants.CredsRequeueAfter}, nil
	}
	log.Info("Assigned Hypervisor Role to PCD Host", "hostName", vmwareHost.Spec.Name)

	scope.ESXIMigration.Status.Phase = vjailbreakv1alpha1.ESXIMigrationPhaseSucceeded
	err = r.Status().Update(ctx, scope.ESXIMigration)
	if err != nil {
		log.Error(err, "Failed to update ESXIMigration status", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		return ctrl.Result{}, errors.Wrap(err, "failed to update ESXi migration status")
	}

	// Remove the ESXi host from vCenter before changing the phase
	err = utils.RemoveESXiFromVCenter(ctx, r.Client, scope)
	if err != nil {
		log.Error(err, "Failed to remove ESXi from vCenter, retrying after one minute", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	log.Info("Successfully removed ESXi from vCenter", "esxiName", scope.ESXIMigration.Spec.ESXiName)

	return ctrl.Result{}, nil
}

func (r *ESXIMigrationReconciler) handleESXiInMaintenanceMode(ctx context.Context, scope *scope.ESXIMigrationScope) (ctrl.Result, error) {
	log := scope.Logger
	log.Info("ESXi is in maintenance mode", "esxiName", scope.ESXIMigration.Spec.ESXiName)
	vmCount, err := utils.CountVMsOnESXi(ctx, r.Client, scope)
	if err != nil {
		log.Error(err, "Failed to count VMs on ESXi", "esxiName", scope.ESXIMigration.Spec.ESXiName)
		return ctrl.Result{}, errors.Wrap(err, "failed to count VMs on ESXi")
	}
	log.Info("Counted VMs on ESXi", "esxiName", scope.ESXIMigration.Spec.ESXiName, "vmCount", vmCount)
	if vmCount != 0 {
		log.Info("VMs present on this ESXi host, waiting for VMs to be moved", "ESXiName", scope.ESXIMigration.Spec.ESXiName)
		// Omkar change back to 5 mins
		scope.ESXIMigration.Status.Phase = vjailbreakv1alpha1.ESXIMigrationPhaseWaitingForVMsToBeMoved
		err = r.Status().Update(ctx, scope.ESXIMigration)
		if err != nil {
			log.Error(err, "Failed to update ESXIMigration status")
			return ctrl.Result{}, errors.Wrap(err, "failed to update ESXi migration status")
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}
	log.Info("No VMs on this ESXi host, removing from vCenter and converting to PCD host", "ESXiName", scope.ESXIMigration.Spec.ESXiName)

	scope.ESXIMigration.Status.Phase = vjailbreakv1alpha1.ESXIMigrationPhaseCordoned
	err = r.Status().Update(ctx, scope.ESXIMigration)
	if err != nil {
		log.Error(err, "Failed to update ESXIMigration status")
		return ctrl.Result{}, errors.Wrap(err, "failed to update ESXi migration status")
	}
	log.Info("Successfully updated ESXIMigration status to cordoned")
	return ctrl.Result{}, nil
}
