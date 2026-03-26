/*
Copyright 2026.

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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	auditlogmanagerv1beta1 "github.com/kyma-project/auditlog-manager.git/api/v1beta1"
)

const (
	requeueInterval      = 5 * time.Second
	requeueErrorInterval = 30 * time.Second
	finalizer            = "auditlogmanager.kyma-project.io/finalizer"
	fieldOwner           = "auditlogmanager.kyma-project.io/owner"
)

// AuditLogReconciler reconciles a AuditLog object
type AuditLogReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	record.EventRecorder
}

func NewAuditLogReconciler(mgr ctrl.Manager) *AuditLogReconciler {
	return &AuditLogReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("auditlog-controller"),
	}
}

// +kubebuilder:rbac:groups=auditlogmanager.kyma-project.io,resources=auditlogs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=auditlogmanager.kyma-project.io,resources=auditlogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=auditlogmanager.kyma-project.io,resources=auditlogs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *AuditLogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling AuditLog resource")

	instance := &auditlogmanagerv1beta1.AuditLog{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("AuditLog resource not found, assuming it was deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !instance.DeletionTimestamp.IsZero() {
		return r.handleDeletingState(ctx, instance)
	}

	// Add finalizer if missing
	if !controllerutil.ContainsFinalizer(instance, finalizer) {
		logger.Info("Adding finalizer")
		controllerutil.AddFinalizer(instance, finalizer)
		if err := r.Update(ctx, instance); err != nil {
			if apierrors.IsConflict(err) {
				// Conflict error is expected when there are concurrent updates
				// Controller-runtime will automatically retry
				logger.Info("Conflict updating finalizer, will retry")
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// State machine dispatch
	switch instance.Status.State {
	case "":
		return r.handleInitialState(ctx, instance)
	case auditlogmanagerv1beta1.StateProcessing:
		return r.handleProcessingState(ctx, instance)
	case auditlogmanagerv1beta1.StateDeleting:
		return r.handleDeletingState(ctx, instance)
	case auditlogmanagerv1beta1.StateError:
		return r.handleErrorState(ctx, instance)
	case auditlogmanagerv1beta1.StateReady, auditlogmanagerv1beta1.StateWarning:
		return r.handleReadyState(ctx, instance)
	default:
		return r.handleInitialState(ctx, instance)
	}
}

func (r *AuditLogReconciler) handleInitialState(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling initial state")

	instance.Status.WithState(auditlogmanagerv1beta1.StateProcessing).
		WithInstallConditionStatus(metav1.ConditionUnknown, instance.Generation)

	if err := r.setInstanceStatus(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *AuditLogReconciler) handleProcessingState(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling processing state")

	if err := r.processAuditLogResources(ctx, instance); err != nil {
		logger.Error(err, "Failed to process AuditLog resources")

		instance.Status.WithState(auditlogmanagerv1beta1.StateError).
			WithInstallConditionStatus(metav1.ConditionFalse, instance.Generation)

		if statusErr := r.setInstanceStatus(ctx, instance); statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{RequeueAfter: requeueErrorInterval}, err
	}

	instance.Status.WithState(auditlogmanagerv1beta1.StateReady).
		WithInstallConditionStatus(metav1.ConditionTrue, instance.Generation)

	if err := r.setInstanceStatus(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *AuditLogReconciler) handleReadyState(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling ready state")

	// Check for spec changes
	statusCondition := meta.FindStatusCondition(instance.Status.Conditions, auditlogmanagerv1beta1.ConditionTypeStartup)
	if statusCondition != nil && statusCondition.ObservedGeneration != instance.Generation {
		logger.Info("Spec changed detected, transitioning to Processing")

		instance.Status.WithState(auditlogmanagerv1beta1.StateProcessing).
			WithInstallConditionStatus(metav1.ConditionUnknown, instance.Generation)

		if err := r.setInstanceStatus(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	// TODO: Add periodic health check when actual verification logic is implemented
	// if err := r.verifyAuditLogResources(ctx, instance); err != nil {
	//     logger.Error(err, "Health check failed")
	//     instance.Status.WithState(auditlogmanagerv1beta1.StateWarning)
	//     if statusErr := r.setInstanceStatus(ctx, instance); statusErr != nil {
	//         return ctrl.Result{}, statusErr
	//     }
	// }

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *AuditLogReconciler) handleErrorState(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling error state, attempting recovery")

	instance.Status.WithState(auditlogmanagerv1beta1.StateProcessing).
		WithInstallConditionStatus(metav1.ConditionUnknown, instance.Generation)

	if err := r.setInstanceStatus(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueErrorInterval}, nil
}

func (r *AuditLogReconciler) handleDeletingState(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling deleting state")

	if !controllerutil.ContainsFinalizer(instance, finalizer) {
		return ctrl.Result{}, nil
	}

	// Transition to Deleting state if not already
	if instance.Status.State != auditlogmanagerv1beta1.StateDeleting {
		logger.Info("Transitioning to Deleting state")
		instance.Status.WithState(auditlogmanagerv1beta1.StateDeleting).
			WithInstallConditionStatus(metav1.ConditionFalse, instance.Generation)

		if err := r.setInstanceStatus(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}

		// Requeue to continue with cleanup in next reconciliation
		// This ensures status is persisted before starting cleanup
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if cleanup is complete
	cleanupComplete, err := r.isCleanupComplete(ctx, instance)
	if err != nil {
		logger.Error(err, "Failed to check cleanup status")
		return ctrl.Result{}, err
	}

	if !cleanupComplete {
		logger.Info("Cleanup in progress, performing cleanup operations")
		if err := r.cleanupAuditLogResources(ctx, instance); err != nil {
			logger.Error(err, "Failed to cleanup resources")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}

		// Requeue to check cleanup status again
		logger.Info("Cleanup operations executed, will verify completion")
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	// Cleanup is complete, remove finalizer
	logger.Info("Cleanup complete, removing finalizer")
	controllerutil.RemoveFinalizer(instance, finalizer)
	if err := r.Update(ctx, instance); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Conflict removing finalizer, will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	r.Event(instance, corev1.EventTypeNormal, "Deleted", "Successfully deleted AuditLog resource")
	return ctrl.Result{}, nil
}

func (r *AuditLogReconciler) setInstanceStatus(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) error {
	return r.setStatusForObjectInstance(ctx, instance)
}

func (r *AuditLogReconciler) setStatusForObjectInstance(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) error {
	logger := logf.FromContext(ctx)

	if err := r.ssaStatus(ctx, instance); err != nil {
		logger.Error(err, "Failed to update status")
		r.Event(instance, corev1.EventTypeWarning, "StatusUpdateFailed",
			fmt.Sprintf("Failed to update status: %v", err))
		return err
	}

	r.Event(instance, corev1.EventTypeNormal, "StatusUpdated",
		fmt.Sprintf("Status updated to %s", instance.Status.State))

	return nil
}

func (r *AuditLogReconciler) ssaStatus(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) error {
	patch := &auditlogmanagerv1beta1.AuditLog{
		TypeMeta: metav1.TypeMeta{
			APIVersion: auditlogmanagerv1beta1.GroupVersion.String(),
			Kind:       "AuditLog",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            instance.Name,
			Namespace:       instance.Namespace,
			ResourceVersion: instance.ResourceVersion,
		},
		Status: instance.Status,
	}

	return r.Status().Update(ctx, patch, &client.SubResourceUpdateOptions{
		UpdateOptions: client.UpdateOptions{
			FieldManager: fieldOwner,
		},
	})
}

// processAuditLogResources is the main processing function that creates/updates audit logging resources
// TODO: Implement actual audit log resource creation and management
//
//nolint:unparam // will return error when implemented
func (r *AuditLogReconciler) processAuditLogResources(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) error {
	logger := logf.FromContext(ctx)
	logger.Info("Processing AuditLog resources",
		"region", instance.Spec.Region,
		"runtimeID", instance.Spec.RuntimeID,
		"subaccountID", instance.Spec.SubaccountID,
		"tenantID", instance.Spec.TenantID)

	// STUB: Actual implementation will create:
	// - DaemonSet for audit log collection
	// - ConfigMap with audit policy
	// - Service/Endpoints for log forwarding
	// - Secrets with credentials
	// - Aggregator/forwarder components

	logger.Info("Successfully processed AuditLog resources (stub)")
	return nil
}

// verifyAuditLogResources performs health checks on audit log resources
// TODO: Implement actual health verification logic
//
//nolint:unused // will be used when health checks are implemented
func (r *AuditLogReconciler) verifyAuditLogResources(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) error {
	logger := logf.FromContext(ctx)
	logger.Info("Verifying AuditLog resources health")

	// STUB: Actual implementation will check:
	// - DaemonSet pods are running
	// - Log forwarding is active
	// - Connectivity to external audit log service
	// - Configuration integrity

	logger.Info("Health verification passed (stub)")
	return nil
}

// cleanupAuditLogResources performs cleanup when AuditLog is deleted
// TODO: Implement actual cleanup logic
func (r *AuditLogReconciler) cleanupAuditLogResources(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) error {
	logger := logf.FromContext(ctx)

	// Check if cleanup timestamp annotation exists
	if instance.Annotations == nil {
		instance.Annotations = make(map[string]string)
	}

	cleanupStartTime, exists := instance.Annotations["auditlogmanager.kyma-project.io/cleanup-started"]
	if !exists {
		// First time entering cleanup - record the start time
		instance.Annotations["auditlogmanager.kyma-project.io/cleanup-started"] = time.Now().Format(time.RFC3339)
		if err := r.Update(ctx, instance); err != nil {
			if apierrors.IsConflict(err) {
				logger.Info("Conflict updating cleanup annotation, will retry")
				return nil
			}
			return err
		}
		logger.Info("Started cleanup process, recorded timestamp")
		return nil
	}

	logger.Info("Cleanup in progress",
		"startTime", cleanupStartTime,
		"elapsed", time.Since(parseTime(cleanupStartTime)).String())

	// STUB: Actual implementation will:
	// - Delete owned resources (DaemonSets, ConfigMaps, Secrets)
	// - Deregister from external audit log service
	// - Clean up temporary data
	// - Revoke credentials

	return nil
}

// isCleanupComplete checks if all cleanup operations have finished
// Returns true if cleanup is complete, false if still in progress
func (r *AuditLogReconciler) isCleanupComplete(ctx context.Context, instance *auditlogmanagerv1beta1.AuditLog) (bool, error) {
	logger := logf.FromContext(ctx)

	if instance.Annotations == nil {
		// No cleanup started yet
		return false, nil
	}

	cleanupStartTime, exists := instance.Annotations["auditlogmanager.kyma-project.io/cleanup-started"]
	if !exists {
		// No cleanup started yet
		return false, nil
	}

	startTime := parseTime(cleanupStartTime)
	elapsed := time.Since(startTime)

	// STUB: Simulate 20-second cleanup process
	// In real implementation, this would check if:
	// - All owned resources are deleted
	// - External service deregistration is complete
	// - Credentials are revoked
	// - All cleanup operations finished successfully

	const cleanupTimeout = 20 * time.Second
	if elapsed < cleanupTimeout {
		logger.Info("Cleanup still in progress",
			"elapsed", elapsed.Round(time.Second).String(),
			"remaining", (cleanupTimeout - elapsed).Round(time.Second).String())
		return false, nil
	}

	logger.Info("Cleanup completed", "duration", elapsed.Round(time.Second).String())
	return true, nil
}

// parseTime is a helper to parse RFC3339 time strings
func parseTime(timeStr string) time.Time {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		// If parsing fails, return current time to avoid blocking
		return time.Now()
	}
	return t
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuditLogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&auditlogmanagerv1beta1.AuditLog{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("auditlog").
		Complete(r)
}
