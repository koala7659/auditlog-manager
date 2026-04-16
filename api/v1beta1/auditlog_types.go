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
// +kubebuilder:validation:Enum=Pending;RegistrationReady;SimApproved;Assigned;Orphaned
type State string

// AuditLogConfig contains configuration for audit log service
type AuditLogConfig struct {
	// ServiceURL is the Audit Log Service URL
	// +optional
	ServiceURL string `json:"serviceURL,omitempty"`
	// TenantID is the tenant ID for the audit log service
	// +optional
	TenantID string `json:"tenantID,omitempty"`
	// GardenerSecretName is the name of Gardener secret with write credentials
	// +optional
	GardenerSecretName string `json:"gardenerSecretName,omitempty"`
	// ConfigMapRef is the name of ConfigMap containing audit log policies
	// +optional
	ConfigMapRef string `json:"configMapRef,omitempty"`
}

// ReadCredentials contains read-only credentials for accessing audit logs
type ReadCredentials struct {
	// URL is the audit log read endpoint
	// +optional
	URL string `json:"url,omitempty"`
	// Username for read access
	// +optional
	Username string `json:"username,omitempty"`
	// Password for read access (base64 encoded)
	// +optional
	Password string `json:"password,omitempty"`
}

// AuditLogSpec defines the desired state of AuditLog
type AuditLogSpec struct {
	// Region is the BTP region where the audit log subaccount is provisioned
	// +required
	Region string `json:"region"`
	// GlobalAccountID is the BTP global account ID where the subaccount will be created
	// +required
	// +kubebuilder:validation:MinLength=1
	GlobalAccountID string `json:"globalAccountID"`
	// Administrators is the list of admin email addresses for the subaccount
	// +required
	// +kubebuilder:validation:MinItems=1
	Administrators []string `json:"administrators"`
	// SubaccountID is the BTP subaccount ID (provisioned by controller)
	// +optional
	SubaccountID string `json:"subaccountID,omitempty"`
	// Config contains configuration for audit log service
	// +optional
	Config AuditLogConfig `json:"config,omitempty"`
	// ReadCredentials contains read-only credentials stored directly in the resource
	// +optional
	ReadCredentials ReadCredentials `json:"readCredentials,omitempty"`
	// RetentionDays is the retention period in days (default: 90)
	// +kubebuilder:default=90
	// +optional
	RetentionDays int `json:"retentionDays,omitempty"`
}

// Valid AuditLog States.
const (
	// StatePending signifies AuditLog is pending provisioning of BTP resources.
	StatePending State = "Pending"

	// StateRegistrationReady signifies BTP resources are fully provisioned and ready for SIM registration.
	StateRegistrationReady State = "RegistrationReady"

	// StateSimApproved signifies the subaccount has been approved by SIM team and is available in the pool.
	StateSimApproved State = "SimApproved"

	// StateAssigned signifies AuditLog is assigned to and in use by a Kyma Runtime.
	StateAssigned State = "Assigned"

	// StateOrphaned signifies the runtime has been deleted but audit logs are retained for the retention period.
	StateOrphaned State = "Orphaned"
)

// Condition types for detailed status tracking
const (
	// ConditionTypeSubaccountReady indicates if BTP subaccount exists and is accessible
	ConditionTypeBTPResources = "BTPResourcesProvisioned"

	// ConditionTypeSubaccountReady indicates if BTP subaccount exists and is accessible
	ConditionTypeSubaccountReady = "SubaccountReady"

	// ConditionTypeServiceInstanceReady indicates if Audit Log Service instance is provisioned
	ConditionTypeServiceInstanceReady = "ServiceInstanceReady"

	// ConditionTypeBindingReady indicates if service binding exists and credentials are retrievable
	ConditionTypeBindingReady = "BindingReady"

	// ConditionTypeCredentialsStored indicates if credentials are stored in Gardener and AuditLog CR
	ConditionTypeCredentialsStored = "CredentialsStored"

	// ConditionTypeReady indicates overall resource health
	ConditionTypeReady = "ResourcesReady"
)

// Condition reasons
const (
	ConditionReasonCreated       = "Created"
	ConditionReasonProvisioning  = "Provisioning"
	ConditionReasonReady         = "Ready"
	ConditionReasonFailed        = "Failed"
	ConditionReasonCISAPIError   = "CISAPIError"
	ConditionReasonQuotaExceeded = "QuotaExceeded"
)

// AuditLogStatus defines the observed state of AuditLog.
type AuditLogStatus struct {
	// conditions represent the current state of the AuditLog resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// state represents the current state of the AuditLog resource.
	// +kubebuilder:validation:Enum=Pending;RegistrationReady;SimApproved;Assigned;Orphaned
	// +kubebuilder:default=Pending
	State State `json:"state"`

	// assignedToRuntime is the runtime ID this AuditLog is assigned to (empty when in pool)
	// +optional
	AssignedToRuntime string `json:"assignedToRuntime,omitempty"`

	// createdAt is the timestamp when the AuditLog was created
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// assignedAt is the timestamp when the AuditLog was assigned to a runtime
	// +optional
	AssignedAt *metav1.Time `json:"assignedAt,omitempty"`

	// orphanedAt is the timestamp when the AuditLog became orphaned
	// +optional
	OrphanedAt *metav1.Time `json:"orphanedAt,omitempty"`
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

func (s *AuditLogStatus) WithCondition(conditionType string, status metav1.ConditionStatus, reason, message string, objGeneration int64) *AuditLogStatus {
	if s.Conditions == nil {
		s.Conditions = make([]metav1.Condition, 0, 1)
	}

	condition := meta.FindStatusCondition(s.Conditions, conditionType)

	if condition == nil {
		condition = &metav1.Condition{
			Type:    conditionType,
			Reason:  reason,
			Message: message,
		}
	} else {
		condition.Reason = reason
		condition.Message = message
	}

	condition.Status = status
	condition.ObservedGeneration = objGeneration
	meta.SetStatusCondition(&s.Conditions, *condition)
	return s
}

// Deprecated: Use WithCondition instead
func (s *AuditLogStatus) WithInstallConditionStatus(status metav1.ConditionStatus, objGeneration int64) *AuditLogStatus {
	return s.WithCondition(ConditionTypeReady, status, ConditionReasonProvisioning, "Provisioning audit log resources", objGeneration)
}
