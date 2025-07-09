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
	"net"
	"os"
	"os/user"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"github.com/pkg/errors"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/scope"
	utils "github.com/platform9/vjailbreak/k8s/migration/pkg/utils"
	"github.com/platform9/vjailbreak/v2v-helper/vcenter"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
)

// VDDKDirectory is the path to VMware VDDK installation directory used for VM disk conversion
const VDDKDirectory = "/home/ubuntu/vmware-vix-disklib-distrib"

// MigrationPlanReconciler reconciles a MigrationPlan object
type MigrationPlanReconciler struct {
	*BaseReconciler
	ctxlog logr.Logger
}

// SetupWithManager sets up the controller with the Manager.
func (r *MigrationPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vjailbreakv1alpha1.MigrationPlan{}).
		Owns(&vjailbreakv1alpha1.Migration{}).
		Complete(r)
}

// NewMigrationPlanReconciler creates a new MigrationPlanReconciler
func NewMigrationPlanReconciler(client client.Client, scheme *runtime.Scheme) *MigrationPlanReconciler {
	r := &MigrationPlanReconciler{
		BaseReconciler: &BaseReconciler{
			Client: client,
			Scheme: scheme,
		},
	}
	r.ctxlog = ctrl.Log.WithName("controllers").WithName("MigrationPlan")
	return r
}

var migrationPlanFinalizer = "migrationplan.vjailbreak.pf9.io/finalizer"

// The default image. This is replaced by Go linker flags in the Dockerfile
var v2vimage = "platform9/v2v-helper:v0.1"

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=migrationplans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=migrationplans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=migrationplans/finalizers,verbs=update
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=migrationtemplates,verbs=get;list;watch

// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=migrationtemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=migrationtemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=migrationtemplates/finalizers,verbs=update

// Reconcile reads that state of the cluster for a MigrationPlan object and makes necessary changes
func (r *MigrationPlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	r.ctxlog = log.FromContext(ctx)
	migrationplan := &vjailbreakv1alpha1.MigrationPlan{}

	if err := r.Get(ctx, req.NamespacedName, migrationplan); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.ctxlog.Error(err, fmt.Sprintf("Unexpected error reading MigrationPlan '%s' object", migrationplan.Name))
		return ctrl.Result{}, err
	}

	err := utils.ValidateMigrationPlan(migrationplan)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to validate MigrationPlan")
	}

	migrationPlanScope, err := scope.NewMigrationPlanScope(scope.MigrationPlanScopeParams{
		Logger:        r.ctxlog,
		Client:        r.Client,
		MigrationPlan: migrationplan,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create scope: %w", err)
	}

	defer func() {
		if err := migrationPlanScope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	if !migrationplan.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, migrationPlanScope)
	}

	return r.reconcileNormal(ctx, migrationPlanScope)
}

func (r *MigrationPlanReconciler) reconcileNormal(ctx context.Context, scope *scope.MigrationPlanScope) (ctrl.Result, error) {
	migrationplan := scope.MigrationPlan
	log := scope.Logger
	log.Info(fmt.Sprintf("Reconciling MigrationPlan '%s'", migrationplan.Name))

	controllerutil.AddFinalizer(migrationplan, migrationPlanFinalizer)

	if res, err := r.ReconcileMigrationPlanJob(ctx, migrationplan, scope); err != nil {
		return res, err
	}
	return ctrl.Result{}, nil
}

//nolint:unparam //future use
func (r *MigrationPlanReconciler) reconcileDelete(
	ctx context.Context,
	scope *scope.MigrationPlanScope) (ctrl.Result, error) {
	migrationplan := scope.MigrationPlan
	ctxlog := log.FromContext(ctx).WithName(constants.MigrationControllerName)

	// The object is being deleted
	ctxlog.Info(fmt.Sprintf("MigrationPlan '%s' CR is being deleted", migrationplan.Name))

	// Now that the finalizer has completed deletion tasks, we can remove it
	// to allow deletion of the Migration object
	controllerutil.RemoveFinalizer(migrationplan, migrationPlanFinalizer)
	if err := r.Update(ctx, migrationplan); err != nil {
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MigrationPlanReconciler) getMigrationTemplateAndCreds(
	ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
) (*vjailbreakv1alpha1.MigrationTemplate, *vjailbreakv1alpha1.VMwareCreds, *corev1.Secret, error) {
	ctxlog := log.FromContext(ctx)

	migrationtemplate := &vjailbreakv1alpha1.MigrationTemplate{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      migrationplan.Spec.MigrationTemplate,
		Namespace: migrationplan.Namespace,
	}, migrationtemplate); err != nil {
		ctxlog.Error(err, "Failed to get MigrationTemplate")
		return nil, nil, nil, fmt.Errorf("failed to get MigrationTemplate: %w", err)
	}

	vmwcreds := &vjailbreakv1alpha1.VMwareCreds{}
	if ok, err := r.checkStatusSuccess(ctx, migrationtemplate.Namespace, migrationtemplate.Spec.Source.VMwareRef, true, vmwcreds); !ok {
		return nil, nil, nil, fmt.Errorf("VMwareCreds not validated: %w", err)
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      vmwcreds.Spec.SecretRef.Name,
		Namespace: migrationplan.Namespace,
	}, secret); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get vCenter Secret: %w", err)
	}

	return migrationtemplate, vmwcreds, secret, nil
}

func (r *MigrationPlanReconciler) reconcilePostMigration(ctx context.Context, scope *scope.MigrationPlanScope, vm string) error {
	migrationplan := scope.MigrationPlan
	ctxlog := log.FromContext(ctx).WithName(constants.MigrationControllerName)

	if migrationplan.Spec.PostMigrationAction == nil {
		ctxlog.Info("No post-migration actions configured")
		return nil
	}

	if migrationplan.Spec.PostMigrationAction.RenameVM == nil &&
		migrationplan.Spec.PostMigrationAction.MoveToFolder == nil {
		ctxlog.Info("No post-migration actions enabled")
		return nil
	}

	// Get required resources
	_, vmwcreds, secret, err := r.getMigrationTemplateAndCreds(ctx, migrationplan)
	if err != nil {
		return fmt.Errorf("failed to get migration resources: %w", err)
	}

	// Extract and validate credentials
	username, password, host, err := extractVCenterCredentials(secret)
	if err != nil {
		return fmt.Errorf("invalid vCenter credentials: %w", err)
	}

	// Create vCenter client and get datacenter
	vcClient, dc, err := createVCenterClientAndDC(ctx, host, username, password, vmwcreds.Spec.DataCenter)
	if err != nil {
		return fmt.Errorf("failed to create vCenter client: %w", err)
	}
	defer func() {
		if vcClient.VCClient != nil {
			sessionManager := session.NewManager(vcClient.VCClient)
			err = sessionManager.Logout(ctx) // Best effort logout
			if err != nil {
				ctxlog.Error(err, "Failed to logout from vCenter")
			}
		}
	}()

	if migrationplan.Spec.PostMigrationAction.RenameVM != nil && *migrationplan.Spec.PostMigrationAction.RenameVM {
		if err := r.renameVM(ctx, vcClient, migrationplan, vm); err != nil {
			return fmt.Errorf("failed to rename VM: %w", err)
		}
		vm += migrationplan.Spec.PostMigrationAction.Suffix
	}

	if migrationplan.Spec.PostMigrationAction.MoveToFolder != nil && *migrationplan.Spec.PostMigrationAction.MoveToFolder {
		if err := r.moveVMToFolder(ctx, vcClient, dc, migrationplan, vm); err != nil {
			return fmt.Errorf("failed to move VM to folder: %w", err)
		}
	}

	return nil
}

