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

	"github.com/pkg/errors"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	constants "github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	scope "github.com/platform9/vjailbreak/k8s/migration/pkg/scope"
	utils "github.com/platform9/vjailbreak/k8s/migration/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// OpenstackCredsReconciler reconciles a OpenstackCreds object
type OpenstackCredsReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Local  bool
}

// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=openstackcreds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=openstackcreds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=openstackcreds/finalizers,verbs=update
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=pcdhosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=pcdhosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=pcdhosts/finalizers,verbs=update
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=pcdclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=pcdclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=pcdclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OpenstackCreds object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *OpenstackCredsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctxlog := log.FromContext(ctx).WithName(constants.OpenstackCredsControllerName)
	// Get the OpenstackCreds object
	openstackcreds := &vjailbreakv1alpha1.OpenstackCreds{}
	if err := r.Get(ctx, req.NamespacedName, openstackcreds); err != nil {
		if apierrors.IsNotFound(err) {
			ctxlog.Info("Resource not found, likely deleted", "openstackcreds", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		ctxlog.Error(err, "Failed to get OpenstackCreds resource", "openstackcreds", req.NamespacedName)
		return ctrl.Result{}, err
	}
	ctxlog.V(1).Info("Retrieved OpenstackCreds resource", "openstackcreds", req.NamespacedName, "resourceVersion", openstackcreds.ResourceVersion)
	scope, err := scope.NewOpenstackCredsScope(scope.OpenstackCredsScopeParams{
		Logger:         ctxlog,
		Client:         r.Client,
		OpenstackCreds: openstackcreds,
	})
	if err != nil {
		ctxlog.Error(err, "Failed to create OpenstackCredsScope")
		return ctrl.Result{}, err
	}

	// Always close the scope when exiting this function such that we can persist any OpenstackCreds changes.
	defer func() {
		if err := scope.Close(); err != nil && reterr == nil {
			ctxlog.Error(err, "Failed to close OpenstackCredsScope")
			reterr = err
		}
	}()

	if !openstackcreds.DeletionTimestamp.IsZero() {
		ctxlog.Info("Resource is being deleted, reconciling deletion", "openstackcreds", req.NamespacedName)
		return r.reconcileDelete(ctx, scope)
	}
	ctxlog.Info("Reconciling normal state", "openstackcreds", req.NamespacedName)
	return r.reconcileNormal(ctx, scope)
}

func (r *OpenstackCredsReconciler) reconcileNormal(ctx context.Context,
	scope *scope.OpenstackCredsScope) (ctrl.Result, error) { //nolint:unparam //future use
	ctxlog := scope.Logger
	ctxlog.Info("Starting normal reconciliation", "openstackcreds", scope.OpenstackCreds.Name, "namespace", scope.OpenstackCreds.Namespace)

	controllerutil.AddFinalizer(scope.OpenstackCreds, constants.OpenstackCredsFinalizer)

	// Check if spec matches with kubectl.kubernetes.io/last-applied-configuration
	if _, err := utils.ValidateAndGetProviderClient(ctx, r.Client, scope.OpenstackCreds); err != nil {
		// Update the status of the OpenstackCreds object
		ctxlog.Error(err, "Error validating OpenstackCreds", "openstackcreds", scope.OpenstackCreds.Name)
		scope.OpenstackCreds.Status.OpenStackValidationStatus = "Failed"
		scope.OpenstackCreds.Status.OpenStackValidationMessage = "Error validating OpenStack credentials"
		ctxlog.Info("Updating status to failed", "openstackcreds", scope.OpenstackCreds.Name)
		if err := r.Status().Update(ctx, scope.OpenstackCreds); err != nil {
			ctxlog.Error(err, "Error updating status of OpenstackCreds", "openstackcreds", scope.OpenstackCreds.Name)
			return ctrl.Result{}, err
		}
		ctxlog.Info("Successfully updated status to failed")
	} else {
		err := utils.UpdateMasterNodeImageID(ctx, r.Client, r.Local)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				ctxlog.Error(err, "Failed to update master node image ID and flavor list, skipping reconciliation")
			} else {
				return ctrl.Result{}, errors.Wrap(err, "failed to update master node image id")
			}
			ctxlog.Error(err, "Failed to update master node image ID and flavor list")
		}
		openstackCredential, err := utils.GetOpenstackCredentialsFromSecret(ctx, r.Client, scope.OpenstackCreds.Spec.SecretRef.Name)
		if err != nil {
			ctxlog.Error(err, "Failed to get OpenStack credentials from secret", "secretName", scope.OpenstackCreds.Spec.SecretRef.Name)
			return ctrl.Result{}, errors.Wrap(err, "failed to get Openstack credentials from secret")
		}

		ctxlog.Info("Creating dummy PCD cluster", "openstackcreds", scope.OpenstackCreds.Name)
		err = utils.CreateEntryForNoPCDCluster(ctx, r.Client, scope.OpenstackCreds)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, errors.Wrap(err, "failed to create dummy PCD cluster")
		}

		flavors, err := utils.ListAllFlavors(ctx, r.Client, scope.OpenstackCreds)
		if err != nil {
			ctxlog.Error(err, "Failed to get flavors", "openstackcreds", scope.OpenstackCreds.Name)
			return ctrl.Result{}, errors.Wrap(err, "failed to get flavors")
		}
		scope.OpenstackCreds.Spec.Flavors = flavors
		if err = r.Update(ctx, scope.OpenstackCreds); err != nil {
			ctxlog.Error(err, "Error updating spec of OpenstackCreds", "openstackcreds", scope.OpenstackCreds.Name)
			return ctrl.Result{}, err
		}

		ctxlog.Info("Successfully authenticated to OpenStack", "authURL", openstackCredential.AuthURL)
		// Update the status of the OpenstackCreds object
		scope.OpenstackCreds.Status.OpenStackValidationStatus = string(corev1.PodSucceeded)
		scope.OpenstackCreds.Status.OpenStackValidationMessage = "Successfully authenticated to Openstack"

		// update the status field openstackInfo
		ctxlog.Info("Getting OpenStack info", "openstackcreds", scope.OpenstackCreds.Name)
		openstackinfo, err := utils.GetOpenstackInfo(ctx, r.Client, scope.OpenstackCreds)
		if err != nil {
			ctxlog.Error(err, "Failed to get OpenStack info", "openstackcreds", scope.OpenstackCreds.Name)
			return ctrl.Result{}, errors.Wrap(err, "failed to get Openstack info")
		}
		scope.OpenstackCreds.Status.Openstack = *openstackinfo
		ctxlog.Info("Updating OpenstackCreds status with info", "openstackcreds", scope.OpenstackCreds.Name)
		if err := r.Status().Update(ctx, scope.OpenstackCreds); err != nil {
			ctxlog.Error(err, "Error updating status of OpenstackCreds", "openstackcreds", scope.OpenstackCreds.Name)
			return ctrl.Result{}, err
		}

		// Now with these creds we should populate the flavors as labels in vmwaremachine object.
		// This will help us to create the vmwaremachine object with the correct flavor.
		vmwaremachineList := &vjailbreakv1alpha1.VMwareMachineList{}
		if err := r.List(ctx, vmwaremachineList); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to list vmwaremachine objects")
		}
		for i := range vmwaremachineList.Items {
			vmwaremachine := &vmwaremachineList.Items[i]
			// Get the cpu and memory of the vmwaremachine object
			cpu := vmwaremachine.Spec.VMInfo.CPU
			memory := vmwaremachine.Spec.VMInfo.Memory
			computeClient, err := utils.GetOpenStackClients(context.TODO(), r.Client, scope.OpenstackCreds)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to get OpenStack clients")
			}
			// Now get the closest flavor based on the cpu and memory
			flavor, err := utils.GetClosestFlavour(ctx, cpu, memory, computeClient.ComputeClient)
			if err != nil && !strings.Contains(err.Error(), "no suitable flavor found") {
				ctxlog.Info(fmt.Sprintf("Error message '%s'", vmwaremachine.Name))
				return ctrl.Result{}, errors.Wrap(err, "failed to get closest flavor")
			}
			// Now label the vmwaremachine object with the flavor name
			if flavor == nil {
				if err := utils.CreateOrUpdateLabel(ctx, r.Client, vmwaremachine, scope.OpenstackCreds.Name, "NOT_FOUND"); err != nil {
					return ctrl.Result{}, errors.Wrap(err, "failed to update vmwaremachine object")
				}
			} else {
				if err := utils.CreateOrUpdateLabel(ctx, r.Client, vmwaremachine, scope.OpenstackCreds.Name, flavor.ID); err != nil {
					return ctrl.Result{}, errors.Wrap(err, "failed to update vmwaremachine object")
				}
			}
		}

		if utils.IsOpenstackPCD(*scope.OpenstackCreds) {
			ctxlog.Info("Syncing PCD info because openstackcreds is PCD", "openstackcreds", scope.OpenstackCreds.Name)
			err = utils.SyncPCDInfo(ctx, r.Client, *scope.OpenstackCreds)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to sync PCD info")
			}
		}
	}
	// Requeue to update the status of the OpenstackCreds object more specifically it will update flavors
	return ctrl.Result{Requeue: true, RequeueAfter: constants.CredsRequeueAfter}, nil
}

