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
	"os"
	"os/user"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/pkg/errors"
	constants "github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/scope"
	utils "github.com/platform9/vjailbreak/k8s/migration/pkg/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
)

const VDDKDirectory = "/home/ubuntu/vmware-vix-disklib-distrib"

// MigrationPlanReconciler reconciles a MigrationPlan object
type MigrationPlanReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	ctxlog logr.Logger
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

	// Always close the scope when exiting this function such that we can persist any MigrationPlan changes.
	defer func() {
		if err := migrationPlanScope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// examine DeletionTimestamp to determine if object is under deletion or not
	if !migrationplan.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, migrationPlanScope)
	}

	return r.reconcileNormal(ctx, migrationPlanScope)
}

func (r *MigrationPlanReconciler) reconcileNormal(ctx context.Context, scope *scope.MigrationPlanScope) (ctrl.Result, error) {
	migrationplan := scope.MigrationPlan
	scope.Logger.Info(fmt.Sprintf("Reconciling MigrationPlan '%s'", migrationplan.Name))

	controllerutil.AddFinalizer(migrationplan, migrationPlanFinalizer)

	if res, err := r.ReconcileMigrationPlanJob(ctx, migrationplan); err != nil {
		return res, err
	}
	return ctrl.Result{}, nil
}

//nolint:unparam //future use
func (r *MigrationPlanReconciler) reconcileDelete(
	ctx context.Context,
	scope *scope.MigrationPlanScope) (ctrl.Result, error) {
	migrationplan := scope.MigrationPlan
	log := scope.Logger

	// The object is being deleted
	log.Info(fmt.Sprintf("MigrationPlan '%s' CR is being deleted", migrationplan.Name))

	// Now that the finalizer has completed deletion tasks, we can remove it
	// to allow deletion of the Migration object
	controllerutil.RemoveFinalizer(migrationplan, migrationPlanFinalizer)
	if err := r.Update(ctx, migrationplan); err != nil {
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, nil
}

// Similar to the Reconcile function above, but specifically for reconciling the Jobs
func (r *MigrationPlanReconciler) ReconcileMigrationPlanJob(ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan) (ctrl.Result, error) {
	// Fetch MigrationTemplate CR
	migrationtemplate := &vjailbreakv1alpha1.MigrationTemplate{}
	if err := r.Get(ctx, types.NamespacedName{Name: migrationplan.Spec.MigrationTemplate, Namespace: migrationplan.Namespace},
		migrationtemplate); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get MigrationTemplate: %w", err)
	}
	// Fetch VMwareCreds CR
	vmwcreds := &vjailbreakv1alpha1.VMwareCreds{}
	if ok, err := r.checkStatusSuccess(ctx, migrationtemplate.Namespace, migrationtemplate.Spec.Source.VMwareRef, true, vmwcreds); !ok {
		return ctrl.Result{
			RequeueAfter: time.Minute,
		}, err
	}
	// Fetch OpenStackCreds CR
	openstackcreds := &vjailbreakv1alpha1.OpenstackCreds{}
	if ok, err := r.checkStatusSuccess(ctx, migrationtemplate.Namespace, migrationtemplate.Spec.Destination.OpenstackRef,
		false, openstackcreds); !ok {
		return ctrl.Result{
			RequeueAfter: time.Minute,
		}, err
	}
	// Starting the Migrations
	if migrationplan.Status.MigrationStatus == "" {
		err := r.UpdateMigrationPlanStatus(ctx, migrationplan, string(corev1.PodRunning), "Migration(s) in progress")
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update MigrationPlan status: %w", err)
		}
	}

	for _, parallelvms := range migrationplan.Spec.VirtualMachines {
		migrationobjs := &vjailbreakv1alpha1.MigrationList{}
		err := r.TriggerMigration(ctx, migrationplan, migrationobjs, openstackcreds, vmwcreds, migrationtemplate, parallelvms)
		if err != nil {
			if strings.Contains(err.Error(), "VDDK_MISSING") {
				r.ctxlog.Info("Requeuing due to missing VDDK files.")
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			return ctrl.Result{}, err
		}
		for i := 0; i < len(migrationobjs.Items); i++ {
			switch migrationobjs.Items[i].Status.Phase {
			case vjailbreakv1alpha1.MigrationPhaseFailed:
				r.ctxlog.Info(fmt.Sprintf("Migration for VM '%s' failed", migrationobjs.Items[i].Spec.VMName))
				if migrationplan.Spec.Retry {
					r.ctxlog.Info(fmt.Sprintf("Retrying migration for VM '%s'", migrationobjs.Items[i].Spec.VMName))
					// Delete the migration so that it can be recreated
					err := r.Delete(ctx, &migrationobjs.Items[i])
					if err != nil {
						return ctrl.Result{}, fmt.Errorf("failed to delete Migration: %w", err)
					}
					migrationplan.Status.MigrationStatus = "Retrying"
					migrationplan.Status.MigrationMessage = fmt.Sprintf("Retrying migration for VM '%s'", migrationobjs.Items[i].Spec.VMName)
					migrationplan.Spec.Retry = false
					err = r.Update(ctx, migrationplan)
					if err != nil {
						return ctrl.Result{}, fmt.Errorf("failed to update Migration status: %w", err)
					}
					return ctrl.Result{}, nil
				}
				err := r.UpdateMigrationPlanStatus(ctx, migrationplan, string(corev1.PodFailed),
					fmt.Sprintf("Migration for VM '%s' failed", migrationobjs.Items[i].Spec.VMName))
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to update MigrationPlan status: %w", err)
				}
				return ctrl.Result{}, nil
			case vjailbreakv1alpha1.MigrationPhaseSucceeded:
				continue
			default:
				r.ctxlog.Info(fmt.Sprintf("Waiting for all VMs in parallel batch %d to complete: %v", i+1, parallelvms))
				return ctrl.Result{}, nil
			}
		}
	}
	r.ctxlog.Info(fmt.Sprintf("All VMs in MigrationPlan '%s' have been successfully migrated", migrationplan.Name))
	migrationplan.Status.MigrationStatus = string(corev1.PodSucceeded)
	err := r.Status().Update(ctx, migrationplan)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update MigrationPlan status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *MigrationPlanReconciler) UpdateMigrationPlanStatus(ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan, status, message string) error {
	migrationplan.Status.MigrationStatus = status
	migrationplan.Status.MigrationMessage = message
	err := r.Status().Update(ctx, migrationplan)
	if err != nil {
		return fmt.Errorf("failed to update MigrationPlan status: %w", err)
	}
	return nil
}