func (*MigrationPlanReconciler) renameVM(
	ctx context.Context,
	vcClient *vcenter.VCenterClient,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
	vm string,
) error {
	ctxlog := log.FromContext(ctx)
	suffix := migrationplan.Spec.PostMigrationAction.Suffix
	if suffix == "" {
		suffix = "_migrated_to_pcd"
		ctxlog.Info("Using default suffix", "suffix", suffix)
	}
	newVMName := vm + suffix
	ctxlog.Info("Renaming VM", "oldName", vm, "newName", newVMName)
	return vcClient.RenameVM(ctx, vm, newVMName)
}

func (*MigrationPlanReconciler) moveVMToFolder(
	ctx context.Context,
	vcClient *vcenter.VCenterClient,
	dc *object.Datacenter,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
	vm string,
) error {
	ctxlog := log.FromContext(ctx)
	folderName := migrationplan.Spec.PostMigrationAction.FolderName
	if folderName == "" {
		folderName = "vjailbreakedVMs"
		ctxlog.Info("Using default folder name", "folderName", folderName)
	}

	ctxlog.Info("Ensuring folder exists...", "folder", folderName)
	if _, err := EnsureVMFolderExists(ctx, vcClient.VCFinder, dc, folderName); err != nil {
		ctxlog.Error(err, "Folder creation/verification failed")
		return fmt.Errorf("failed to ensure folder '%s' exists: %w", folderName, err)
	}

	ctxlog.Info("Moving VM to folder", "vm", vm, "folder", folderName)
	if err := vcClient.MoveVMFolder(ctx, vm, folderName); err != nil {
		ctxlog.Error(err, "VM move failed")
		return fmt.Errorf("failed to move VM '%s' to folder '%s': %w", vm, folderName, err)
	}
	return nil
}

func createVCenterClientAndDC(
	ctx context.Context,
	host, username, password, datacenterName string,
) (*vcenter.VCenterClient, *object.Datacenter, error) {
	ctxlog := log.FromContext(ctx)
	ctxlog.Info("Creating vCenter client...", "host", host, "insecure", true)

	vcClient, err := vcenter.VCenterClientBuilder(ctx, username, password, host, true)
	if err != nil {
		ctxlog.Error(err, "Failed to create vCenter client")
		return nil, nil, fmt.Errorf("failed to create vCenter client: %w", err)
	}
	ctxlog.Info("vCenter client created successfully")

	ctxlog.Info("Using datacenter", "datacenter", datacenterName)
	dc, err := vcClient.VCFinder.Datacenter(ctx, datacenterName)
	if err != nil {
		ctxlog.Error(err, "Failed to find datacenter")
		return nil, nil, fmt.Errorf("failed to find datacenter '%s': %w", datacenterName, err)
	}
	ctxlog.Info("Datacenter located", "datacenter", dc)

	return vcClient, dc, nil
}

func extractVCenterCredentials(secret *corev1.Secret) (username, password, host string, err error) {
	u, ok := secret.Data["VCENTER_USERNAME"]
	if !ok {
		err = fmt.Errorf("username not found in secret")
		return
	}
	p, ok := secret.Data["VCENTER_PASSWORD"]
	if !ok {
		err = fmt.Errorf("password not found in secret")
		return
	}
	h, ok := secret.Data["VCENTER_HOST"]
	if !ok {
		err = fmt.Errorf("host not found in secret")
		return
	}
	username = string(u)
	password = string(p)
	host = string(h)
	return
}

// getMigrationCredentials fetches the required credentials for migration
// handleInitialMigrationStatus handles the initial status update for a migration
func (r *MigrationPlanReconciler) handleInitialMigrationStatus(ctx context.Context, migrationplan *vjailbreakv1alpha1.MigrationPlan) error {
	if migrationplan.Status.MigrationStatus == "" {
		if err := r.UpdateMigrationPlanStatus(ctx, migrationplan, corev1.PodRunning, "Migration(s) in progress"); err != nil {
			return fmt.Errorf("failed to update MigrationPlan status: %w", err)
		}
	}
	return nil
}

// validateVMsForMigration validates VMs before starting migration
func (r *MigrationPlanReconciler) validateVMsForMigration(ctx context.Context, migrationplan *vjailbreakv1alpha1.MigrationPlan, vmNames []string) error {
	for _, vmName := range vmNames {
		// Get VMwareMachine for this VM
		vmMachine := &vjailbreakv1alpha1.VMwareMachine{}
		if err := r.Get(ctx, types.NamespacedName{Name: vmName, Namespace: migrationplan.Namespace}, vmMachine); err != nil {
			return fmt.Errorf("failed to get VMwareMachine for VM '%s': %w", vmName, err)
		}

		// Log VM info for debugging
		r.ctxlog.Info("Validating VM",
			"vm", vmName,
			"numCPUs", vmMachine.Spec.VMInfo.CPU,
			"memoryMB", vmMachine.Spec.VMInfo.Memory)

		// Validate VM configuration
		if vmMachine.Spec.VMInfo.CPU < 1 {
			return fmt.Errorf("VM '%s' has invalid CPU count: %d", vmName, vmMachine.Spec.VMInfo.CPU)
		}

		if vmMachine.Spec.VMInfo.Memory < 1024 {
			return fmt.Errorf("VM '%s' has insufficient memory: %dMB (minimum 1024MB)", vmName, vmMachine.Spec.VMInfo.Memory)
		}

		// Validate MAC addresses
		if len(vmMachine.Spec.VMInfo.MacAddresses) == 0 {
			return fmt.Errorf("VM '%s' has no MAC addresses defined", vmName)
		}

		// Validate disks
		if len(vmMachine.Spec.VMInfo.Disks) == 0 {
			return fmt.Errorf("VM '%s' has no disks defined", vmName)
		}
	}
	return nil
}

// handlePausedMigration handles the paused state of a migration
func (r *MigrationPlanReconciler) handlePausedMigration(ctx context.Context, migrationplan *vjailbreakv1alpha1.MigrationPlan) (ctrl.Result, error) {
	migrationplan.Status.MigrationStatus = "Paused"
	migrationplan.Status.MigrationMessage = "Migration plan is paused"
	if err := r.Update(ctx, migrationplan); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update MigrationPlan status: %w", err)
	}
	return ctrl.Result{}, nil
}

// getMigrationCredentials fetches the required credentials for migration
func (r *MigrationPlanReconciler) getMigrationCredentials(ctx context.Context, migrationplan *vjailbreakv1alpha1.MigrationPlan) (*vjailbreakv1alpha1.MigrationTemplate, *vjailbreakv1alpha1.VMwareCreds, *vjailbreakv1alpha1.OpenstackCreds, error) {
	// Fetch MigrationTemplate CR
	migrationtemplate := &vjailbreakv1alpha1.MigrationTemplate{}
	if err := r.Get(ctx, types.NamespacedName{Name: migrationplan.Spec.MigrationTemplate, Namespace: migrationplan.Namespace},
		migrationtemplate); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get MigrationTemplate: %w", err)
	}

	// Fetch VMwareCreds CR
	vmwcreds := &vjailbreakv1alpha1.VMwareCreds{}
	if ok, err := r.checkStatusSuccess(ctx, migrationtemplate.Namespace, migrationtemplate.Spec.Source.VMwareRef, true, vmwcreds); !ok {
		return nil, nil, nil, err
	}

	// Fetch OpenStackCreds CR
	openstackcreds := &vjailbreakv1alpha1.OpenstackCreds{}
	if ok, err := r.checkStatusSuccess(ctx, migrationtemplate.Namespace, migrationtemplate.Spec.Destination.OpenstackRef,
		false, openstackcreds); !ok {
		return nil, nil, nil, err
	}

	return migrationtemplate, vmwcreds, openstackcreds, nil
}

