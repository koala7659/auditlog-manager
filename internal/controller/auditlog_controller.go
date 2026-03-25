/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	auditlogmanagerv1beta1 "github.com/kyma-project/auditlog-manager.git/api/v1beta1"
)

// AuditLogReconciler reconciles a AuditLog object
type AuditLogReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=auditlogmanager.kyma-project.io,resources=auditlogs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=auditlogmanager.kyma-project.io,resources=auditlogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=auditlogmanager.kyma-project.io,resources=auditlogs/finalizers,verbs=update

func (r *AuditLogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling AuditLog resource", "namespace", req.Namespace, "name", req.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuditLogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&auditlogmanagerv1beta1.AuditLog{}).
		Named("auditlog").
		Complete(r)
}
