package fsm

import (
	"context"
	"fmt"
	"reflect"
	"runtime"

	auditlogapi "github.com/kyma-project/auditlog-manager/api/v1beta1"
	btp "github.com/kyma-project/auditlog-manager/internal/btp"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type systemState struct {
	instance auditlogapi.AuditLog
	snapshot auditlogapi.AuditLogStatus
}

type stateFn func(context.Context, *fsm, *systemState) (stateFn, *ctrl.Result, error)

// auditlog reconciler specific configuration
type FSMCfg struct {
}

func (f stateFn) String() string {
	return f.name()
}

func (f stateFn) name() string {
	name := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	return name
}

type K8s struct {
	KcpClient    client.Client
	GardenClient client.Client
	record.EventRecorder
}

//mockery:generate: false
type Fsm interface {
	Run(ctx context.Context, v auditlogapi.AuditLog) (ctrl.Result, error)
}

type fsm struct {
	fn stateFn
	K8s
	FSMCfg
	btp.BTPClient
}

func NewFsm(cfg FSMCfg, k8s K8s, btpClinet btp.BTPClient) Fsm {
	return &fsm{
		fn:        sFnRun,
		FSMCfg:    cfg,
		K8s:       k8s,
		BTPClient: btpClinet,
	}
}

func (m *fsm) Run(ctx context.Context, v auditlogapi.AuditLog) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	state := systemState{instance: v}
	var err error
	var result *ctrl.Result
loop:
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			break loop
		default:
			prevStateFnName := m.fn.name()
			m.fn, result, err = m.fn(ctx, m, &state)
			nextStateFnName := m.fn.name()
			logger.Info(fmt.Sprintf("switching FSM state from %s to %s", prevStateFnName, nextStateFnName))
			// m.log.V(log_level.TRACE).WithValues("result", result, "err", err, "mFnIsNill", m.fn == nil).Info(fmt.Sprintf("switching state from %s to %s", stateFnName, newStateFnName))
			if m.fn == nil || err != nil {
				break loop
			}
		}
	}

	logger.WithValues("result", result).Info("Reconciliation done")

	if result != nil {
		return *result, err
	}

	return ctrl.Result{}, err
}
