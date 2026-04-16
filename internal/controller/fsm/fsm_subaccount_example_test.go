package fsm

import (
	"context"
	"testing"

	auditlogmanagerv1beta1 "github.com/kyma-project/auditlog-manager/api/v1beta1"
	"github.com/kyma-project/auditlog-manager/internal/btp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Example test demonstrating how to use the BTPClient mock
// NOTE: This is a simplified example showing BTP client mocking only.
// A complete test would also need to mock the Kubernetes client to avoid nil pointer errors.
func TestSFnCreateSubaccount_NewSubaccount(t *testing.T) {
	t.Skip("Skipping: requires Kubernetes client mock to be implemented")

	// Given
	ctx := context.Background()
	mockBTPClient := mocks.NewBTPClient(t)

	// Setup mock expectations using the expecter pattern
	mockBTPClient.EXPECT().
		CreateSubaccount(
			mock.Anything,
			"us-east-1",
			"global-account-guid-123",
			"auditlog-test-instance",
			[]string{"admin@example.com"},
		).
		Return("subaccount-guid-123", nil).
		Once()

	// Create FSM with mock
	f := &fsm{
		BTPClient: mockBTPClient,
		K8s: K8s{
			KcpClient: nil, // TODO: Need to mock KcpClient.Status().Update() and KcpClient.Update()
		},
	}

	// Create system state
	s := &systemState{
		instance: auditlogmanagerv1beta1.AuditLog{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-instance",
				Generation: 1,
			},
			Spec: auditlogmanagerv1beta1.AuditLogSpec{
				Region:          "us-east-1",
				GlobalAccountID: "global-account-guid-123",
				Administrators:  []string{"admin@example.com"},
			},
		},
	}

	// When
	nextState, result, err := sFnCreateSubaccount(ctx, f, s)

	// Then
	assert.NoError(t, err)
	assert.Nil(t, result) // stop() returns nil result
	assert.Nil(t, nextState)

	// Verify SubaccountID was set in spec
	assert.Equal(t, "subaccount-guid-123", s.instance.Spec.SubaccountID)

	// Mockery automatically verifies all expectations were met
}

func TestSFnVerifySubaccount_SubaccountReady(t *testing.T) {
	// Given
	ctx := context.Background()
	mockBTPClient := mocks.NewBTPClient(t)

	subaccountID := "existing-subaccount-guid"

	// Setup mock expectations
	mockBTPClient.EXPECT().
		GetSubaccount(mock.Anything, subaccountID).
		Return(true, "OK", nil). // exists=true, state=OK, no error
		Once()

	// Create FSM with mock
	f := &fsm{
		BTPClient: mockBTPClient,
		K8s: K8s{
			KcpClient: nil,
		},
	}

	// Create system state with existing SubaccountID
	s := &systemState{
		instance: auditlogmanagerv1beta1.AuditLog{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-instance",
				Generation: 1,
			},
			Spec: auditlogmanagerv1beta1.AuditLogSpec{
				Region:          "us-east-1",
				GlobalAccountID: "global-account-guid",
				Administrators:  []string{"admin@example.com"},
				SubaccountID:    subaccountID,
			},
			Status: auditlogmanagerv1beta1.AuditLogStatus{
				Conditions: []metav1.Condition{},
			},
		},
		snapshot: auditlogmanagerv1beta1.AuditLogStatus{},
	}

	// When
	nextState, result, err := sFnVerifySubaccount(ctx, f, s)

	// Then
	assert.NoError(t, err)
	assert.Nil(t, result)       // Result is nil (returned to FSM loop)
	assert.NotNil(t, nextState) // Returns sFnUpdateStatus

	// Verify condition was set
	assert.Len(t, s.instance.Status.Conditions, 1)
	assert.Equal(t, string(auditlogmanagerv1beta1.ConditionTypeSubaccountReady), s.instance.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, s.instance.Status.Conditions[0].Status)

	// Mockery automatically verifies all expectations were met
}