// ReconcileMigrationPlanJob reconciles jobs created by the migration plan
func (r *MigrationPlanReconciler) ReconcileMigrationPlanJob(ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
	scope *scope.MigrationPlanScope) (ctrl.Result, error) {
	// If the plan is already in a terminal state, we don't need to do anything else.
	if migrationplan.Status.MigrationStatus == corev1.PodFailed || migrationplan.Status.MigrationStatus == corev1.PodSucceeded {
		r.ctxlog.Info("MigrationPlan is in a terminal state, reconciliation will be skipped.")
		return ctrl.Result{}, nil
	}

	migrationtemplate, vmwcreds, openstackcreds, err := r.getMigrationCredentials(ctx, migrationplan)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get migration credentials: %w", err)
	}
	if err := r.handleInitialMigrationStatus(ctx, migrationplan); err != nil {
		return ctrl.Result{}, err
	}
	if utils.IsMigrationPlanPaused(ctx, migrationplan.Name, r.Client) {
		return r.handlePausedMigration(ctx, migrationplan)
	}

	openstackClients, err := utils.GetOpenStackClients(ctx, r.Client, openstackcreds)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get OpenStack clients: %w", err)
	}

	// --- 1. Validation and Launch Stage ---
	// This loop ensures a Migration object exists for every VM and is in the correct initial state.
	for _, parallelvms := range migrationplan.Spec.VirtualMachines {
		// Restore this important pre-flight check for the VM's basic spec
		if err := r.validateVMsForMigration(ctx, migrationplan, parallelvms); err != nil {
			if statusUpdateErr := r.UpdateMigrationPlanStatus(ctx, migrationplan, corev1.PodFailed, err.Error()); statusUpdateErr != nil {
				r.ctxlog.Error(statusUpdateErr, "Failed to update MigrationPlan status after validation failure")
				return ctrl.Result{}, statusUpdateErr
			}
			return ctrl.Result{}, err
		}
		for _, vmName := range parallelvms {
			vmK8sName, err := utils.ConvertToK8sName(vmName)
			if err != nil {
				r.ctxlog.Error(err, "Could not convert VM name to a valid Kubernetes resource name; skipping this VM", "vmName", vmName)
				continue
			}
			existingMigration := &vjailbreakv1alpha1.Migration{}
			err = r.Get(ctx, types.NamespacedName{Name: utils.MigrationNameFromVMName(vmK8sName), Namespace: migrationplan.Namespace}, existingMigration)
			if err == nil {
				continue
			}
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("failed to check for existing migration for %s: %w", vmName, err)
			}
			conflictMsg, validationErr := r.validateVMInOpenStack(ctx, openstackClients, migrationtemplate, vmName, migrationplan.Namespace)
			if validationErr != nil {
				r.ctxlog.Error(validationErr, "A non-blocking error occurred during validation", "vm", vmName)
				continue
			}
			if err := r.createAndLaunchMigration(ctx, migrationplan, migrationtemplate, vmwcreds, openstackcreds, vmName, conflictMsg); err != nil {
				r.ctxlog.Error(err, "Failed to create and launch migration for VM", "vm", vmName)
				continue
			}
		}
	}

	// --- 2. Monitoring and Post-Action Stage ---
	// List all child migrations for this plan to check their status.
	allMigrations := &vjailbreakv1alpha1.MigrationList{}
	listOpts := []client.ListOption{
		client.InNamespace(migrationplan.Namespace),
		client.MatchingLabels{"migrationplan": migrationplan.Name},
	}
	if err := r.List(ctx, allMigrations, listOpts...); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list migration objects for plan: %w", err)
	}

	totalVMs := 0
	for _, batch := range migrationplan.Spec.VirtualMachines {
		totalVMs += len(batch)
	}

	if len(allMigrations.Items) != totalVMs {
		// Not all migration objects have been created yet, check again soon.
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	succeededCount := 0
	for _, migration := range allMigrations.Items {
		// Restore the post-migration logic call
		if migration.Status.Phase == vjailbreakv1alpha1.VMMigrationPhaseSucceeded {
			succeededCount++
			// Check if post-migration actions have already been completed for this VM
			if migration.Annotations["postMigrationActionCompleted"] != "true" {
				r.ctxlog.Info("Migration succeeded, running post-migration actions.", "vm", migration.Spec.VMName)
				if err := r.reconcilePostMigration(ctx, scope, migration.Spec.VMName); err != nil {
					r.ctxlog.Error(err, "Post-migration actions failed", "vm", migration.Spec.VMName)
					// You may want to update a condition on the Migration object here
				} else {
					// Annotate the migration to prevent running actions again
					if migration.Annotations == nil {
						migration.Annotations = make(map[string]string)
					}
					migration.Annotations["postMigrationActionCompleted"] = "true"
					if err := r.Update(ctx, &migration); err != nil {
						r.ctxlog.Error(err, "Failed to update migration with post-action annotation", "vm", migration.Spec.VMName)
					}
				}
			}
		} else if migration.Status.Phase == vjailbreakv1alpha1.VMMigrationPhaseFailed && (len(migration.Status.Conditions) > 0 && migration.Status.Conditions[0].Reason != "Blocked") {
			// A migration has failed for a reason other than being blocked by validation
			errMsg := fmt.Sprintf("Migration for VM %s has failed.", migration.Spec.VMName)
			return ctrl.Result{}, r.UpdateMigrationPlanStatus(ctx, migrationplan, corev1.PodFailed, errMsg)
		}
	}

	// If all migrations have succeeded, mark the plan as succeeded.
	if succeededCount == len(allMigrations.Items) {
		r.ctxlog.Info("All migrations for the plan have succeeded.")
		return ctrl.Result{}, r.UpdateMigrationPlanStatus(ctx, migrationplan, corev1.PodSucceeded, "All VMs in MigrationPlan have been successfully migrated")
	}

	// If we've reached here, migrations are still pending or running.
	if migrationplan.Status.MigrationStatus != corev1.PodRunning {
		return ctrl.Result{}, r.UpdateMigrationPlanStatus(ctx, migrationplan, corev1.PodRunning, "Migrations are in progress")
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// validateVMInOpenStack checks a single VM for MAC/IP conflicts in OpenStack.
// It returns a specific error starting with "CONFLICT:" if the migration should be blocked.
func (r *MigrationPlanReconciler) validateVMInOpenStack(
	ctx context.Context,
	openstackClients *utils.OpenStackClients,
	migrationtemplate *vjailbreakv1alpha1.MigrationTemplate,
	vmName string,
	namespace string,
) (string, error) { // Returns (conflict message, operational error)
	log := log.FromContext(ctx)

	vmMachine := &vjailbreakv1alpha1.VMwareMachine{}
	if err := r.Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, vmMachine); err != nil {
		return "", fmt.Errorf("failed to get VMwareMachine for VM '%s': %w", vmName, err)
	}

	log.Info("Validating VM in OpenStack", "vm", vmName)

	// Check for MAC address conflicts
	for _, mac := range vmMachine.Spec.VMInfo.MacAddresses {
		if mac == "" {
			continue
		}
		macAllocated, err := utils.IsMacAllocatedInOpenStack(ctx, openstackClients.NetworkingClient, mac)
		if err != nil {
			return "", fmt.Errorf("failed to check MAC allocation: %w", err)
		}
		if macAllocated {
			return fmt.Sprintf("CONFLICT:MAC_ALREADY_ALLOCATED:MAC %s is already allocated", mac), nil
		}
	}

	// Validate IP address if present
	if ip := vmMachine.Spec.VMInfo.IPAddress; ip != "" {
		// Find the target subnet
		subnetID, err := r.findSubnetForIP(ctx, openstackClients.NetworkingClient, migrationtemplate, vmMachine)
		if err != nil {
			// Return configuration errors as conflicts
			return fmt.Sprintf("CONFLICT:CONFIGURATION_ERROR:%s", err.Error()), nil
		}
		if subnetID == "" {
			// This can happen if the network is not mapped; we treat it as a config conflict.
			return fmt.Sprintf("CONFLICT:NETWORK_NOT_MAPPED:Cannot validate IP %s; network not found in NetworkMapping", ip), nil
		}

		// Check if IP is in the allocation pool
		inPool, err := utils.IsIPInAllocationPool(ctx, openstackClients.NetworkingClient, subnetID, ip)
		if err != nil {
			return "", fmt.Errorf("error checking IP allocation pool: %w", err)
		}
		if !inPool {
			return fmt.Sprintf("CONFLICT:IP_NOT_IN_ALLOCATION_POOL:IP %s is not in the allocation pool of subnet %s", ip, subnetID), nil
		}

		// Check if IP is already allocated
		allocated, err := utils.IsIPAllocatedInOpenStack(ctx, openstackClients.NetworkingClient, ip)
		if err != nil {
			return "", fmt.Errorf("failed to check IP allocation: %w", err)
		}
		if allocated {
			return fmt.Sprintf("CONFLICT:IP_ALREADY_ALLOCATED:IP %s is already allocated", ip), nil
		}
	}

	return "", nil // No conflicts found
}

// createAndLaunchMigration handles the creation of the Migration resource and the conditional launch of the migration Job.
func (r *MigrationPlanReconciler) createAndLaunchMigration(
	ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
	migrationtemplate *vjailbreakv1alpha1.MigrationTemplate,
	vmwcreds *vjailbreakv1alpha1.VMwareCreds,
	openstackcreds *vjailbreakv1alpha1.OpenstackCreds,
	vmName string,
	conflictMsg string,
) error {
	log := log.FromContext(ctx)

	vmMachine := &vjailbreakv1alpha1.VMwareMachine{}
	if err := r.Get(ctx, types.NamespacedName{Name: vmName, Namespace: migrationplan.Namespace}, vmMachine); err != nil {
		return fmt.Errorf("failed to get VMwareMachine %s: %w", vmName, err)
	}

	// Step 1: Always create the Migration object
	migrationObj, err := r.CreateMigration(ctx, migrationplan, vmName, vmMachine)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create migration object for %s: %w", vmName, err)
	}

	// Step 2: If there's a conflict, update status and STOP.
	if conflictMsg != "" {
		log.Info("Blocking migration due to validation conflict", "vm", vmName, "reason", conflictMsg)
		migrationObj.Status.Phase = vjailbreakv1alpha1.VMMigrationPhaseFailed
		migrationObj.Status.Conditions = []corev1.PodCondition{{
			Type:               "Validation",
			Status:             corev1.ConditionFalse,
			Reason:             "Blocked",
			Message:            conflictMsg,
			LastTransitionTime: metav1.Now(),
		}}
		return r.Status().Update(ctx, migrationObj)
	}

	// Step 3: If no conflict, proceed to create ConfigMaps and the Job.
	log.Info("Validation successful, launching migration job", "vm", vmName)

	_, err = r.CreateMigrationConfigMap(ctx, migrationplan, migrationtemplate, migrationObj, openstackcreds, vmwcreds, vmName, vmMachine)
	if err != nil {
		return fmt.Errorf("failed to create ConfigMap for VM %s: %w", vmName, err)
	}
	fbcm, err := r.CreateFirstbootConfigMap(ctx, migrationplan, vmName)
	if err != nil {
		return fmt.Errorf("failed to create Firstboot ConfigMap for VM %s: %w", vmName, err)
	}
	if err = r.validateVDDKPresence(ctx, migrationObj, log); err != nil {
		return err
	}

	return r.CreateJob(ctx,
		migrationplan,
		migrationObj,
		vmName,
		fbcm.Name,
		vmwcreds.Spec.SecretRef.Name,
		openstackcreds.Spec.SecretRef.Name)
}