func (r *OpenstackCredsReconciler) reconcileDelete(ctx context.Context,
	scope *scope.OpenstackCredsScope) (ctrl.Result, error) { //nolint:unparam //future use
	ctxlog := scope.Logger
	ctxlog.Info("Reconciling deletion", "openstackcreds", scope.OpenstackCreds.Name, "namespace", scope.OpenstackCreds.Namespace)
	// Delete the associated secret
	client := r.Client
	secretName := scope.OpenstackCreds.Spec.SecretRef.Name

	ctxlog.Info("Deleting PCD cluster", "openstackcreds", scope.OpenstackCreds.Name)
	if err := utils.DeleteEntryForNoPCDCluster(ctx, client, scope.OpenstackCreds); err != nil {
		ctxlog.Error(err, "Failed to delete PCD cluster", "openstackcreds", scope.OpenstackCreds.Name)
		return ctrl.Result{}, errors.Wrap(err, "failed to delete PCD cluster")
	}

	ctxlog.Info("Deleting associated secret", "secretName", secretName, "namespace", constants.NamespaceMigrationSystem)
	err := client.Delete(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: constants.NamespaceMigrationSystem,
		},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		ctxlog.Error(err, "Failed to delete associated secret", "secretName", secretName)
		return ctrl.Result{}, errors.Wrap(err, "failed to delete associated secret")
	}
	ctxlog.Info("Successfully deleted associated secret or it was already gone", "secretName", secretName)
	ctxlog.Info("Removing finalizer", "finalizer", constants.OpenstackCredsFinalizer)
	controllerutil.RemoveFinalizer(scope.OpenstackCreds, constants.OpenstackCredsFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OpenstackCredsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vjailbreakv1alpha1.OpenstackCreds{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
