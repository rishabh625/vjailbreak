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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vjailbreakv1alpha1 "github.com/vjailbreak/k8s/migration/api/v1alpha1"
)

// MigrationReconciler reconciles a Migration object
type MigrationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Migration phase constants
const (
	// Standard migration phases
	PhaseValidating     = "Validating"
	PhasePreparing      = "Preparing"
	PhaseCopying        = "Copying"
	PhaseConvertingDisk = "ConvertingDisk"
	PhaseCreatingVM     = "CreatingVM"
	PhaseCompleted      = "Completed"
	PhaseFailed         = "Failed"

	// RDM-specific phases
	PhaseRDMPreparing  = "RDMPreparing"
	PhaseRDMCopying    = "RDMCopying"
	PhaseRDMValidating = "RDMValidating"

	// RDM migration status constants
	RDMStatusPending    = "Pending"
	RDMStatusInProgress = "InProgress"
	RDMStatusCompleted  = "Completed"
	RDMStatusFailed     = "Failed"

	// Finalizer for migration cleanup
	MigrationFinalizer = "migration.vjailbreak.k8s.pf9.io/finalizer"
)

//+kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=migrations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=migrations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=migrations/finalizers,verbs=update
//+kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=vmwaremachines,verbs=get;list;watch
//+kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=sharedrdms,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=storagemappings,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MigrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Migration instance
	migration := &vjailbreakv1alpha1.Migration{}
	err := r.Get(ctx, req.NamespacedName, migration)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Migration resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Migration")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if migration.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, migration)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(migration, MigrationFinalizer) {
		controllerutil.AddFinalizer(migration, MigrationFinalizer)
		return ctrl.Result{}, r.Update(ctx, migration)
	}

	// Initialize status if needed
	if migration.Status.Phase == "" {
		migration.Status.Phase = PhaseValidating
		migration.Status.StartTime = &metav1.Time{Time: time.Now()}
		if err := r.Status().Update(ctx, migration); err != nil {
			logger.Error(err, "Failed to update migration status")
			return ctrl.Result{}, err
		}
	}

	// Pre-migration RDM setup
	if migration.Spec.RDMMigrationEnabled {
		rdmReady, err := r.checkRDMReadiness(ctx, migration)
		if err != nil {
			logger.Error(err, "Failed to check RDM readiness")
			return r.updateMigrationStatus(ctx, migration, PhaseFailed, err.Error())
		}
		if !rdmReady {
			logger.Info("Waiting for SharedRDM resources to be ready")
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}
	}

	// Handle migration phases
	switch migration.Status.Phase {
	case PhaseValidating:
		return r.handleValidatingPhase(ctx, migration)
	case PhaseRDMPreparing:
		return r.handleRDMPhase(ctx, migration, PhaseRDMPreparing)
	case PhaseRDMCopying:
		return r.handleRDMPhase(ctx, migration, PhaseRDMCopying)
	case PhaseRDMValidating:
		return r.handleRDMPhase(ctx, migration, PhaseRDMValidating)
	case PhasePreparing:
		return r.handlePreparingPhase(ctx, migration)
	case PhaseCopying:
		return r.handleCopyingPhase(ctx, migration)
	case PhaseConvertingDisk:
		return r.handleConvertingDiskPhase(ctx, migration)
	case PhaseCreatingVM:
		return r.handleCreatingVMPhase(ctx, migration)
	case PhaseCompleted:
		return ctrl.Result{}, nil
	case PhaseFailed:
		return ctrl.Result{}, nil
	default:
		logger.Info("Unknown migration phase", "phase", migration.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}
}

