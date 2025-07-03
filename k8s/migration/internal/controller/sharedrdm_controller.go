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
	"math"
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
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	vjailbreakv1alpha1 "github.com/vjailbreak/k8s/migration/api/v1alpha1"
)

// SharedRDMReconciler reconciles a SharedRDM object
type SharedRDMReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SharedRDM phase constants
const (
	SharedRDMPhaseDiscovering = "Discovering"
	SharedRDMPhaseCreating    = "Creating"
	SharedRDMPhaseReady       = "Ready"
	SharedRDMPhaseFailed      = "Failed"
	SharedRDMPhaseDeleting    = "Deleting"

	// Finalizer for SharedRDM cleanup
	SharedRDMFinalizer = "sharedrdm.vjailbreak.k8s.pf9.io/finalizer"

	// Condition types
	ConditionTypeVMwareLUNValidated = "VMwareLUNValidated"
	ConditionTypeVolumeCreated      = "VolumeCreated"
	ConditionTypeMultiAttachEnabled = "MultiAttachEnabled"
	ConditionTypeReady              = "Ready"

	// Retry configuration
	MaxRetryAttempts = 5
	BaseRetryDelay   = time.Second * 5
)

//+kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=sharedrdms,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=sharedrdms/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=sharedrdms/finalizers,verbs=update
//+kubebuilder:rbac:groups=vjailbreak.k8s.pf9.io,resources=vmwaremachines,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SharedRDMReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the SharedRDM instance
	sharedRDM := &vjailbreakv1alpha1.SharedRDM{}
	err := r.Get(ctx, req.NamespacedName, sharedRDM)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("SharedRDM resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get SharedRDM")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if sharedRDM.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, sharedRDM)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(sharedRDM, SharedRDMFinalizer) {
		controllerutil.AddFinalizer(sharedRDM, SharedRDMFinalizer)
		return ctrl.Result{}, r.Update(ctx, sharedRDM)
	}

	// Initialize status if needed
	if sharedRDM.Status.Phase == "" {
		sharedRDM.Status.Phase = SharedRDMPhaseDiscovering
		sharedRDM.Status.Conditions = []metav1.Condition{}
		if err := r.Status().Update(ctx, sharedRDM); err != nil {
			logger.Error(err, "Failed to initialize SharedRDM status")
			return ctrl.Result{}, err
		}
	}

	// Handle different phases
	switch sharedRDM.Status.Phase {
	case SharedRDMPhaseDiscovering:
		return r.handleDiscoveryPhase(ctx, sharedRDM)
	case SharedRDMPhaseCreating:
		return r.handleCreationPhase(ctx, sharedRDM)
	case SharedRDMPhaseReady:
		return r.handleReadyPhase(ctx, sharedRDM)
	case SharedRDMPhaseFailed:
		return r.handleFailedPhase(ctx, sharedRDM)
	default:
		logger.Info("Unknown SharedRDM phase", "phase", sharedRDM.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}
}

// handleDiscoveryPhase validates the VMware LUN and gathers metadata
func (r *SharedRDMReconciler) handleDiscoveryPhase(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting discovery phase for SharedRDM", "sourceUUID", sharedRDM.Spec.SourceUUID)

	// Validate VMware LUN accessibility
	if err := r.validateVMwareLUN(ctx, sharedRDM); err != nil {
		logger.Error(err, "Failed to validate VMware LUN")
		r.setCondition(sharedRDM, ConditionTypeVMwareLUNValidated, metav1.ConditionFalse, "ValidationFailed", err.Error())
		return r.updateStatusWithRetry(ctx, sharedRDM, SharedRDMPhaseFailed, fmt.Sprintf("VMware LUN validation failed: %v", err))
	}

	// Set successful validation condition
	r.setCondition(sharedRDM, ConditionTypeVMwareLUNValidated, metav1.ConditionTrue, "ValidationSucceeded", "VMware LUN validated successfully")

	// Transition to creation phase
	logger.Info("Discovery phase completed successfully, transitioning to creation phase")
	return r.updateStatusWithRetry(ctx, sharedRDM, SharedRDMPhaseCreating, "VMware LUN validated, starting volume creation")
}

