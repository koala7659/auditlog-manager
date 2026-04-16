package fsm

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// sFnCreateServiceManagerBinding handles Service Manager binding creation.
// This is the second step in provisioning the audit log stack.
//
// TODO: Implement granular Service Manager binding logic
// This will be split into separate steps:
// 1. Create/verify Service Manager binding
// 2. Create/verify auditlog-management instance
// 3. Create/verify auditlog instance
// 4. Create/verify bindings
// 5. Store credentials
func sFnCreateServiceManagerBinding(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Service Manager binding creation not yet implemented", "instance", s.instance.Name)

	// TODO: Implement granular SM binding creation
	// For now, stop reconciliation
	return stop()
}
