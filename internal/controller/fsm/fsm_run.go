package fsm

import (
	"context"
	"time"

	"github.com/kyma-project/auditlog-manager/internal/btp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	finalizer            = "auditlogmanager.kyma-project.io/finalizer"
	fieldOwner           = "auditlogmanager.kyma-project.io/owner"
	requeueInterval      = 5 * time.Second
	requeueErrorInterval = 30 * time.Second
	requeueCheckInterval = 10 * time.Second
)

func sFnRun(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Running FSM to reconcile BTP auditlog resources", "instance", s.instance.Name)

	instanceIsBeingDeleted := !s.instance.GetDeletionTimestamp().IsZero()
	instanceHasFinalizer := controllerutil.ContainsFinalizer(&s.instance, finalizer)

	if instanceIsBeingDeleted {
		resourcesArePresent, err := resourcesExists(ctx, m, s)
		if err != nil {
			logger.Error(err, "Failed to check resource existence")
			return updateStatusAndRequeueAfter(requeueErrorInterval)
		}
		if resourcesArePresent {
			logger.Info("Deletion not yet implemented, removing finalizer", "instance", s.instance.Name)
			return stop()
		}

		if instanceHasFinalizer {
			return removeFinalizerAndStop(ctx, m, s)
		}

		logger.Info("Auditlog instance is deleted!", "instance", s.instance.Name)
		return stop()
	}

	if !instanceHasFinalizer {
		return addFinalizerAndRequeue(ctx, m, s)
	}

	if s.instance.Spec.SubaccountID == "" {
		return switchState(sFnCreateSubaccount)
	}

	logger.Info("SubaccountID already set in spec, delegating to verification", "subaccountID", s.instance.Spec.SubaccountID)
	return switchState(sFnVerifySubaccount)
}

func addFinalizerAndRequeue(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	controllerutil.AddFinalizer(&s.instance, finalizer)
	err := m.KcpClient.Update(ctx, &s.instance)
	if err != nil {
		return requeueWithError(err)
	}
	return requeue()
}

func removeFinalizerAndStop(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	controllerutil.RemoveFinalizer(&s.instance, finalizer)
	err := m.KcpClient.Update(ctx, &s.instance)
	if err != nil {
		return requeueWithError(err)
	}
	return stop()
}

func resourcesExists(ctx context.Context, m *fsm, s *systemState) (bool, error) {
	if s.instance.Spec.SubaccountID == "" {
		return false, nil
	}

	status, err := m.BTPClient.VerifyLoggingStack(ctx, s.instance.Spec.SubaccountID)
	if err != nil {
		return false, err
	}

	// Resources exist if status is Ready or Processing (not NoInstallation)
	return status == btp.InstallStatusReady || status == btp.InstallStatusProcessing, nil
}