// findSubnetForIP locates the correct OpenStack subnet ID for a given VM's IP address.
func (r *MigrationPlanReconciler) findSubnetForIP(ctx context.Context, networkingClient *gophercloud.ServiceClient, migrationtemplate *vjailbreakv1alpha1.MigrationTemplate, vmMachine *vjailbreakv1alpha1.VMwareMachine) (string, error) {
	ip := vmMachine.Spec.VMInfo.IPAddress
	networkMapping := &vjailbreakv1alpha1.NetworkMapping{}
	if err := r.Get(ctx, types.NamespacedName{Name: migrationtemplate.Spec.NetworkMapping, Namespace: vmMachine.Namespace}, networkMapping); err != nil {
		return "", fmt.Errorf("failed to get NetworkMapping")
	}

	for _, srcNet := range vmMachine.Spec.VMInfo.Networks {
		var openstackNetName string
		for _, mapping := range networkMapping.Spec.Networks {
			if mapping.Source == srcNet {
				openstackNetName = mapping.Target
				break
			}
		}

		if openstackNetName == "" {
			continue
		}

		netList, err := networks.List(networkingClient, networks.ListOpts{Name: openstackNetName}).AllPages()
		if err != nil {
			return "", fmt.Errorf("failed to list OpenStack networks")
		}
		nets, err := networks.ExtractNetworks(netList)
		if err != nil || len(nets) == 0 {
			continue // Try next network if this one isn't found
		}

		subnetList, err := subnets.List(networkingClient, subnets.ListOpts{NetworkID: nets[0].ID}).AllPages()
		if err != nil {
			return "", fmt.Errorf("failed to list subnets for network %s", openstackNetName)
		}
		allSubnets, err := subnets.ExtractSubnets(subnetList)
		if err != nil {
			return "", fmt.Errorf("failed to extract subnets")
		}

		for _, subnet := range allSubnets {
			if _, ipNet, err := net.ParseCIDR(subnet.CIDR); err == nil {
				if ipNet.Contains(net.ParseIP(ip)) {
					return subnet.ID, nil // Found it
				}
			}
		}
	}
	return "", nil // Return empty string if not found in any mapped network
}

