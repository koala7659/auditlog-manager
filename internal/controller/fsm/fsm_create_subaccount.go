package fsm

import (
	"context"
	"fmt"

	auditlogmanagerv1beta1 "github.com/kyma-project/auditlog-manager/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// sFnCreateSubaccount handles BTP subaccount creation.
// This is the first step in provisioning the audit log stack.
//
// Flow:
// 1. If SubaccountID not in spec → create new subaccount, update spec, stop (reconciliation triggered automatically)
func sFnCreateSubaccount(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	if s.instance.Spec.SubaccountID != "" {
		logger.Error(nil, "SubaccountID is already set in spec - cannot create new one", "subaccountID", s.instance.Spec.SubaccountID)
		return stop()
	}

	// Step 1: No SubaccountID in spec, need to create new subaccount
	logger.Info("Creating new BTP subaccount", "region", s.instance.Spec.Region, "name", s.instance.Name)
	s.instance.Status.WithState(auditlogmanagerv1beta1.StatePending)

	// Generate display name for subaccount
	displayName := fmt.Sprintf("auditlog-%s", s.instance.Name)

	subaccountGUID, err := m.BTPClient.CreateSubaccount(
		ctx,
		s.instance.Spec.Region,
		s.instance.Spec.GlobalAccountID,
		displayName,
		s.instance.Spec.Administrators,
	)
	if err != nil {
		logger.Error(err, "Failed to create subaccount")
		s.instance.Status.WithCondition(
			auditlogmanagerv1beta1.ConditionTypeSubaccountReady,
			metav1.ConditionFalse,
			auditlogmanagerv1beta1.ConditionReasonFailed,
			fmt.Sprintf("Subaccount creation failed: %v", err),
			s.instance.Generation,
		)
		return updateStatusAndRequeueAfter(requeueErrorInterval)
	}

	logger.Info("Subaccount creation initiated", "subaccountGUID", subaccountGUID)

	// Step 3: Update status condition to show creation initiated
	s.instance.Status.WithCondition(
		auditlogmanagerv1beta1.ConditionTypeSubaccountReady,
		metav1.ConditionUnknown,
		auditlogmanagerv1beta1.ConditionReasonCreated,
		fmt.Sprintf("Subaccount creation initiated, GUID: %s", subaccountGUID),
		s.instance.Generation,
	)

	// Step 4: Update status first (provides better observability)
	// Note: Status().Update() updates s.instance with the latest resourceVersion from server
	if err := m.KcpClient.Status().Update(ctx, &s.instance, &client.SubResourceUpdateOptions{
		UpdateOptions: client.UpdateOptions{
			FieldManager: fieldOwner,
		}}); err != nil {
		logger.Error(err, "Failed to update status after subaccount creation")
		// Continue anyway - status update failure shouldn't block progress
	}

	// Step 5: Store SubaccountID in spec (persistent state)
	// The s.instance object now has the latest resourceVersion from the Status().Update() above,
	// so this spec update is safe from conflicts
	s.instance.Spec.SubaccountID = subaccountGUID
	if err := m.KcpClient.Update(ctx, &s.instance, &client.UpdateOptions{
		FieldManager: fieldOwner,
	}); err != nil {
		logger.Error(err, "Failed to update spec with SubaccountID")
		s.instance.Status.WithCondition(
			auditlogmanagerv1beta1.ConditionTypeSubaccountReady,
			metav1.ConditionFalse,
			auditlogmanagerv1beta1.ConditionReasonFailed,
			fmt.Sprintf("Failed to update spec with SubaccountID: %v", err),
			s.instance.Generation,
		)
		return updateStatusAndStop()
	}

	// Step 6: Stop reconciliation
	// The spec update above will trigger a new reconciliation automatically
	logger.Info("Subaccount creation initiated, stopping reconciliation (spec update will trigger new reconciliation)")
	return stop()
}
