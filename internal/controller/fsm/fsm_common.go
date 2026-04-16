package fsm

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

func updateStatusAndRequeue() (stateFn, *ctrl.Result, error) {
	return sFnUpdateStatus(&ctrl.Result{Requeue: true}, nil), nil, nil
}

func updateStatusAndRequeueAfter(
	//nolint:unparam
	duration time.Duration) (stateFn, *ctrl.Result, error) {
	return sFnUpdateStatus(&ctrl.Result{RequeueAfter: duration}, nil), nil, nil
}

func updateStatusAndStop() (stateFn, *ctrl.Result, error) {
	return sFnUpdateStatus(nil, nil), nil, nil
}

func requeue() (stateFn, *ctrl.Result, error) {
	return nil, &ctrl.Result{Requeue: true}, nil
}

func requeueWithError(err error) (stateFn, *ctrl.Result, error) {
	return nil, nil, err
}

func requeueAfter(d time.Duration) (stateFn, *ctrl.Result, error) {
	return nil, &ctrl.Result{RequeueAfter: d}, nil
}

func stop() (stateFn, *ctrl.Result, error) {
	return nil, nil, nil
}

func switchState(fn stateFn) (stateFn, *ctrl.Result, error) {
	return fn, nil, nil
}

//func stopWithMetrics() (stateFn, *ctrl.Result, error) {
//	return func(_ context.Context, m *fsm, _ *systemState) (stateFn, *ctrl.Result, error) {
//		return stop()
//	}, nil, nil
//}