// UpdateMigrationPlanStatus updates the status of a MigrationPlan
func (r *MigrationPlanReconciler) UpdateMigrationPlanStatus(
	ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
	status corev1.PodPhase,
	message string,
) error {
	migrationplan.Status.MigrationStatus = status
	migrationplan.Status.MigrationMessage = message

	// Add condition for better status tracking
	condition := corev1.PodCondition{
		Type:               "MigrationStatus",
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Message:            message,
	}

	// Set appropriate condition reason based on status
	switch status {
	case corev1.PodFailed:
		condition.Reason = "Failed"
		if strings.HasPrefix(message, "CONFLICT:") {
			condition.Reason = "Blocked"
		}
	case corev1.PodRunning:
		condition.Reason = "InProgress"
	case corev1.PodSucceeded:
		condition.Reason = "Succeeded"
	default:
		condition.Reason = string(status)
	}

	migrationplan.Status.Conditions = []corev1.PodCondition{condition}

	err := r.Status().Update(ctx, migrationplan)
	if err != nil {
		return fmt.Errorf("failed to update MigrationPlan status: %w", err)
	}
	return nil
}

// CreateMigration creates a new Migration resource
func (r *MigrationPlanReconciler) CreateMigration(ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
	vm string, vmMachine *vjailbreakv1alpha1.VMwareMachine) (*vjailbreakv1alpha1.Migration, error) {
	ctxlog := r.ctxlog.WithValues("vm", vm)
	ctxlog.Info("Creating Migration for VM")

	vmname, err := utils.ConvertToK8sName(vm)
	if err != nil {
		return nil, fmt.Errorf("failed to convert VM name: %w", err)
	}
	vminfo := &vmMachine.Spec.VMInfo

	migrationobj := &vjailbreakv1alpha1.Migration{}
	err = r.Get(ctx, types.NamespacedName{Name: utils.MigrationNameFromVMName(vmname), Namespace: migrationplan.Namespace}, migrationobj)
	if err != nil && apierrors.IsNotFound(err) {
		migrationobj = &vjailbreakv1alpha1.Migration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.MigrationNameFromVMName(vmname),
				Namespace: migrationplan.Namespace,
				Labels: map[string]string{
					"migrationplan":              migrationplan.Name,
					constants.NumberOfDisksLabel: strconv.Itoa(len(vminfo.Disks)),
				},
			},
			Spec: vjailbreakv1alpha1.MigrationSpec{
				MigrationPlan: migrationplan.Name,
				VMName:        vm,
				// PodRef will be set in the migration controller
				PodRef:          fmt.Sprintf("v2v-helper-%s", vmname),
				InitiateCutover: !migrationplan.Spec.MigrationStrategy.AdminInitiatedCutOver,
			},
		}
		migrationobj.Labels = MergeLabels(migrationobj.Labels, migrationplan.Labels)
		err = r.createResource(ctx, migrationplan, migrationobj)
		if err != nil {
			return nil, fmt.Errorf("failed to create Migration for VM %s: %w", vm, err)
		}
	}
	return migrationobj, nil
}

// CreateJob creates a job to run v2v-helper
func (r *MigrationPlanReconciler) CreateJob(ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
	migrationobj *vjailbreakv1alpha1.Migration,
	vm string,
	firstbootconfigMapName string,
	vmwareSecretRef string,
	openstackSecretRef string) error {
	vmname, err := utils.ConvertToK8sName(vm)
	if err != nil {
		return fmt.Errorf("failed to convert VM name: %w", err)
	}
	jobName := fmt.Sprintf("v2v-helper-%s", vmname)
	pointtrue := true
	cutoverlabel := "yes"
	if migrationplan.Spec.MigrationStrategy.AdminInitiatedCutOver {
		cutoverlabel = "no"
	}
	job := &batchv1.Job{}
	err = r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: migrationplan.Namespace}, job)
	if err != nil && apierrors.IsNotFound(err) {
		r.ctxlog.Info(fmt.Sprintf("Creating new Job '%s' for VM '%s'", jobName, vmname))
		job = &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: migrationplan.Namespace,
			},
			Spec: batchv1.JobSpec{
				PodFailurePolicy: &batchv1.PodFailurePolicy{
					Rules: []batchv1.PodFailurePolicyRule{
						{
							Action: batchv1.PodFailurePolicyActionFailJob,
							OnExitCodes: &batchv1.PodFailurePolicyOnExitCodesRequirement{
								Values:   []int32{0},
								Operator: batchv1.PodFailurePolicyOnExitCodesOpNotIn,
							},
						},
					},
				},
				TTLSecondsAfterFinished: nil,
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"vm-name":      vmname,
							"startCutover": cutoverlabel,
						},
					},
					Spec: corev1.PodSpec{
						RestartPolicy:                 corev1.RestartPolicyNever,
						ServiceAccountName:            "migration-controller-manager",
						TerminationGracePeriodSeconds: ptr.To(constants.TerminationPeriod),
						HostNetwork:                   true,
						Containers: []corev1.Container{
							{
								Name:            "fedora",
								Image:           v2vimage,
								ImagePullPolicy: corev1.PullIfNotPresent,
								Command:         []string{"/home/fedora/manager"},
								SecurityContext: &corev1.SecurityContext{
									Privileged: &pointtrue,
								},
								Env: []corev1.EnvVar{
									{
										Name: "POD_NAME",
										ValueFrom: &corev1.EnvVarSource{
											FieldRef: &corev1.ObjectFieldSelector{
												FieldPath: "metadata.name",
											},
										},
									},
									{
										Name:  "SOURCE_VM_NAME",
										Value: vm,
									},
								},
								EnvFrom: []corev1.EnvFromSource{
									{
										SecretRef: &corev1.SecretEnvSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: vmwareSecretRef,
											},
										},
									},
									{
										SecretRef: &corev1.SecretEnvSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: openstackSecretRef,
											},
										},
									},
									{
										ConfigMapRef: &corev1.ConfigMapEnvSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "pf9-env",
											},
										},
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "vddk",
										MountPath: "/home/fedora/vmware-vix-disklib-distrib",
									},
									{
										Name:      "dev",
										MountPath: "/dev",
									},
									{
										Name:      "firstboot",
										MountPath: "/home/fedora/scripts",
									},
									{
										Name:      "logs",
										MountPath: "/var/log/pf9",
									},
									{
										Name:      "virtio-driver",
										MountPath: "/home/fedora/virtio-win",
									},
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:              resource.MustParse("1000m"),
										corev1.ResourceMemory:           resource.MustParse("1Gi"),
										corev1.ResourceEphemeralStorage: resource.MustParse("3Gi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:              resource.MustParse("2000m"),
										corev1.ResourceMemory:           resource.MustParse("3Gi"),
										corev1.ResourceEphemeralStorage: resource.MustParse("3Gi"),
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "vddk",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/home/ubuntu/vmware-vix-disklib-distrib",
										Type: utils.NewHostPathType("Directory"),
									},
								},
							},
							{
								Name: "dev",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/dev",
										Type: utils.NewHostPathType("Directory"),
									},
								},
							},
							{
								Name: "firstboot",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: firstbootconfigMapName,
										},
									},
								},
							},
							{
								Name: "logs",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/var/log/pf9",
										Type: utils.NewHostPathType("DirectoryOrCreate"),
									},
								},
							},
							{
								Name: "virtio-driver",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/home/ubuntu/virtio-win",
										Type: utils.NewHostPathType("DirectoryOrCreate"),
									},
								},
							},
						},
					},
				},
			},
		}
		if err := r.createResource(ctx, migrationobj, job); err != nil {
			r.ctxlog.Error(err, fmt.Sprintf("Failed to create Job '%s'", jobName))
			return err
		}
	}
	return nil
}

