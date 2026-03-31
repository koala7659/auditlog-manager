package fsm

import (
	"context"
	"reflect"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func sFnUpdateStatus(result *ctrl.Result, err error) stateFn {
	return func(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
		logger := log.FromContext(ctx)

		if err != nil {
			//m.Metrics.IncRuntimeFSMStopCounter()
		}
		// make sure there is a change in status
		if reflect.DeepEqual(s.instance.Status, s.snapshot) {
			return nil, result, err
		}

		updateErr := m.KcpClient.Status().Update(ctx, &s.instance, &client.SubResourceUpdateOptions{
			UpdateOptions: client.UpdateOptions{
				FieldManager: fieldOwner,
			},
		})

		if updateErr != nil {
			logger.Error(updateErr, "unable to update instance status!")
			if err == nil {
				err = updateErr
			}
			return nil, nil, err
		}

		//m.Metrics.SetRuntimeStates(s.instance)
		next := sFnEmmitEvent(nil, result, err)
		return next, nil, nil
	}
}
