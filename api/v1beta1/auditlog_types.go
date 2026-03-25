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

package v1beta1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// State defines the observed state of AuditLog
// +kubebuilder:validation:Enum=Ready;Processing;Warning;Error;Deleting
type State string

// AuditLogSpec defines the desired state of AuditLog
type AuditLogSpec struct {
	// Region is a field of AuditLog that defines BTP region where custom audit log stack is created.
	// +required
	Region string `json:"region"`
	// SubaccountID is a field of AuditLog that defines SubaccountID owner subaccount for audit log.
	// +required
	SubaccountID string `json:"subaccountID"`
	// RuntimeID is a field of AuditLog that defines the Kyma Runtime that is associated with audit logging stack.
	// +required
	RuntimeID string `json:"runtimeID"`
	// TenantID is a field of AuditLog that defines the tenant ID that is associated with audit logging stack.
	// +required
	TenantID string `json:"tenantID"`
}

// Valid AuditLog States.
const (
	// StateReady signifies AuditLog is ready and has been created successfully.
	StateReady State = "Ready"

	// StateProcessing signifies AuditLog is reconciling and is in the process of Creation.
	// Processing can also signal that the Installation previously encountered an error and is now recovering.
	StateProcessing State = "Processing"

	// StateWarning signifies a warning for AuditLog. This signifies that the Creation
	// process encountered a problem.
	StateWarning State = "Warning"

	// StateError signifies an error for AuditLog. This signifies that the Creation
	// process encountered an error.
	// Contrary to Processing, it can be expected that this state should change on the next retry.
	StateError State = "Error"

	// StateDeleting signifies AuditLog has been deleted.
	// This is the state that is used when a deletionTimestamp was detected and Finalizers are picked up.
	StateDeleting State = "Deleting"
)

var (
	ConditionTypeStartup = "Starting"
	ConditionReasonReady = "Ready"
)

// AuditLogStatus defines the observed state of AuditLog.
type AuditLogStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the AuditLog resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// state represents the current state of the AuditLog resource.
	// +kubebuilder:validation:Enum=Ready;Processing;Warning;Error;Deleting
	// +kubebuilder:default=Processing
	State State `json:"state"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state",description="State of the AuditLog"
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=".spec.region",description="BTP Region"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AuditLog is the Schema for the auditlogs API
type AuditLog struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec AuditLogSpec `json:"spec"`

	// status defines the observed state of AuditLog
	// +optional
	Status AuditLogStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AuditLogList contains a list of AuditLog
type AuditLogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AuditLog `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuditLog{}, &AuditLogList{})
}

func (s *AuditLogStatus) WithState(state State) *AuditLogStatus {
	s.State = state
	return s
}

func (s *AuditLogStatus) WithInstallConditionStatus(status metav1.ConditionStatus, objGeneration int64) *AuditLogStatus {
	if s.Conditions == nil {
		s.Conditions = make([]metav1.Condition, 0, 1)
	}

	condition := meta.FindStatusCondition(s.Conditions, ConditionTypeStartup)

	if condition == nil {
		condition = &metav1.Condition{
			Type:    ConditionTypeStartup,
			Reason:  ConditionReasonReady,
			Message: "Starting auditlog instance",
		}
	}

	condition.Status = status
	condition.ObservedGeneration = objGeneration
	meta.SetStatusCondition(&s.Conditions, *condition)
	return s
}