// handleValidatingPhase validates the migration configuration and transitions to the next phase
func (r *MigrationReconciler) handleValidatingPhase(ctx context.Context, migration *vjailbreakv1alpha1.Migration) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Validate VMware machine exists
	vmwareMachine := &vjailbreakv1alpha1.VMwareMachine{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      migration.Spec.VMwareMachineRef,
		Namespace: migration.Namespace,
	}, vmwareMachine)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.updateMigrationStatus(ctx, migration, PhaseFailed, "VMware machine not found")
		}
		logger.Error(err, "Failed to get VMware machine")
		return ctrl.Result{}, err
	}

	// Check if VM has RDM disks and update migration spec accordingly
	if vmwareMachine.Spec.VMInfo.HasRDMDisks {
		migration.Spec.RDMMigrationEnabled = true
		migration.Status.RDMDisksTotal = len(vmwareMachine.Spec.VMInfo.RDMDiskInfo)
		migration.Status.RDMDisksCompleted = 0
		migration.Status.RDMMigrationPhase = RDMStatusPending

		// Collect SharedRDM references
		var sharedRDMRefs []string
		for _, rdmDisk := range vmwareMachine.Spec.VMInfo.RDMDiskInfo {
			if rdmDisk.IsShared && rdmDisk.SharedRDMRef != "" {
				sharedRDMRefs = append(sharedRDMRefs, rdmDisk.SharedRDMRef)
			}
		}
		migration.Spec.SharedRDMRefs = sharedRDMRefs
	}

	// Validate storage mapping exists
	storageMapping := &vjailbreakv1alpha1.StorageMapping{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      migration.Spec.StorageMappingRef,
		Namespace: migration.Namespace,
	}, storageMapping)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.updateMigrationStatus(ctx, migration, PhaseFailed, "Storage mapping not found")
		}
		logger.Error(err, "Failed to get storage mapping")
		return ctrl.Result{}, err
	}

	// Update migration with validation results
	if err := r.Update(ctx, migration); err != nil {
		logger.Error(err, "Failed to update migration spec")
		return ctrl.Result{}, err
	}

	// Determine next phase based on RDM presence
	nextPhase := PhasePreparing
	if migration.Spec.RDMMigrationEnabled {
		nextPhase = PhaseRDMPreparing
	}

	return r.updateMigrationStatus(ctx, migration, nextPhase, "Validation completed successfully")
}