// CreateFirstbootConfigMap creates a firstboot config map for migration
func (r *MigrationPlanReconciler) CreateFirstbootConfigMap(ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan, vm string) (*corev1.ConfigMap, error) {
	vmname, err := utils.ConvertToK8sName(vm)
	if err != nil {
		return nil, fmt.Errorf("failed to convert VM name: %w", err)
	}
	configMapName := fmt.Sprintf("firstboot-config-%s", vmname)
	configMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: migrationplan.Namespace}, configMap)
	if err != nil && apierrors.IsNotFound(err) {
		r.ctxlog.Info(fmt.Sprintf("Creating new ConfigMap '%s' for VM '%s'", configMapName, vmname))
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: migrationplan.Namespace,
			},
			Data: map[string]string{
				"user_firstboot.sh": migrationplan.Spec.FirstBootScript,
			},
		}
		err = r.createResource(ctx, migrationplan, configMap)
		if err != nil {
			r.ctxlog.Error(err, fmt.Sprintf("Failed to create ConfigMap '%s'", configMapName))
			return nil, err
		}
	}
	return configMap, nil
}

// CreateMigrationConfigMap creates a config map for migration
func (r *MigrationPlanReconciler) CreateMigrationConfigMap(ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
	migrationtemplate *vjailbreakv1alpha1.MigrationTemplate,
	migrationobj *vjailbreakv1alpha1.Migration,
	openstackcreds *vjailbreakv1alpha1.OpenstackCreds,
	vmwcreds *vjailbreakv1alpha1.VMwareCreds, vm string, vmMachine *vjailbreakv1alpha1.VMwareMachine) (*corev1.ConfigMap, error) {
	vmname, err := utils.ConvertToK8sName(vm)
	if err != nil {
		return nil, fmt.Errorf("failed to convert VM name: %w", err)
	}
	configMapName := utils.GetMigrationConfigMapName(vmname)
	virtiodrivers := ""
	if migrationtemplate.Spec.VirtioWinDriver == "" {
		virtiodrivers = "https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso"
	} else {
		virtiodrivers = migrationtemplate.Spec.VirtioWinDriver
	}
	openstacknws, openstackvolumetypes, err := r.reconcileMapping(ctx, migrationtemplate, openstackcreds, vmwcreds, vm)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile mapping: %w", err)
	}

	openstackports := []string{}
	// If advanced options are set, replace the networks and/or volume types with the ones in the advanced options
	if !reflect.DeepEqual(migrationplan.Spec.AdvancedOptions, vjailbreakv1alpha1.AdvancedOptions{}) {
		if len(migrationplan.Spec.AdvancedOptions.GranularNetworks) > 0 {
			if err = utils.VerifyNetworks(ctx, r.Client, openstackcreds, migrationplan.Spec.AdvancedOptions.GranularNetworks); err != nil {
				return nil, fmt.Errorf("failed to verify networks in advanced mapping: %w", err)
			}
			openstacknws = migrationplan.Spec.AdvancedOptions.GranularNetworks
		}
		if len(migrationplan.Spec.AdvancedOptions.GranularVolumeTypes) > 0 {
			if err = utils.VerifyStorage(ctx, r.Client, openstackcreds, migrationplan.Spec.AdvancedOptions.GranularVolumeTypes); err != nil {
				return nil, fmt.Errorf("failed to verify volume types in advanced mapping: %w", err)
			}
			openstackvolumetypes = migrationplan.Spec.AdvancedOptions.GranularVolumeTypes
		}
		if len(migrationplan.Spec.AdvancedOptions.GranularPorts) > 0 {
			if err = utils.VerifyPorts(ctx, r.Client, openstackcreds, migrationplan.Spec.AdvancedOptions.GranularPorts); err != nil {
				return nil, fmt.Errorf("failed to verify ports in advanced mapping: %w", err)
			}
			openstackports = migrationplan.Spec.AdvancedOptions.GranularPorts
		}
	}

	// Create MigrationConfigMap
	configMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: migrationplan.Namespace}, configMap)
	if err != nil && apierrors.IsNotFound(err) {
		r.ctxlog.Info(fmt.Sprintf("Creating new ConfigMap '%s' for VM '%s'", configMapName, vmname))
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: migrationplan.Namespace,
			},
			Data: map[string]string{
				"CONVERT":               "true", // Assume that the vm always has to be converted
				"TYPE":                  migrationplan.Spec.MigrationStrategy.Type,
				"DATACOPYSTART":         migrationplan.Spec.MigrationStrategy.DataCopyStart.Format(time.RFC3339),
				"CUTOVERSTART":          migrationplan.Spec.MigrationStrategy.VMCutoverStart.Format(time.RFC3339),
				"CUTOVEREND":            migrationplan.Spec.MigrationStrategy.VMCutoverEnd.Format(time.RFC3339),
				"NEUTRON_NETWORK_NAMES": strings.Join(openstacknws, ","),
				"NEUTRON_PORT_IDS":      strings.Join(openstackports, ","),
				"CINDER_VOLUME_TYPES":   strings.Join(openstackvolumetypes, ","),
				"VIRTIO_WIN_DRIVER":     virtiodrivers,
				"PERFORM_HEALTH_CHECKS": strconv.FormatBool(migrationplan.Spec.MigrationStrategy.PerformHealthChecks),
				"HEALTH_CHECK_PORT":     migrationplan.Spec.MigrationStrategy.HealthCheckPort,
			},
		}
		if utils.IsOpenstackPCD(*openstackcreds) {
			configMap.Data["TARGET_AVAILABILITY_ZONE"] = migrationtemplate.Spec.TargetPCDClusterName
		}

		// Check if assigned IP is set
		if vmMachine.Spec.VMInfo.AssignedIP != "" {
			configMap.Data["ASSIGNED_IP"] = vmMachine.Spec.VMInfo.AssignedIP
		} else {
			configMap.Data["ASSIGNED_IP"] = ""
		}

		// Check if target flavor is set
		if vmMachine.Spec.TargetFlavorID != "" {
			configMap.Data["TARGET_FLAVOR_ID"] = vmMachine.Spec.TargetFlavorID
		} else {
			var computeClient *utils.OpenStackClients
			// If target flavor is not set, use the closest matching flavor
			computeClient, err = utils.GetOpenStackClients(ctx, r.Client, openstackcreds)
			if err != nil {
				return nil, fmt.Errorf("failed to get OpenStack clients: %w", err)
			}
			var flavor *flavors.Flavor
			flavor, err = utils.GetClosestFlavour(ctx, vmMachine.Spec.VMInfo.CPU, vmMachine.Spec.VMInfo.Memory, computeClient.ComputeClient)
			if err != nil {
				return nil, fmt.Errorf("failed to get closest flavor: %w", err)
			}
			if flavor == nil {
				return nil, fmt.Errorf("no suitable flavor found for %d vCPUs and %d MB RAM", vmMachine.Spec.VMInfo.CPU, vmMachine.Spec.VMInfo.Memory)
			}
			configMap.Data["TARGET_FLAVOR_ID"] = flavor.ID
		}

		if vmMachine.Spec.VMInfo.OSFamily == "" {
			return nil, fmt.Errorf(
				"OSFamily is not available for the VM '%s', "+
					"cannot perform the migration. Please set OSFamily explicitly in the VMwareMachine CR",
				vmMachine.Name)
		}

		configMap.Data["OS_FAMILY"] = vmMachine.Spec.VMInfo.OSFamily

		if migrationtemplate.Spec.OSFamily != "" {
			configMap.Data["OS_FAMILY"] = migrationtemplate.Spec.OSFamily
		}

		err = r.createResource(ctx, migrationobj, configMap)
		if err != nil {
			r.ctxlog.Error(err, fmt.Sprintf("Failed to create ConfigMap '%s'", configMapName))
			return nil, err
		}
	}
	return configMap, nil
}

