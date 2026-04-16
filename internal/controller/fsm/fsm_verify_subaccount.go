package fsm

import (
	"context"
	"fmt"

	auditlogmanagerv1beta1 "github.com/kyma-project/auditlog-manager/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// sFnVerifySubaccount verifies the BTP subaccount state and updates conditions accordingly.
// This state is called after SubaccountID is set in spec.
//
// Flow:
// 1. Verify subaccount exists in BTP via API call
// 2. If doesn't exist → UNRECOVERABLE ERROR (audit data lost)
// 3. If exists and ready → proceed to next step
// 4. If exists but not ready → requeue and poll

func sFnVerifySubaccount(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	if s.instance.Spec.SubaccountID == "" {
		logger.Error(nil, "SubaccountID not set in spec - cannot verify")
		return stop()
	}

	logger.Info("Verifying BTP subaccount", "subaccountID", s.instance.Spec.SubaccountID)

	// ALWAYS verify with BTP API - never trust condition state alone
	exists, state, err := m.BTPClient.GetSubaccount(ctx, s.instance.Spec.SubaccountID)
	if err != nil {
		logger.Error(err, "Failed to verify subaccount existence")
		s.instance.Status.WithCondition(
			auditlogmanagerv1beta1.ConditionTypeSubaccountReady,
			metav1.ConditionFalse,
			auditlogmanagerv1beta1.ConditionReasonFailed,
			fmt.Sprintf("Subaccount verification failed: %v", err),
			s.instance.Generation,
		)
		return updateStatusAndRequeueAfter(requeueErrorInterval)
	}

	if !exists {
		// UNRECOVERABLE ERROR: Subaccount was deleted externally
		// We cannot recreate it because audit log data is permanently lost
		logger.Error(nil, "Subaccount deleted externally - UNRECOVERABLE",
			"subaccountID", s.instance.Spec.SubaccountID,
			"reason", "Audit log data permanently lost")
		s.instance.Status.WithCondition(
			auditlogmanagerv1beta1.ConditionTypeSubaccountReady,
			metav1.ConditionFalse,
			"SubaccountLost",
			fmt.Sprintf("Subaccount %s was deleted externally. Audit log data is permanently lost. Manual intervention required.", s.instance.Spec.SubaccountID),
			s.instance.Generation,
		)
		// Keep SubaccountID in spec for audit trail
		// Stop reconciliation - this is permanent failure
		return updateStatusAndStop()
	}

	// Subaccount exists, check its state
	switch state {
	case "OK":
		logger.Info("Subaccount is ready", "subaccountID", s.instance.Spec.SubaccountID, "state", state)

		// Check if condition already set to True
		condition := meta.FindStatusCondition(s.instance.Status.Conditions, auditlogmanagerv1beta1.ConditionTypeSubaccountReady)
		if condition != nil && condition.Status == metav1.ConditionTrue {
			// Condition already set, proceed to next state
			logger.Info("SubaccountReady condition already True, proceeding to Service Manager binding")
			return switchState(sFnCreateServiceManagerBinding)
		}

		// Set condition to True and requeue (next reconciliation will proceed to next state)
		s.instance.Status.WithCondition(
			auditlogmanagerv1beta1.ConditionTypeSubaccountReady,
			metav1.ConditionTrue,
			auditlogmanagerv1beta1.ConditionReasonReady,
			"Subaccount provisioned and ready",
			s.instance.Generation,
		)
		return updateStatusAndRequeue()

	case "CREATING":
		logger.Info("Subaccount is still being created", "subaccountID", s.instance.Spec.SubaccountID, "state", state)
		s.instance.Status.WithCondition(
			auditlogmanagerv1beta1.ConditionTypeSubaccountReady,
			metav1.ConditionUnknown,
			auditlogmanagerv1beta1.ConditionReasonProvisioning,
			"Subaccount creation in progress",
			s.instance.Generation,
		)
		// Poll again after check interval
		return updateStatusAndRequeueAfter(requeueCheckInterval)

	case "DELETING":
		logger.Info("Subaccount is being deleted", "subaccountID", s.instance.Spec.SubaccountID, "state", state)
		s.instance.Status.WithCondition(
			auditlogmanagerv1beta1.ConditionTypeSubaccountReady,
			metav1.ConditionFalse,
			"SubaccountDeleting",
			"Subaccount is being deleted externally - will become unrecoverable",
			s.instance.Generation,
		)
		// Wait and check again - if it disappears, it becomes unrecoverable error
		return updateStatusAndRequeueAfter(requeueCheckInterval)

	case "ERROR", "SUSPENDED":
		logger.Info("Subaccount is in error state", "subaccountID", s.instance.Spec.SubaccountID, "state", state)
		s.instance.Status.WithCondition(
			auditlogmanagerv1beta1.ConditionTypeSubaccountReady,
			metav1.ConditionFalse,
			auditlogmanagerv1beta1.ConditionReasonFailed,
			fmt.Sprintf("Subaccount in %s state", state),
			s.instance.Generation,
		)
		// Error state - retry after error interval
		return updateStatusAndRequeueAfter(requeueErrorInterval)

	default:
		logger.Info("Subaccount in unknown state", "subaccountID", s.instance.Spec.SubaccountID, "state", state)
		s.instance.Status.WithCondition(
			auditlogmanagerv1beta1.ConditionTypeSubaccountReady,
			metav1.ConditionUnknown,
			auditlogmanagerv1beta1.ConditionReasonProvisioning,
			fmt.Sprintf("Subaccount in state: %s", state),
			s.instance.Generation,
		)
		// Unknown state - poll again
		return updateStatusAndRequeueAfter(requeueCheckInterval)
	}
}