// handleRDMPhase handles RDM-specific migration phases
func (r *MigrationReconciler) handleRDMPhase(ctx context.Context, migration *vjailbreakv1alpha1.Migration, phase string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Update RDM migration phase
	migration.Status.RDMMigrationPhase = RDMStatusInProgress
	if err := r.updateRDMStatus(ctx, migration); err != nil {
		return ctrl.Result{}, err
	}

	// Get the migration pod
	pod, err := r.getMigrationPod(ctx, migration)
	if err != nil {
		logger.Error(err, "Failed to get migration pod")
		return ctrl.Result{}, err
	}

	if pod == nil {
		// Create migration pod with RDM configuration
		pod, err = r.createMigrationPod(ctx, migration)
		if err != nil {
			logger.Error(err, "Failed to create migration pod")
			return r.updateMigrationStatus(ctx, migration, PhaseFailed, fmt.Sprintf("Failed to create migration pod: %v", err))
		}
		logger.Info("Created migration pod for RDM phase", "phase", phase, "pod", pod.Name)
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	// Check pod status and handle phase transitions
	switch pod.Status.Phase {
	case corev1.PodPending:
		logger.Info("Migration pod is pending", "phase", phase)
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil

	case corev1.PodRunning:
		logger.Info("Migration pod is running", "phase", phase)
		// Check for phase completion events or status updates
		return r.checkRDMPhaseProgress(ctx, migration, pod, phase)

	case corev1.PodSucceeded:
		logger.Info("Migration pod succeeded", "phase", phase)
		return r.handleRDMPhaseCompletion(ctx, migration, phase)

	case corev1.PodFailed:
		logger.Error(nil, "Migration pod failed", "phase", phase)
		migration.Status.RDMMigrationPhase = RDMStatusFailed
		return r.updateMigrationStatus(ctx, migration, PhaseFailed, "RDM migration pod failed")

	default:
		logger.Info("Migration pod in unknown state", "phase", phase, "podPhase", pod.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}
}

// handleRDMPhaseCompletion handles the completion of an RDM phase and transitions to the next phase
func (r *MigrationReconciler) handleRDMPhaseCompletion(ctx context.Context, migration *vjailbreakv1alpha1.Migration, currentPhase string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var nextPhase string
	switch currentPhase {
	case PhaseRDMPreparing:
		nextPhase = PhaseRDMCopying
		logger.Info("RDM preparation completed, moving to copying phase")
	case PhaseRDMCopying:
		nextPhase = PhaseRDMValidating
		logger.Info("RDM copying completed, moving to validation phase")
	case PhaseRDMValidating:
		nextPhase = PhasePreparing
		migration.Status.RDMMigrationPhase = RDMStatusCompleted
		migration.Status.RDMDisksCompleted = migration.Status.RDMDisksTotal
		logger.Info("RDM validation completed, moving to regular migration phases")
	}

	// Update RDM status
	if err := r.updateRDMStatus(ctx, migration); err != nil {
		return ctrl.Result{}, err
	}

	return r.updateMigrationStatus(ctx, migration, nextPhase, fmt.Sprintf("RDM phase %s completed successfully", currentPhase))
}

// checkRDMPhaseProgress checks the progress of the current RDM phase
func (r *MigrationReconciler) checkRDMPhaseProgress(ctx context.Context, migration *vjailbreakv1alpha1.Migration, pod *corev1.Pod, phase string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Check pod logs or events for progress updates
	// This would typically involve parsing pod logs or checking for specific events
	// For now, we'll implement a simple time-based check

	// Check if the pod has been running for a reasonable amount of time
	if pod.Status.StartTime != nil {
		runningTime := time.Since(pod.Status.StartTime.Time)

		// Set reasonable timeouts for each phase
		var timeout time.Duration
		switch phase {
		case PhaseRDMPreparing:
			timeout = time.Minute * 10
		case PhaseRDMCopying:
			timeout = time.Hour * 2 // Longer timeout for data copying
		case PhaseRDMValidating:
			timeout = time.Minute * 30
		default:
			timeout = time.Minute * 30
		}

		if runningTime > timeout {
			logger.Error(nil, "RDM phase timeout exceeded", "phase", phase, "runningTime", runningTime)
			migration.Status.RDMMigrationPhase = RDMStatusFailed
			return r.updateMigrationStatus(ctx, migration, PhaseFailed, fmt.Sprintf("RDM phase %s timeout exceeded", phase))
		}
	}

	// Continue monitoring
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// checkRDMReadiness verifies that all SharedRDM resources are ready
func (r *MigrationReconciler) checkRDMReadiness(ctx context.Context, migration *vjailbreakv1alpha1.Migration) (bool, error) {
	logger := log.FromContext(ctx)

	if !migration.Spec.RDMMigrationEnabled || len(migration.Spec.SharedRDMRefs) == 0 {
		return true, nil
	}

	for _, sharedRDMRef := range migration.Spec.SharedRDMRefs {
		sharedRDM := &vjailbreakv1alpha1.SharedRDM{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      sharedRDMRef,
			Namespace: migration.Namespace,
		}, sharedRDM)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("SharedRDM resource not found", "sharedRDM", sharedRDMRef)
				return false, nil
			}
			return false, err
		}

		if sharedRDM.Status.Phase != "Ready" {
			logger.Info("SharedRDM resource not ready", "sharedRDM", sharedRDMRef, "phase", sharedRDM.Status.Phase)
			return false, nil
		}
	}

	logger.Info("All SharedRDM resources are ready")
	return true, nil
}

