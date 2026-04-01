package fsm

import (
	"context"

	auditlogmanagerv1beta1 "github.com/kyma-project/auditlog-manager/api/v1beta1"
	"github.com/kyma-project/auditlog-manager/internal/btp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func sFnVerifyCreation(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	logger.Info("Verifying logging stack creation", "instance", s.instance.Name, "tenantID", s.instance.Spec.TenantID)

	status, err := m.BTPClient.VerifyLoggingStack(ctx, s.instance.Spec.TenantID)
	if err != nil {
		logger.Error(err, "Failed to verify logging stack")
		s.instance.Status.WithState(auditlogmanagerv1beta1.StateError).
			WithInstallConditionStatus(metav1.ConditionFalse, s.instance.Generation)
		return updateStatusAndRequeueAfter(requeueErrorInterval)
	}

	switch status {
	case btp.InstallStatusReady:
		logger.Info("Logging stack is ready")
		s.instance.Status.WithState(auditlogmanagerv1beta1.StateReady).
			WithInstallConditionStatus(metav1.ConditionTrue, s.instance.Generation)
		return updateStatusAndRequeueAfter(requeueInterval)

	case btp.InstallStatusProcessing:
		logger.Info("Logging stack still processing")
		// Keep state as Processing, requeue to check again
		return requeueAfter(requeueCheckInterval)

	case btp.InstallStatusError:
		logger.Info("Logging stack in error state")
		s.instance.Status.WithState(auditlogmanagerv1beta1.StateError).
			WithInstallConditionStatus(metav1.ConditionFalse, s.instance.Generation)
		return updateStatusAndRequeueAfter(requeueErrorInterval)

	case btp.NoInstallationStatus:
		logger.Info("No installation found, retrying creation")
		// This shouldn't happen after creation, but handle it by retrying
		return switchState(sFnCreateResources)

	default:
		logger.Info("Unexpected status, will retry verification", "status", status)
		return requeueAfter(requeueCheckInterval)
	}
}