func TestSFnVerifySubaccount_SubaccountDeleted_Unrecoverable(t *testing.T) {
	// Given
	ctx := context.Background()
	mockBTPClient := mocks.NewBTPClient(t)

	subaccountID := "deleted-subaccount-guid"

	// Setup mock expectations - subaccount doesn't exist
	mockBTPClient.EXPECT().
		GetSubaccount(mock.Anything, subaccountID).
		Return(false, "", nil). // exists=false, no state, no error
		Once()

	// Create FSM with mock
	f := &fsm{
		BTPClient: mockBTPClient,
		K8s: K8s{
			KcpClient: nil,
		},
	}

	// Create system state
	s := &systemState{
		instance: auditlogmanagerv1beta1.AuditLog{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-instance",
				Generation: 1,
			},
			Spec: auditlogmanagerv1beta1.AuditLogSpec{
				Region:          "us-east-1",
				GlobalAccountID: "global-account-guid",
				Administrators:  []string{"admin@example.com"},
				SubaccountID:    subaccountID,
			},
			Status: auditlogmanagerv1beta1.AuditLogStatus{
				Conditions: []metav1.Condition{},
			},
		},
		snapshot: auditlogmanagerv1beta1.AuditLogStatus{},
	}

	// When
	nextState, result, err := sFnVerifySubaccount(ctx, f, s)

	// Then
	assert.NoError(t, err)
	assert.Nil(t, result)       // Result is nil (returned to FSM loop)
	assert.NotNil(t, nextState) // Returns sFnUpdateStatus

	// Verify condition shows unrecoverable error
	assert.Len(t, s.instance.Status.Conditions, 1)
	assert.Equal(t, string(auditlogmanagerv1beta1.ConditionTypeSubaccountReady), s.instance.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionFalse, s.instance.Status.Conditions[0].Status)
	assert.Equal(t, "SubaccountLost", s.instance.Status.Conditions[0].Reason)
	assert.Contains(t, s.instance.Status.Conditions[0].Message, "deleted externally")
	assert.Contains(t, s.instance.Status.Conditions[0].Message, "permanently lost")

	// SubaccountID should still be in spec for audit trail
	assert.Equal(t, subaccountID, s.instance.Spec.SubaccountID)

	// Mockery automatically verifies all expectations were met
}

// Example showing how to use RunAndReturn for complex behavior
func TestBTPClient_Mock_WithRunAndReturn(t *testing.T) {
	// Given
	mockBTPClient := mocks.NewBTPClient(t)

	// Setup mock with custom logic using RunAndReturn
	mockBTPClient.EXPECT().
		GetSubaccount(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, guid string) (bool, string, error) {
			// Custom logic based on input
			if guid == "valid-guid" {
				return true, "OK", nil
			}
			return false, "", nil
		}).
		Times(2) // Expect this to be called twice

	// When
	exists1, state1, err1 := mockBTPClient.GetSubaccount(context.Background(), "valid-guid")
	exists2, state2, err2 := mockBTPClient.GetSubaccount(context.Background(), "invalid-guid")

	// Then
	assert.NoError(t, err1)
	assert.True(t, exists1)
	assert.Equal(t, "OK", state1)

	assert.NoError(t, err2)
	assert.False(t, exists2)
	assert.Empty(t, state2)
}

// Example showing how to verify method calls with specific matchers
func TestBTPClient_Mock_WithMatchers(t *testing.T) {
	// Given
	mockBTPClient := mocks.NewBTPClient(t)

	// Setup mock with specific argument matchers
	mockBTPClient.EXPECT().
		CreateSubaccount(
			mock.Anything, // any context
			mock.MatchedBy(func(region string) bool { // custom matcher for region
				return region == "us-east-1" || region == "eu-west-1"
			}),
			mock.Anything, // globalAccountID
			mock.MatchedBy(func(name string) bool { // custom matcher for display name
				return len(name) > 0 && len(name) <= 100
			}),
			mock.Anything, // administrators
		).
		Return("mock-guid", nil).
		Once()

	// When
	guid, err := mockBTPClient.CreateSubaccount(
		context.Background(),
		"us-east-1",
		"global-account-123",
		"test-name",
		[]string{"admin@test.com"},
	)

	// Then
	assert.NoError(t, err)
	assert.Equal(t, "mock-guid", guid)
}