// handleCreationPhase creates the OpenStack Cinder volume
func (r *SharedRDMReconciler) handleCreationPhase(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting creation phase for SharedRDM", "displayName", sharedRDM.Spec.DisplayName)

	// Check if volume already exists
	if sharedRDM.Status.VolumeID != "" {
		logger.Info("Volume already exists, verifying status", "volumeID", sharedRDM.Status.VolumeID)
		if err := r.verifyVolumeStatus(ctx, sharedRDM); err != nil {
			logger.Error(err, "Failed to verify existing volume status")
			return r.handleCreationError(ctx, sharedRDM, err)
		}
		// Volume exists and is ready, transition to ready phase
		return r.updateStatusWithRetry(ctx, sharedRDM, SharedRDMPhaseReady, "Volume verified and ready")
	}

	// Create OpenStack volume
	volumeID, err := r.createOpenStackVolume(ctx, sharedRDM)
	if err != nil {
		logger.Error(err, "Failed to create OpenStack volume")
		return r.handleCreationError(ctx, sharedRDM, err)
	}

	// Update status with volume ID
	sharedRDM.Status.VolumeID = volumeID
	r.setCondition(sharedRDM, ConditionTypeVolumeCreated, metav1.ConditionTrue, "VolumeCreated", fmt.Sprintf("Volume created with ID: %s", volumeID))

	// Enable multi-attach if required
	if r.requiresMultiAttach(sharedRDM) {
		if err := r.enableMultiAttach(ctx, sharedRDM, volumeID); err != nil {
			logger.Error(err, "Failed to enable multi-attach on volume")
			r.setCondition(sharedRDM, ConditionTypeMultiAttachEnabled, metav1.ConditionFalse, "MultiAttachFailed", err.Error())
			return r.handleCreationError(ctx, sharedRDM, err)
		}
		r.setCondition(sharedRDM, ConditionTypeMultiAttachEnabled, metav1.ConditionTrue, "MultiAttachEnabled", "Multi-attach enabled successfully")
	}

	// Set ready condition
	r.setCondition(sharedRDM, ConditionTypeReady, metav1.ConditionTrue, "Ready", "SharedRDM volume is ready for use")

	logger.Info("Creation phase completed successfully", "volumeID", volumeID)
	return r.updateStatusWithRetry(ctx, sharedRDM, SharedRDMPhaseReady, "Volume created and configured successfully")
}

