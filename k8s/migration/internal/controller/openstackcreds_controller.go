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
}

// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=openstackcreds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=openstackcreds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=openstackcreds/finalizers,verbs=update

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
	ctxlog := log.FromContext(ctx)
	// Get the OpenstackCreds object
	openstackcreds := &vjailbreakv1alpha1.OpenstackCreds{}
	if err := r.Get(ctx, req.NamespacedName, openstackcreds); err != nil {
		if apierrors.IsNotFound(err) {
			ctxlog.Info("Received ignorable event for a recently deleted OpenstackCreds.")
			return ctrl.Result{}, nil
		}
		ctxlog.Error(err, fmt.Sprintf("Unexpected error reading OpenstackCreds '%s' object", openstackcreds.Name))
		return ctrl.Result{}, err
	}
	scope, err := scope.NewOpenstackCredsScope(scope.OpenstackCredsScopeParams{
		Logger:         ctxlog,
		Client:         r.Client,
		OpenstackCreds: openstackcreds,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always close the scope when exiting this function such that we can persist any OpenstackCreds changes.
	defer func() {
		if err := scope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	if !openstackcreds.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, scope)
	}
	return r.reconcileNormal(ctx, scope)
}

func (r *OpenstackCredsReconciler) reconcileNormal(ctx context.Context,
	scope *scope.OpenstackCredsScope) (ctrl.Result, error) { //nolint:unparam //future use
	ctxlog := log.FromContext(ctx).WithName(constants.OpenstackCredsControllerName)
	ctxlog.Info(fmt.Sprintf("Reconciling OpenstackCreds '%s'", scope.OpenstackCreds.Name))

	controllerutil.AddFinalizer(scope.OpenstackCreds, constants.OpenstackCredsFinalizer)

	// Check if spec matches with kubectl.kubernetes.io/last-applied-configuration
	ctxlog.Info(fmt.Sprintf("OpenstackCreds '%s' CR is being created or updated", scope.OpenstackCreds.Name))
	ctxlog.Info(fmt.Sprintf("Validating OpenstackCreds '%s' object", scope.OpenstackCreds.Name))
	if _, err := utils.ValidateAndGetProviderClient(ctx, scope.OpenstackCreds); err != nil {
		// Update the status of the OpenstackCreds object
		ctxlog.Error(err, fmt.Sprintf("Error validating OpenstackCreds '%s'", scope.OpenstackCreds.Name))
		scope.OpenstackCreds.Status.OpenStackValidationStatus = "Failed"
		scope.OpenstackCreds.Status.OpenStackValidationMessage = fmt.Sprintf("Error validating OpenstackCreds '%s'", scope.OpenstackCreds.Name)
		if err := r.Status().Update(ctx, scope.OpenstackCreds); err != nil {
			ctxlog.Error(err, fmt.Sprintf("Error updating status of OpenstackCreds '%s'", scope.OpenstackCreds.Name))
			return ctrl.Result{}, err
		}
	} else {
		err := utils.UpdateMasterNodeImageID(ctx, r.Client)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				ctxlog.Error(err, "failed to update master node image id and flavor list, skipping reconciliation")
			} else {
				return ctrl.Result{}, errors.Wrap(err, "failed to update master node image id")
			}
		}
		openstackCredential, err := utils.GetOpenstackCredentials(context.TODO(), scope.OpenstackCreds.Spec.SecretRef.Name)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to get Openstack credentials from secret")
		}

		ctxlog.Info(fmt.Sprintf("Getting flavors for OpenstackCreds '%s'", scope.OpenstackCreds.Name))
		flavors, err := utils.ListAllFlavors(ctx, scope.OpenstackCreds)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to get flavors")
		}
		ctxlog.Info(fmt.Sprintf("Successfully got flavors for OpenstackCreds '%s'", scope.OpenstackCreds.Name))
		ctxlog.Info(fmt.Sprintf("Setting flavors for OpenstackCreds '%v\n'", flavors))
		scope.OpenstackCreds.Spec.Flavors = flavors

		// Update the spec field
		if err = r.Client.Update(ctx, scope.OpenstackCreds); err != nil {
			ctxlog.Error(err, fmt.Sprintf("Error updating spec of OpenstackCreds '%s'", scope.OpenstackCreds.Name))
			return ctrl.Result{}, err
		}

		ctxlog.Info(fmt.Sprintf("Successfully authenticated to Openstack '%s'", openstackCredential.AuthURL))
		// Update the status of the OpenstackCreds object
		scope.OpenstackCreds.Status.OpenStackValidationStatus = "Succeeded"
		scope.OpenstackCreds.Status.OpenStackValidationMessage = "Successfully authenticated to Openstack"

		// update the status field openstackInfo
		openstackinfo, err := utils.GetOpenstackInfo(ctx, scope.OpenstackCreds)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to get Openstack info")
		}
		scope.OpenstackCreds.Status.Openstack = *openstackinfo
		if err := r.Status().Update(ctx, scope.OpenstackCreds); err != nil {
			ctxlog.Error(err, fmt.Sprintf("Error updating status of OpenstackCreds '%s'", scope.OpenstackCreds.Name))
			return ctrl.Result{}, err
		}

		// Now with these creds we should populate the flavors as labels in vmwaremachine object.
		// This will help us to create the vmwaremachine object with the correct flavor.
		vmwaremachineList := &vjailbreakv1alpha1.VMwareMachineList{}
		if err := r.Client.List(ctx, vmwaremachineList); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to list vmwaremachine objects")
		}
		for i := range vmwaremachineList.Items {
			vmwaremachine := &vmwaremachineList.Items[i]
			// Get the cpu and memory of the vmwaremachine object
			cpu := vmwaremachine.Spec.VMs.CPU
			memory := vmwaremachine.Spec.VMs.Memory
			computeClient, err := utils.GetOpenStackClients(context.TODO(), scope.OpenstackCreds)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to get OpenStack clients")
			}
			// Now get the closest flavor based on the cpu and memory
			flavor, err := utils.GetClosestFlavour(cpu, memory, computeClient.ComputeClient)
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
	}
	// Requeue to update the status of the OpenstackCreds object more specifically it will update flavors
	return ctrl.Result{Requeue: true, RequeueAfter: constants.OpenstackCredsRequeueAfter}, nil
}

func (r *OpenstackCredsReconciler) reconcileDelete(ctx context.Context,
	scope *scope.OpenstackCredsScope) (ctrl.Result, error) { //nolint:unparam //future use
	ctxlog := log.FromContext(ctx)
	ctxlog.Info(fmt.Sprintf("Reconciling OpenstackCreds '%s' deletion", scope.OpenstackCreds.Name))
	// Delete the associated secret
	err := r.Client.Delete(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scope.OpenstackCreds.Spec.SecretRef.Name,
			Namespace: constants.NamespaceMigrationSystem,
		},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrap(err, "failed to delete associated secret")
	}
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
