package fsm

import (
	"context"

	auditlogmanagerv1beta1 "github.com/kyma-project/auditlog-manager/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func sFnCreateResources(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Transition to Processing state if not already
	if s.instance.Status.State != auditlogmanagerv1beta1.StateProcessing {
		logger.Info("Transitioning to Processing state")
		s.instance.Status.WithState(auditlogmanagerv1beta1.StateProcessing).
			WithInstallConditionStatus(metav1.ConditionFalse, s.instance.Generation)

		return updateStatusAndRequeue()
	}

	logger.Info("Creating Resources for instance", "instance", s.instance.Name, "tenantID", s.instance.Spec.TenantID)

	// Call BTP client with error handling
	if err := m.BTPClient.CreateLoggingStack(ctx, s.instance.Spec.TenantID); err != nil {
		logger.Error(err, "Failed to create logging stack")
		s.instance.Status.WithState(auditlogmanagerv1beta1.StateError).
			WithInstallConditionStatus(metav1.ConditionFalse, s.instance.Generation)
		return updateStatusAndRequeueAfter(requeueErrorInterval)
	}

	logger.Info("Logging stack creation initiated, transitioning to verification")
	// Transition to verification state
	return switchState(sFnVerifyCreation)
}
