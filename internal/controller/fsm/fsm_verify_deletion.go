package fsm

import (
	"context"

	"github.com/kyma-project/auditlog-manager/internal/btp"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func sFnVerifyDeletion(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	logger.Info("Verifying logging stack deletion", "instance", s.instance.Name, "tenantID", s.instance.Spec.TenantID)

	status, err := m.BTPClient.VerifyLoggingStack(ctx, s.instance.Spec.TenantID)
	if err != nil {
		logger.Error(err, "Failed to verify deletion")
		// On error, retry verification after error interval
		return updateStatusAndRequeueAfter(requeueErrorInterval)
	}

	switch status {
	case btp.NoInstallationStatus:
		logger.Info("Logging stack deleted successfully")
		// Deletion complete - finalizer removal will be handled in sFnRun
		return stop()

	case btp.InstallStatusProcessing, btp.InstallStatusReady, btp.InstallStatusError:
		// Resources still exist, keep waiting
		logger.Info("Waiting for deletion to complete", "status", status)
		return requeueAfter(requeueCheckInterval)

	default:
		logger.Info("Unexpected status during deletion, will retry", "status", status)
		return requeueAfter(requeueCheckInterval)
	}
}