func (r *MigrationPlanReconciler) CreateMigration(ctx context.Context,
	migrationplan *vjailbreakv1alpha1.MigrationPlan,
	vm string, vmMachine *vjailbreakv1alpha1.VMwareMachine) (*vjailbreakv1alpha1.Migration, error) {
	ctxlog := r.ctxlog.WithValues("vm", vm)
	ctxlog.Info("Creating Migration for VM")

	vmname, err := utils.ConvertToK8sName(vm)
	if err != nil {
		return nil, fmt.Errorf("failed to convert VM name: %w", err)
	}
	vminfo := &vmMachine.Spec.VMs

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
		err = r.createResource(ctx, migrationplan, migrationobj)
		if err != nil {
			return nil, fmt.Errorf("failed to create Migration for VM %s: %w", vm, err)
		}
	}
	return migrationobj, nil
}

// Create Job to run v2v-helper
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
								ImagePullPolicy: corev1.PullAlways,
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
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:              resource.MustParse("1000m"),
										corev1.ResourceMemory:           resource.MustParse("1Gi"),
										corev1.ResourceEphemeralStorage: resource.MustParse("200Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:              resource.MustParse("2000m"),
										corev1.ResourceMemory:           resource.MustParse("3Gi"),
										corev1.ResourceEphemeralStorage: resource.MustParse("2Gi"),
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
			if err = utils.VerifyNetworks(ctx, openstackcreds, migrationplan.Spec.AdvancedOptions.GranularNetworks); err != nil {
				return nil, fmt.Errorf("failed to verify networks in advanced mapping: %w", err)
			}
			openstacknws = migrationplan.Spec.AdvancedOptions.GranularNetworks
		}
		if len(migrationplan.Spec.AdvancedOptions.GranularVolumeTypes) > 0 {
			if err = utils.VerifyStorage(ctx, openstackcreds, migrationplan.Spec.AdvancedOptions.GranularVolumeTypes); err != nil {
				return nil, fmt.Errorf("failed to verify volume types in advanced mapping: %w", err)
			}
			openstackvolumetypes = migrationplan.Spec.AdvancedOptions.GranularVolumeTypes
		}
		if len(migrationplan.Spec.AdvancedOptions.GranularPorts) > 0 {
			if err = utils.VerifyPorts(ctx, openstackcreds, migrationplan.Spec.AdvancedOptions.GranularPorts); err != nil {
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
				"OS_TYPE":               migrationtemplate.Spec.OSType,
				"VIRTIO_WIN_DRIVER":     virtiodrivers,
				"PERFORM_HEALTH_CHECKS": strconv.FormatBool(migrationplan.Spec.MigrationStrategy.PerformHealthChecks),
				"HEALTH_CHECK_PORT":     migrationplan.Spec.MigrationStrategy.HealthCheckPort,
			},
		}
		// Check if target flavor is set
		if vmMachine.Spec.TargetFlavorID != "" {
			configMap.Data["TARGET_FLAVOR_ID"] = vmMachine.Spec.TargetFlavorID
		} else {
			var computeClient *utils.OpenStackClients
			// If target flavor is not set, use the closest matching flavor
			computeClient, err = utils.GetOpenStackClients(ctx, openstackcreds)
			if err != nil {
				return nil, fmt.Errorf("failed to get OpenStack clients: %w", err)
			}
			var flavor *flavors.Flavor
			flavor, err = utils.GetClosestFlavour(vmMachine.Spec.VMs.CPU, vmMachine.Spec.VMs.Memory, computeClient.ComputeClient)
			if err != nil {
				return nil, fmt.Errorf("failed to get closest flavor: %w", err)
			}
			if flavor == nil {
				return nil, fmt.Errorf("no suitable flavor found for %d vCPUs and %d MB RAM", vmMachine.Spec.VMs.CPU, vmMachine.Spec.VMs.Memory)
			}
			configMap.Data["TARGET_FLAVOR_ID"] = flavor.ID
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
	err := r.Get(ctx, types.NamespacedName{Name: credsname, Namespace: namespace}, credsobj)
	if err != nil {
		return false, fmt.Errorf("failed to get Creds: %w", err)
	}

	if isvmware && credsobj.(*vjailbreakv1alpha1.VMwareCreds).Status.VMwareValidationStatus != string(corev1.PodSucceeded) {
		return false, fmt.Errorf("VMwareCreds '%s' CR is not validated", credsobj.(*vjailbreakv1alpha1.VMwareCreds).Name)
	} else if !isvmware && credsobj.(*vjailbreakv1alpha1.OpenstackCreds).Status.OpenStackValidationStatus != string(corev1.PodSucceeded) {
		return false, fmt.Errorf("OpenstackCreds '%s' CR is not validated", credsobj.(*vjailbreakv1alpha1.OpenstackCreds).Name)
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
	vmnws, err := utils.GetVMwNetworks(ctx, vmwcreds, vmwcreds.Spec.DataCenter, vm)
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
		return nil, fmt.Errorf("VMware Network(s) not found in NetworkMapping")
	}

	if networkmap.Status.NetworkmappingValidationStatus != string(corev1.PodSucceeded) {
		err = utils.VerifyNetworks(ctx, openstackcreds, openstacknws)
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
	vmds, err := utils.GetVMwDatastore(ctx, vmwcreds, vmwcreds.Spec.DataCenter, vm)
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
		return nil, fmt.Errorf("VMware Datastore(s) not found in StorageMapping")
	}
	if storagemap.Status.StoragemappingValidationStatus != string(corev1.PodSucceeded) {
		err = utils.VerifyStorage(ctx, openstackcreds, openstackvolumetypes)
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
	err := r.Client.List(ctx, nodeList, &client.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list nodes")
	}
	counter := len(nodeList.Items)

	vmMachines := &vjailbreakv1alpha1.VMwareMachineList{}

	err = r.Client.List(ctx, vmMachines, &client.ListOptions{Namespace: migrationtemplate.Namespace})
	if err != nil {
		return errors.Wrap(err, "failed to list vmwaremachines")
	}

	var vmMachineObj *vjailbreakv1alpha1.VMwareMachine
	for _, vm := range parallelvms {
		vmMachineObj = nil
		for i := range vmMachines.Items {
			if vmMachines.Items[i].Spec.VMs.Name == vm {
				vmMachineObj = &vmMachines.Items[i]
				break
			}
		}
		if vmMachineObj == nil {
			return errors.Wrap(fmt.Errorf("VM '%s' not found in VMwareMachine", vm), "failed to find vmwaremachine")
		}
		migrationobj, err := r.CreateMigration(ctx, migrationplan, vm, vmMachineObj)
		if err != nil {
			if apierrors.IsAlreadyExists(err) && migrationobj.Status.Phase == vjailbreakv1alpha1.MigrationPhaseSucceeded {
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

// SetupWithManager sets up the controller with the Manager.
func (r *MigrationPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vjailbreakv1alpha1.MigrationPlan{}).
		Owns(&vjailbreakv1alpha1.Migration{}).
		Complete(r)
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

	files, err := os.ReadDir(VDDKDirectory)
	if err != nil {
		logger.Error(err, "VDDK directory could not be read")

		migrationobj.Status.Phase = vjailbreakv1alpha1.MigrationPhasePending
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

		migrationobj.Status.Phase = vjailbreakv1alpha1.MigrationPhasePending
		migrationobj.Status.Conditions = append(migrationobj.Status.Conditions, corev1.PodCondition{
			Type:               "VDDKCheck",
			Status:             corev1.ConditionFalse,
			Reason:             "VDDKDirectoryEmpty",
			Message:            "VDDK directory is empty. Please upload the required files.",
			LastTransitionTime: metav1.Now(),
		})

		if err = r.Status().Update(ctx, migrationobj); err != nil {
			return errors.Wrap(err, "failed to update migration status after empty VDDK dir")
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

	migrationobj.Status.Conditions = cleanedConditions

	if err = r.Status().Update(ctx, migrationobj); err != nil {
		return errors.Wrap(err, "failed to update migration status after validating VDDK presence")
	}
	return nil
}