func (r *MigrationPlanReconciler) createResource(ctx context.Context, owner metav1.Object, controlled client.Object) error {
	err := ctrl.SetControllerReference(owner, controlled, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	err = r.Create(ctx, controlled)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}
	return nil
}

//nolint:dupl // Same logic to migrationtemplate reconciliation, excluding from linting to keep both reconcilers separate
func (r *MigrationPlanReconciler) checkStatusSuccess(ctx context.Context,
	namespace, credsname string,
	isvmware bool,
	credsobj client.Object) (bool, error) {
	client := r.Client
	err := client.Get(ctx, types.NamespacedName{Name: credsname, Namespace: namespace}, credsobj)
	if err != nil {
		return false, fmt.Errorf("failed to get Creds: %w", err)
	}

	if isvmware {
		vmwareCreds, ok := credsobj.(*vjailbreakv1alpha1.VMwareCreds)
		if !ok {
			return false, fmt.Errorf("failed to convert credentials to VMwareCreds: %w", err)
		}
		if vmwareCreds.Status.VMwareValidationStatus != string(corev1.PodSucceeded) {
			return false, fmt.Errorf("vmwarecreds '%s' CR is not validated", vmwareCreds.Name)
		}
	} else {
		openstackCreds, ok := credsobj.(*vjailbreakv1alpha1.OpenstackCreds)
		if !ok {
			return false, fmt.Errorf("failed to convert credentials to OpenstackCreds: %w", err)
		}
		if openstackCreds.Status.OpenStackValidationStatus != string(corev1.PodSucceeded) {
			return false, fmt.Errorf("openstackcreds '%s' CR is not validated", openstackCreds.Name)
		}
	}
	return true, nil
}

func (r *MigrationPlanReconciler) reconcileMapping(ctx context.Context,
	migrationtemplate *vjailbreakv1alpha1.MigrationTemplate,
	openstackcreds *vjailbreakv1alpha1.OpenstackCreds,
	vmwcreds *vjailbreakv1alpha1.VMwareCreds,
	vm string) (openstacknws, openstackvolumetypes []string, err error) {
	openstacknws, err = r.reconcileNetwork(ctx, migrationtemplate, openstackcreds, vmwcreds, vm)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to reconcile network: %w", err)
	}
	openstackvolumetypes, err = r.reconcileStorage(ctx, migrationtemplate, vmwcreds, openstackcreds, vm)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to reconcile storage: %w", err)
	}
	return openstacknws, openstackvolumetypes, nil
}

//nolint:dupl // Similar logic to storages reconciliation, excluding from linting to keep it readable
func (r *MigrationPlanReconciler) reconcileNetwork(ctx context.Context,
	migrationtemplate *vjailbreakv1alpha1.MigrationTemplate,
	openstackcreds *vjailbreakv1alpha1.OpenstackCreds,
	vmwcreds *vjailbreakv1alpha1.VMwareCreds,
	vm string) ([]string, error) {
	vmnws, err := utils.GetVMwNetworks(ctx, r.Client, vmwcreds, vmwcreds.Spec.DataCenter, vm)
	if err != nil {
		return nil, fmt.Errorf("failed to get network: %w", err)
	}
	// Fetch the networkmap
	networkmap := &vjailbreakv1alpha1.NetworkMapping{}
	err = r.Get(ctx, types.NamespacedName{Name: migrationtemplate.Spec.NetworkMapping, Namespace: migrationtemplate.Namespace}, networkmap)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve NetworkMapping CR: %w", err)
	}

	openstacknws := []string{}
	for _, vmnw := range vmnws {
		for _, nwm := range networkmap.Spec.Networks {
			if vmnw == nwm.Source {
				openstacknws = append(openstacknws, nwm.Target)
			}
		}
	}
	if len(openstacknws) != len(vmnws) {
		return nil, fmt.Errorf("VMware Network(s) not found in NetworkMapping vm(%d) openstack(%d)", len(vmnws), len(openstacknws))
	}

	if networkmap.Status.NetworkmappingValidationStatus != string(corev1.PodSucceeded) {
		err = utils.VerifyNetworks(ctx, r.Client, openstackcreds, openstacknws)
		if err != nil {
			return nil, fmt.Errorf("failed to verify networks: %w", err)
		}
		networkmap.Status.NetworkmappingValidationStatus = string(corev1.PodSucceeded)
		networkmap.Status.NetworkmappingValidationMessage = "NetworkMapping validated"
		err = r.Status().Update(ctx, networkmap)
		if err != nil {
			return nil, fmt.Errorf("failed to update networkmapping status: %w", err)
		}
	}
	return openstacknws, nil
}

//nolint:dupl // Similar logic to networks reconciliation, excluding from linting to keep it readable
func (r *MigrationPlanReconciler) reconcileStorage(ctx context.Context,
	migrationtemplate *vjailbreakv1alpha1.MigrationTemplate,
	vmwcreds *vjailbreakv1alpha1.VMwareCreds,
	openstackcreds *vjailbreakv1alpha1.OpenstackCreds,
	vm string) ([]string, error) {
	vmds, err := utils.GetVMwDatastore(ctx, r.Client, vmwcreds, vmwcreds.Spec.DataCenter, vm)
	if err != nil {
		return nil, fmt.Errorf("failed to get datastores: %w", err)
	}
	// Fetch the StorageMap
	storagemap := &vjailbreakv1alpha1.StorageMapping{}
	err = r.Get(ctx, types.NamespacedName{Name: migrationtemplate.Spec.StorageMapping, Namespace: migrationtemplate.Namespace}, storagemap)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve StorageMapping CR: %w", err)
	}

	openstackvolumetypes := []string{}
	for _, vmdatastore := range vmds {
		for _, storagemaptype := range storagemap.Spec.Storages {
			if vmdatastore == storagemaptype.Source {
				openstackvolumetypes = append(openstackvolumetypes, storagemaptype.Target)
			}
		}
	}
	if len(openstackvolumetypes) != len(vmds) {
		return nil, fmt.Errorf("VMware Datastore(s) not found in StorageMapping vm(%d) openstack(%d)", len(vmds), len(openstackvolumetypes))
	}
	if storagemap.Status.StoragemappingValidationStatus != string(corev1.PodSucceeded) {
		err = utils.VerifyStorage(ctx, r.Client, openstackcreds, openstackvolumetypes)
		if err != nil {
			return nil, fmt.Errorf("failed to verify datastores: %w", err)
		}
		storagemap.Status.StoragemappingValidationStatus = string(corev1.PodSucceeded)
		storagemap.Status.StoragemappingValidationMessage = "StorageMapping validated"
		err = r.Status().Update(ctx, storagemap)
		if err != nil {
			return nil, fmt.Errorf("failed to update storagemapping status: %w", err)
		}
	}
	return openstackvolumetypes, nil
}