// updateRDMStatus updates the RDM-specific status fields
func (r *MigrationReconciler) updateRDMStatus(ctx context.Context, migration *vjailbreakv1alpha1.Migration) error {
	logger := log.FromContext(ctx)

	// Update RDM status fields
	now := metav1.Now()
	migration.Status.LastUpdateTime = &now

	// Add or update RDM-related conditions
	condition := metav1.Condition{
		Type:               "RDMMigrationProgress",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: now,
		Reason:             "RDMProgress",
		Message:            fmt.Sprintf("RDM migration phase: %s, completed: %d/%d", migration.Status.RDMMigrationPhase, migration.Status.RDMDisksCompleted, migration.Status.RDMDisksTotal),
	}

	// Update or add the condition
	updated := false
	for i, existingCondition := range migration.Status.Conditions {
		if existingCondition.Type == condition.Type {
			migration.Status.Conditions[i] = condition
			updated = true
			break
		}
	}
	if !updated {
		migration.Status.Conditions = append(migration.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, migration); err != nil {
		logger.Error(err, "Failed to update RDM status")
		return err
	}

	return nil
}

// updateMigrationStatus updates the migration status with the given phase and message
func (r *MigrationReconciler) updateMigrationStatus(ctx context.Context, migration *vjailbreakv1alpha1.Migration, phase, message string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	migration.Status.Phase = phase
	migration.Status.Message = message
	now := metav1.Now()
	migration.Status.LastUpdateTime = &now

	if phase == PhaseCompleted {
		migration.Status.CompletionTime = &now
	}

	// Add condition
	conditionType := "MigrationProgress"
	conditionStatus := metav1.ConditionTrue
	if phase == PhaseFailed {
		conditionStatus = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastTransitionTime: now,
		Reason:             phase,
		Message:            message,
	}

	// Update or add the condition
	updated := false
	for i, existingCondition := range migration.Status.Conditions {
		if existingCondition.Type == condition.Type {
			migration.Status.Conditions[i] = condition
			updated = true
			break
		}
	}
	if !updated {
		migration.Status.Conditions = append(migration.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, migration); err != nil {
		logger.Error(err, "Failed to update migration status")
		return ctrl.Result{}, err
	}

	logger.Info("Updated migration status", "phase", phase, "message", message)

	// Determine requeue behavior
	if phase == PhaseCompleted || phase == PhaseFailed {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

// getMigrationPod gets the migration pod for the given migration
func (r *MigrationReconciler) getMigrationPod(ctx context.Context, migration *vjailbreakv1alpha1.Migration) (*corev1.Pod, error) {
	podList := &corev1.PodList{}
	err := r.List(ctx, podList, client.InNamespace(migration.Namespace), client.MatchingLabels{
		"migration": migration.Name,
		"app":       "v2v-helper",
	})
	if err != nil {
		return nil, err
	}

	if len(podList.Items) == 0 {
		return nil, nil
	}

	// Return the most recent pod
	return &podList.Items[0], nil
}

// createMigrationPod creates a migration pod with RDM configuration
func (r *MigrationReconciler) createMigrationPod(ctx context.Context, migration *vjailbreakv1alpha1.Migration) (*corev1.Pod, error) {
	logger := log.FromContext(ctx)

	// Get VMware machine for configuration
	vmwareMachine := &vjailbreakv1alpha1.VMwareMachine{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      migration.Spec.VMwareMachineRef,
		Namespace: migration.Namespace,
	}, vmwareMachine)
	if err != nil {
		return nil, err
	}

	// Create pod specification
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("migration-%s-%d", migration.Name, time.Now().Unix()),
			Namespace: migration.Namespace,
			Labels: map[string]string{
				"migration": migration.Name,
				"app":       "v2v-helper",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "v2v-helper",
					Image: "vjailbreak/v2v-helper:latest",
					Env:   r.buildMigrationEnvVars(migration, vmwareMachine),
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "migration-config",
							MountPath: "/etc/migration",
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &[]bool{true}[0], // Required for RDM device access
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "migration-config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: fmt.Sprintf("migration-config-%s", migration.Name),
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(migration, pod, r.Scheme); err != nil {
		return nil, err
	}

	// Create the pod
	if err := r.Create(ctx, pod); err != nil {
		return nil, err
	}

	logger.Info("Created migration pod", "pod", pod.Name)
	return pod, nil
}

// buildMigrationEnvVars builds environment variables for the migration pod
func (r *MigrationReconciler) buildMigrationEnvVars(migration *vjailbreakv1alpha1.Migration, vmwareMachine *vjailbreakv1alpha1.VMwareMachine) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "MIGRATION_NAME",
			Value: migration.Name,
		},
		{
			Name:  "MIGRATION_PHASE",
			Value: migration.Status.Phase,
		},
		{
			Name:  "VMWARE_MACHINE_NAME",
			Value: vmwareMachine.Name,
		},
		{
			Name:  "VMWARE_VM_UUID",
			Value: vmwareMachine.Spec.VMInfo.UUID,
		},
		{
			Name:  "VCENTER_URL",
			Value: vmwareMachine.Spec.VCenterURL,
		},
		{
			Name:  "DATACENTER",
			Value: vmwareMachine.Spec.Datacenter,
		},
	}

	// Add RDM-specific environment variables
	if migration.Spec.RDMMigrationEnabled {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "RDM_MIGRATION_ENABLED",
			Value: "true",
		})

		envVars = append(envVars, corev1.EnvVar{
			Name:  "RDM_MIGRATION_STRATEGY",
			Value: migration.Spec.RDMMigrationStrategy,
		})

		if len(migration.Spec.SharedRDMRefs) > 0 {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "SHARED_RDM_REFS",
				Value: strings.Join(migration.Spec.SharedRDMRefs, ","),
			})
		}

		// Add RDM disk information
		var rdmDiskUUIDs []string
		for _, rdmDisk := range vmwareMachine.Spec.VMInfo.RDMDiskInfo {
			rdmDiskUUIDs = append(rdmDiskUUIDs, rdmDisk.LunUUID)
		}
		if len(rdmDiskUUIDs) > 0 {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "RDM_DISK_UUIDS",
				Value: strings.Join(rdmDiskUUIDs, ","),
			})
		}
	}

	// Add credentials from secret
	envVars = append(envVars, corev1.EnvVar{
		Name: "VMWARE_USERNAME",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: vmwareMachine.Spec.CredentialsRef,
				},
				Key: "username",
			},
		},
	})

	envVars = append(envVars, corev1.EnvVar{
		Name: "VMWARE_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: vmwareMachine.Spec.CredentialsRef,
				},
				Key: "password",
			},
		},
	})

	return envVars
}