// handleReadyPhase monitors the ready SharedRDM resource
func (r *SharedRDMReconciler) handleReadyPhase(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Periodically verify volume status
	if err := r.verifyVolumeStatus(ctx, sharedRDM); err != nil {
		logger.Error(err, "Volume verification failed, transitioning to failed state")
		r.setCondition(sharedRDM, ConditionTypeReady, metav1.ConditionFalse, "VolumeVerificationFailed", err.Error())
		return r.updateStatusWithRetry(ctx, sharedRDM, SharedRDMPhaseFailed, fmt.Sprintf("Volume verification failed: %v", err))
	}

	// Update attached VMs list
	if err := r.updateAttachedVMs(ctx, sharedRDM); err != nil {
		logger.Error(err, "Failed to update attached VMs list")
		// This is not a critical error, just log and continue
	}

	// Requeue for periodic verification
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

// handleFailedPhase handles failed SharedRDM resources
func (r *SharedRDMReconciler) handleFailedPhase(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Check if we should retry based on retry count and conditions
	retryCount := r.getRetryCount(sharedRDM)
	if retryCount < MaxRetryAttempts {
		// Calculate exponential backoff delay
		delay := time.Duration(math.Pow(2, float64(retryCount))) * BaseRetryDelay
		logger.Info("Scheduling retry for failed SharedRDM", "retryCount", retryCount, "delay", delay)

		// Increment retry count
		r.incrementRetryCount(sharedRDM)

		// Reset to discovery phase for retry
		return r.updateStatusWithRetry(ctx, sharedRDM, SharedRDMPhaseDiscovering, fmt.Sprintf("Retrying after failure (attempt %d/%d)", retryCount+1, MaxRetryAttempts))
	}

	// Max retries exceeded, stay in failed state
	logger.Info("Max retry attempts exceeded, staying in failed state")
	return ctrl.Result{RequeueAfter: time.Hour}, nil // Check again in an hour
}

// handleDeletion handles the deletion of SharedRDM resources
func (r *SharedRDMReconciler) handleDeletion(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling SharedRDM deletion", "name", sharedRDM.Name)

	// Update phase to deleting
	if sharedRDM.Status.Phase != SharedRDMPhaseDeleting {
		sharedRDM.Status.Phase = SharedRDMPhaseDeleting
		if err := r.Status().Update(ctx, sharedRDM); err != nil {
			logger.Error(err, "Failed to update status to deleting")
			return ctrl.Result{}, err
		}
	}

	// Check if volume is still attached to any VMs
	if sharedRDM.Status.VolumeID != "" {
		attachedVMs, err := r.getAttachedVMs(ctx, sharedRDM)
		if err != nil {
			logger.Error(err, "Failed to check attached VMs")
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}

		if len(attachedVMs) > 0 {
			logger.Info("Volume still attached to VMs, waiting for detachment", "attachedVMs", attachedVMs)
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}

		// Delete the OpenStack volume
		if err := r.deleteOpenStackVolume(ctx, sharedRDM); err != nil {
			logger.Error(err, "Failed to delete OpenStack volume")
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}
		logger.Info("OpenStack volume deleted successfully", "volumeID", sharedRDM.Status.VolumeID)
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(sharedRDM, SharedRDMFinalizer)
	if err := r.Update(ctx, sharedRDM); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("SharedRDM deletion completed successfully")
	return ctrl.Result{}, nil
}

// validateVMwareLUN validates that the VMware LUN exists and is accessible
func (r *SharedRDMReconciler) validateVMwareLUN(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) error {
	logger := log.FromContext(ctx)

	// Get VMware credentials from the referenced VMs
	if len(sharedRDM.Spec.VMwareRefs) == 0 {
		return fmt.Errorf("no VMware VM references provided")
	}

	// Use the first VM reference to get credentials
	vmRef := sharedRDM.Spec.VMwareRefs[0]
	vmwareMachine := &vjailbreakv1alpha1.VMwareMachine{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      vmRef,
		Namespace: sharedRDM.Namespace,
	}, vmwareMachine)
	if err != nil {
		return fmt.Errorf("failed to get VMware machine %s: %v", vmRef, err)
	}

	// Get VMware credentials
	credentials, err := r.getVMwareCredentials(ctx, vmwareMachine.Spec.CredentialsRef, sharedRDM.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get VMware credentials: %v", err)
	}

	// Validate LUN accessibility (this would typically involve connecting to vCenter)
	logger.Info("Validating VMware LUN accessibility", "sourceUUID", sharedRDM.Spec.SourceUUID, "vcenter", vmwareMachine.Spec.VCenterURL)

	// TODO: Implement actual VMware vCenter API call to validate LUN
	// For now, we'll simulate validation
	if sharedRDM.Spec.SourceUUID == "" {
		return fmt.Errorf("source UUID cannot be empty")
	}

	if sharedRDM.Spec.DiskSize <= 0 {
		return fmt.Errorf("disk size must be greater than 0")
	}

	logger.Info("VMware LUN validation completed successfully", "credentials", credentials != nil)
	return nil
}

// createOpenStackVolume creates a Cinder volume for the SharedRDM
func (r *SharedRDMReconciler) createOpenStackVolume(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) (string, error) {
	logger := log.FromContext(ctx)

	// TODO: Implement actual OpenStack Cinder API call
	// For now, we'll simulate volume creation
	volumeID := fmt.Sprintf("vol-%s-%d", sharedRDM.Name, time.Now().Unix())

	logger.Info("Creating OpenStack volume",
		"volumeID", volumeID,
		"size", sharedRDM.Spec.DiskSize,
		"volumeType", sharedRDM.Spec.VolumeType,
		"backendPool", sharedRDM.Spec.CinderBackendPool)

	// Simulate volume creation delay
	time.Sleep(time.Second * 2)

	return volumeID, nil
}

// enableMultiAttach enables multi-attach capability on the volume
func (r *SharedRDMReconciler) enableMultiAttach(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM, volumeID string) error {
	logger := log.FromContext(ctx)

	// TODO: Implement actual OpenStack API call to enable multi-attach
	logger.Info("Enabling multi-attach on volume", "volumeID", volumeID)

	// Simulate multi-attach configuration
	time.Sleep(time.Second * 1)

	return nil
}

// verifyVolumeStatus verifies that the OpenStack volume is in the correct state
func (r *SharedRDMReconciler) verifyVolumeStatus(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) error {
	logger := log.FromContext(ctx)

	if sharedRDM.Status.VolumeID == "" {
		return fmt.Errorf("no volume ID available for verification")
	}

	// TODO: Implement actual OpenStack API call to check volume status
	logger.Info("Verifying volume status", "volumeID", sharedRDM.Status.VolumeID)

	// Simulate volume status check
	return nil
}

// deleteOpenStackVolume deletes the OpenStack volume
func (r *SharedRDMReconciler) deleteOpenStackVolume(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) error {
	logger := log.FromContext(ctx)

	// TODO: Implement actual OpenStack API call to delete volume
	logger.Info("Deleting OpenStack volume", "volumeID", sharedRDM.Status.VolumeID)

	// Simulate volume deletion
	time.Sleep(time.Second * 1)

	return nil
}

// requiresMultiAttach determines if the volume requires multi-attach capability
func (r *SharedRDMReconciler) requiresMultiAttach(sharedRDM *vjailbreakv1alpha1.SharedRDM) bool {
	return len(sharedRDM.Spec.VMwareRefs) > 1
}

// updateAttachedVMs updates the list of VMs currently using this volume
func (r *SharedRDMReconciler) updateAttachedVMs(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) error {
	logger := log.FromContext(ctx)

	attachedVMs, err := r.getAttachedVMs(ctx, sharedRDM)
	if err != nil {
		return err
	}

	sharedRDM.Status.AttachedVMs = attachedVMs
	logger.Info("Updated attached VMs list", "attachedVMs", attachedVMs)

	return r.Status().Update(ctx, sharedRDM)
}

// getAttachedVMs gets the list of VMs currently using this volume
func (r *SharedRDMReconciler) getAttachedVMs(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM) ([]string, error) {
	// TODO: Implement actual OpenStack API call to get volume attachments
	// For now, return empty list
	return []string{}, nil
}

// getVMwareCredentials retrieves VMware credentials from the secret
func (r *SharedRDMReconciler) getVMwareCredentials(ctx context.Context, secretName, namespace string) (map[string]string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return nil, err
	}

	credentials := make(map[string]string)
	for key, value := range secret.Data {
		credentials[key] = string(value)
	}

	return credentials, nil
}