// TriggerMigration triggers a migration process
func (r *MigrationPlanReconciler) TriggerMigration(ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
	migrationobjs *vjailbreakv1alpha1.MigrationList,
	openstackcreds *vjailbreakv1alpha1.OpenstackCreds,
	vmwcreds *vjailbreakv1alpha1.VMwareCreds,
	migrationtemplate *vjailbreakv1alpha1.MigrationTemplate,
	parallelvms []string) error {
	ctxlog := r.ctxlog.WithValues("migrationplan", migrationplan.Name)
	var (
		fbcm *corev1.ConfigMap
	)

	nodeList := &corev1.NodeList{}
	err := r.List(ctx, nodeList)
	if err != nil {
		return errors.Wrap(err, "failed to list nodes")
	}
	counter := len(nodeList.Items)

	vmMachines := &vjailbreakv1alpha1.VMwareMachineList{}

	err = r.List(ctx, vmMachines, &client.ListOptions{Namespace: migrationtemplate.Namespace, LabelSelector: labels.SelectorFromSet(map[string]string{constants.VMwareCredsLabel: vmwcreds.Name})})
	if err != nil {
		return errors.Wrap(err, "failed to list vmwaremachines")
	}

	var vmMachineObj *vjailbreakv1alpha1.VMwareMachine
	for _, vm := range parallelvms {
		vmMachineObj = nil
		for i := range vmMachines.Items {
			if vmMachines.Items[i].Spec.VMInfo.Name == vm {
				vmMachineObj = &vmMachines.Items[i]
				break
			}
		}
		if vmMachineObj == nil {
			return errors.Wrap(fmt.Errorf("VM '%s' not found in VMwareMachine", vm), "failed to find vmwaremachine")
		}
		migrationobj, err := r.CreateMigration(ctx, migrationplan, vm, vmMachineObj)
		if err != nil {
			if apierrors.IsAlreadyExists(err) && migrationobj.Status.Phase == vjailbreakv1alpha1.VMMigrationPhaseSucceeded {
				r.ctxlog.Info(fmt.Sprintf("Migration for VM '%s' already exists", vm))
				continue
			}
			return fmt.Errorf("failed to create Migration for VM %s: %w", vm, err)
		}
		migrationobjs.Items = append(migrationobjs.Items, *migrationobj)
		_, err = r.CreateMigrationConfigMap(ctx, migrationplan, migrationtemplate, migrationobj, openstackcreds, vmwcreds, vm, vmMachineObj)
		if err != nil {
			return fmt.Errorf("failed to create ConfigMap for VM %s: %w", vm, err)
		}
		fbcm, err = r.CreateFirstbootConfigMap(ctx, migrationplan, vm)
		if err != nil {
			return fmt.Errorf("failed to create Firstboot ConfigMap for VM %s: %w", vm, err)
		}
		//nolint:gocritic // err is already declared above
		if err = r.validateVDDKPresence(ctx, migrationobj, ctxlog); err != nil {
			return err
		}

		err = r.CreateJob(ctx,
			migrationplan,
			migrationobj,
			vm,
			fbcm.Name,
			vmwcreds.Spec.SecretRef.Name,
			openstackcreds.Spec.SecretRef.Name)
		if err != nil {
			return fmt.Errorf("failed to create Job for VM %s: %w", vm, err)
		}
		counter--

		if counter == 0 {
			// Control the number of VMs in parallel
			counter = len(nodeList.Items)
			time.Sleep(constants.MigrationTriggerDelay)
		}
	}
	return nil
}

func (r *MigrationPlanReconciler) validateVDDKPresence(
	ctx context.Context,
	migrationobj *vjailbreakv1alpha1.Migration,
	logger logr.Logger,
) error {
	currentUser, err := user.Current()
	whoami := "unknown"
	if err == nil {
		whoami = currentUser.Username
	}

	oldConditions := migrationobj.Status.Conditions

	files, err := os.ReadDir(VDDKDirectory)
	if err != nil {
		logger.Error(err, "VDDK directory could not be read")

		migrationobj.Status.Phase = vjailbreakv1alpha1.VMMigrationPhasePending
		setCondition := corev1.PodCondition{
			Type:               "VDDKCheck",
			Status:             corev1.ConditionFalse,
			Reason:             "VDDKDirectoryMissing",
			Message:            "VDDK directory is missing. Please create and upload the required files.",
			LastTransitionTime: metav1.Now(),
		}

		newConditions := []corev1.PodCondition{}
		for _, c := range migrationobj.Status.Conditions {
			if c.Type != "VDDKCheck" {
				newConditions = append(newConditions, c)
			}
		}
		newConditions = append(newConditions, setCondition)
		migrationobj.Status.Conditions = newConditions

		if err = r.Status().Update(ctx, migrationobj); err != nil {
			return errors.Wrap(err, "failed to update migration status after missing VDDK dir")
		}

		return errors.Wrapf(err, "VDDK_MISSING: directory could not be read")
	}

	if len(files) == 0 {
		logger.Info("VDDK directory is empty, skipping Job creation. Will retry in 30s.",
			"path", VDDKDirectory,
			"whoami", whoami)

		migrationobj.Status.Phase = vjailbreakv1alpha1.VMMigrationPhasePending
		migrationobj.Status.Conditions = append(migrationobj.Status.Conditions, corev1.PodCondition{
			Type:               "VDDKCheck",
			Status:             corev1.ConditionFalse,
			Reason:             "VDDKDirectoryEmpty",
			Message:            "VDDK directory is empty. Please upload the required files.",
			LastTransitionTime: metav1.Now(),
		})

		if !reflect.DeepEqual(migrationobj.Status.Conditions, oldConditions) {
			if err = r.Status().Update(ctx, migrationobj); err != nil {
				return errors.Wrap(err, "failed to update migration status after empty VDDK dir")
			}
		}

		return errors.Wrapf(errors.New("VDDK_MISSING"), "vddk directory is empty")
	}

	// Clear previous VDDKCheck condition if directory is valid
	cleanedConditions := []corev1.PodCondition{}
	for _, c := range migrationobj.Status.Conditions {
		if c.Type != "VDDKCheck" {
			cleanedConditions = append(cleanedConditions, c)
		}
	}

	migrationobj.Status.Phase = vjailbreakv1alpha1.VMMigrationPhasePending
	migrationobj.Status.Conditions = cleanedConditions

	if !reflect.DeepEqual(migrationobj.Status.Conditions, oldConditions) {
		if err = r.Status().Update(ctx, migrationobj); err != nil {
			return errors.Wrap(err, "failed to update migration status after validating VDDK presence")
		}
	}
	return nil
}

// MergeLabels combines two label maps into a single map, with values from b overriding values from a if keys conflict.
// This function is used to create a complete set of labels for Kubernetes resources created during migration.
func MergeLabels(a, b map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		result[k] = v
	}
	return result
}

// EnsureVMFolderExists ensures that the specified folder exists in the datacenter.
// If the folder does not exist, it creates a new folder with the specified name.
func EnsureVMFolderExists(ctx context.Context, finder *find.Finder, dc *object.Datacenter, folderName string) (*object.Folder, error) {
	finder.SetDatacenter(dc)

	// Check if folder exists
	folder, err := finder.Folder(ctx, folderName)
	if err == nil {
		return folder, nil
	}

	// Create folder if missing
	folders, err := dc.Folders(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get datacenter folders: %w", err)
	}
	folder, err = folders.VmFolder.CreateFolder(ctx, folderName)
	if err != nil {
		return nil, fmt.Errorf("failed to create folder '%s': %w", folderName, err)
	}
	return folder, nil
}