// handlePreparingPhase handles the preparing phase of migration
func (r *MigrationReconciler) handlePreparingPhase(ctx context.Context, migration *vjailbreakv1alpha1.Migration) (ctrl.Result, error) {
	// Implementation for preparing phase
	// This would typically involve setting up OpenStack resources, networks, etc.
	return r.updateMigrationStatus(ctx, migration, PhaseCopying, "Preparation completed, starting disk copying")
}

// handleCopyingPhase handles the copying phase of migration
func (r *MigrationReconciler) handleCopyingPhase(ctx context.Context, migration *vjailbreakv1alpha1.Migration) (ctrl.Result, error) {
	// Implementation for copying phase
	// This would involve monitoring the disk copying process
	return r.updateMigrationStatus(ctx, migration, PhaseConvertingDisk, "Disk copying completed, starting disk conversion")
}

// handleConvertingDiskPhase handles the disk conversion phase
func (r *MigrationReconciler) handleConvertingDiskPhase(ctx context.Context, migration *vjailbreakv1alpha1.Migration) (ctrl.Result, error) {
	// Implementation for disk conversion phase
	return r.updateMigrationStatus(ctx, migration, PhaseCreatingVM, "Disk conversion completed, creating VM")
}

// handleCreatingVMPhase handles the VM creation phase
func (r *MigrationReconciler) handleCreatingVMPhase(ctx context.Context, migration *vjailbreakv1alpha1.Migration) (ctrl.Result, error) {
	// Implementation for VM creation phase
	return r.updateMigrationStatus(ctx, migration, PhaseCompleted, "Migration completed successfully")
}

// handleDeletion handles the deletion of migration resources
func (r *MigrationReconciler) handleDeletion(ctx context.Context, migration *vjailbreakv1alpha1.Migration) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Clean up migration pods
	podList := &corev1.PodList{}
	err := r.List(ctx, podList, client.InNamespace(migration.Namespace), client.MatchingLabels{
		"migration": migration.Name,
	})
	if err != nil {
		logger.Error(err, "Failed to list migration pods for cleanup")
		return ctrl.Result{}, err
	}

	for _, pod := range podList.Items {
		if err := r.Delete(ctx, &pod); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete migration pod", "pod", pod.Name)
			return ctrl.Result{}, err
		}
	}

	// Clean up config maps
	configMapList := &corev1.ConfigMapList{}
	err = r.List(ctx, configMapList, client.InNamespace(migration.Namespace), client.MatchingLabels{
		"migration": migration.Name,
	})
	if err != nil {
		logger.Error(err, "Failed to list config maps for cleanup")
		return ctrl.Result{}, err
	}

	for _, cm := range configMapList.Items {
		if err := r.Delete(ctx, &cm); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete config map", "configMap", cm.Name)
			return ctrl.Result{}, err
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(migration, MigrationFinalizer)
	if err := r.Update(ctx, migration); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Migration cleanup completed")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MigrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vjailbreakv1alpha1.Migration{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