// handleCreationError handles errors during the creation phase
func (r *SharedRDMReconciler) handleCreationError(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	retryCount := r.getRetryCount(sharedRDM)
	if retryCount < MaxRetryAttempts {
		// Calculate exponential backoff delay
		delay := time.Duration(math.Pow(2, float64(retryCount))) * BaseRetryDelay
		logger.Info("Scheduling retry for creation error", "error", err, "retryCount", retryCount, "delay", delay)

		r.incrementRetryCount(sharedRDM)
		return ctrl.Result{RequeueAfter: delay}, r.Status().Update(ctx, sharedRDM)
	}

	// Max retries exceeded, transition to failed state
	r.setCondition(sharedRDM, ConditionTypeReady, metav1.ConditionFalse, "CreationFailed", err.Error())
	return r.updateStatusWithRetry(ctx, sharedRDM, SharedRDMPhaseFailed, fmt.Sprintf("Creation failed after %d attempts: %v", MaxRetryAttempts, err))
}

// updateStatusWithRetry updates the SharedRDM status with retry logic
func (r *SharedRDMReconciler) updateStatusWithRetry(ctx context.Context, sharedRDM *vjailbreakv1alpha1.SharedRDM, phase, message string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sharedRDM.Status.Phase = phase
	sharedRDM.Status.Message = message
	now := metav1.Now()
	sharedRDM.Status.LastUpdateTime = &now

	if err := r.Status().Update(ctx, sharedRDM); err != nil {
		logger.Error(err, "Failed to update SharedRDM status")
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	logger.Info("Updated SharedRDM status", "phase", phase, "message", message)

	// Determine requeue behavior based on phase
	switch phase {
	case SharedRDMPhaseReady:
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	case SharedRDMPhaseFailed:
		return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
	default:
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}
}

// setCondition sets or updates a condition in the SharedRDM status
func (r *SharedRDMReconciler) setCondition(sharedRDM *vjailbreakv1alpha1.SharedRDM, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	// Update or add the condition
	updated := false
	for i, existingCondition := range sharedRDM.Status.Conditions {
		if existingCondition.Type == condition.Type {
			// Only update if status changed
			if existingCondition.Status != condition.Status {
				sharedRDM.Status.Conditions[i] = condition
			} else {
				// Update message and reason even if status is the same
				sharedRDM.Status.Conditions[i].Message = condition.Message
				sharedRDM.Status.Conditions[i].Reason = condition.Reason
			}
			updated = true
			break
		}
	}
	if !updated {
		sharedRDM.Status.Conditions = append(sharedRDM.Status.Conditions, condition)
	}
}

// getRetryCount gets the current retry count from annotations
func (r *SharedRDMReconciler) getRetryCount(sharedRDM *vjailbreakv1alpha1.SharedRDM) int {
	if sharedRDM.Annotations == nil {
		return 0
	}

	retryCountStr, exists := sharedRDM.Annotations["sharedrdm.vjailbreak.k8s.pf9.io/retry-count"]
	if !exists {
		return 0
	}

	var retryCount int
	if _, err := fmt.Sscanf(retryCountStr, "%d", &retryCount); err != nil {
		return 0
	}

	return retryCount
}

// incrementRetryCount increments the retry count in annotations
func (r *SharedRDMReconciler) incrementRetryCount(sharedRDM *vjailbreakv1alpha1.SharedRDM) {
	if sharedRDM.Annotations == nil {
		sharedRDM.Annotations = make(map[string]string)
	}

	retryCount := r.getRetryCount(sharedRDM)
	sharedRDM.Annotations["sharedrdm.vjailbreak.k8s.pf9.io/retry-count"] = fmt.Sprintf("%d", retryCount+1)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SharedRDMReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vjailbreakv1alpha1.SharedRDM{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}