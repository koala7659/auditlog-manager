package fsm

import (
	"context"

	auditlogmanagerv1beta1 "github.com/kyma-project/auditlog-manager/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func sFnDeleteResources(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Transition to Deleting state if not already
	if s.instance.Status.State != auditlogmanagerv1beta1.StateDeleting {
		logger.Info("Transitioning to Deleting state")
		s.instance.Status.WithState(auditlogmanagerv1beta1.StateDeleting).
			WithInstallConditionStatus(metav1.ConditionFalse, s.instance.Generation)

		return updateStatusAndRequeue()
	}

	logger.Info("Deleting Resources for instance", "instance", s.instance.Name, "tenantID", s.instance.Spec.TenantID)

	// Call BTP client with error handling
	if err := m.BTPClient.DeleteLoggingStack(ctx, s.instance.Spec.TenantID); err != nil {
		logger.Error(err, "Failed to delete logging stack")
		// Continue with verification anyway - deletion might have partially succeeded
		// Verification will determine actual state
	}

	logger.Info("Logging stack deletion initiated, transitioning to verification")
	// Transition to verification
	return switchState(sFnVerifyDeletion)
}
