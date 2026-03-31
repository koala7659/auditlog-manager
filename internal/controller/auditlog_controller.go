package controller

import (
	"context"
	"time"

	"github.com/kyma-project/auditlog-manager/internal/btp"
	"github.com/kyma-project/auditlog-manager/internal/controller/fsm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	auditlogmanagerv1beta1 "github.com/kyma-project/auditlog-manager/api/v1beta1"
)

const (
	requeueInterval      = 5 * time.Second
	requeueErrorInterval = 30 * time.Second
)

// AuditLogReconciler reconciles a AuditLog object
type AuditLogReconciler struct {
	KCPClient     client.Client
	GardenClient  client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
	Cfg           fsm.FSMCfg
	BTPClient     btp.BTPClient
}

func NewAuditLogReconciler(mgr ctrl.Manager, gardenerClient client.Client, btpClient btp.BTPClient) *AuditLogReconciler {
	return &AuditLogReconciler{
		KCPClient:     mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("auditlog-controller"),
		GardenClient:  gardenerClient,
		BTPClient:     btpClient,
		Cfg:           fsm.FSMCfg{},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuditLogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&auditlogmanagerv1beta1.AuditLog{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("auditlog-controller").
		Complete(r)
}

// +kubebuilder:rbac:groups=auditlogmanager.kyma-project.io,resources=auditlogs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=auditlogmanager.kyma-project.io,resources=auditlogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=auditlogmanager.kyma-project.io,resources=auditlogs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *AuditLogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling AuditLog resource")
	logger.Info(req.String())

	instance := auditlogmanagerv1beta1.AuditLog{}
	if err := r.KCPClient.Get(ctx, req.NamespacedName, &instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("AuditLog resource not found, assuming it was deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	stateFSM := fsm.NewFsm(
		r.Cfg,
		fsm.K8s{
			KcpClient:     r.KCPClient,
			GardenClient:  r.GardenClient,
			EventRecorder: r.EventRecorder,
		},
		r.BTPClient,
	)

	return stateFSM.Run(ctx, instance)
}

/*
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
*/
