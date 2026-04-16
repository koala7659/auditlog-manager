# Granular BTP Provisioning - Architecture Design

## Overview

The audit log controller now uses a **granular, step-by-step provisioning approach** where each BTP operation is handled separately with proper verification and retry logic.

## Key Principles

### 1. **BTP API is the Source of Truth**
- Never rely solely on Kubernetes conditions to determine state
- Always verify with BTP API before proceeding
- Conditions are for observability, not state management

### 2. **Idempotent Operations**
- Every FSM state can be retried safely
- Each state checks if work is already done before acting
- Handles externally deleted resources (drift)

### 3. **Resume from Failure Point**
- Store operational state in `spec.SubaccountID`, `spec.Config`, etc.
- If step 3 fails, retry from step 3 (not step 1)
- No wasted work on retry

### 4. **Async Operation Handling**
- BTP operations are asynchronous (create → poll → ready)
- Each state either completes work or requeues to poll status
- Proper timeout and error handling

## FSM State Flow (Creation)

```
┌──────────────────────────┐
│ sFnRun                   │ Entry point
│ (checks finalizer, etc)  │
└────────────┬─────────────┘
             │ !resourcesArePresent
             v
┌──────────────────────────┐
│ sFnCreateSubaccount      │ ← YOU ARE HERE
│                          │
│ Logic:                   │
│ 1. If SubaccountID in    │
│    spec → verify with    │
│    GetSubaccount()       │
│ 2. If state=OK → next    │
│ 3. If state=CREATING →   │
│    requeue & poll        │
│ 4. If not exists →       │
│    CreateSubaccount()    │
│ 5. Store GUID in spec    │
│ 6. Requeue to verify     │
└────────────┬─────────────┘
             │ Condition: SubaccountReady=True
             v
┌──────────────────────────┐
│ sFnCreateServiceManager  │ TODO: Next to implement
│       Binding            │
│                          │
│ Logic:                   │
│ 1. Check if SM binding   │
│    exists (API call)     │
│ 2. If exists → next      │
│ 3. Create binding        │
│ 4. Requeue to verify     │
└────────────┬─────────────┘
             │ Condition: BTPResourcesProvisioned=True
             v
┌──────────────────────────┐
│ sFnCreateServiceInstances│ TODO
│                          │
│ Logic:                   │
│ 1. Create auditlog-mgmt  │
│ 2. Poll until ready      │
│ 3. Create auditlog       │
│ 4. Poll until ready      │
└────────────┬─────────────┘
             │ Condition: ServiceInstanceReady=True
             v
┌──────────────────────────┐
│ sFnCreateBindings        │ TODO
│                          │
│ Logic:                   │
│ 1. Create bindings       │
│ 2. Extract credentials   │
└────────────┬─────────────┘
             │ Condition: BindingReady=True
             v
┌──────────────────────────┐
│ sFnStoreCredentials      │ TODO
│                          │
│ Logic:                   │
│ 1. Store in Gardener     │
│ 2. Store in CR spec      │
└────────────┬─────────────┘
             │ Condition: CredentialsStored=True
             v
┌──────────────────────────┐
│ StateRegistrationReady   │ ADR: Ready for SIM
└──────────────────────────┘
```

## Implementation Status

### ✅ Completed: Subaccount Creation and Verification

**Files:**
- `internal/controller/fsm/fsm_create_subaccount.go` - Creates new BTP subaccounts
- `internal/controller/fsm/fsm_verify_subaccount.go` - Verifies subaccount state

**Features:**
- **Create**: Creates new subaccount via BTP API, stores SubaccountID in spec
- **Verify**: Always verifies with BTP API first (never trusts condition state alone)
- Handles all subaccount states: OK, CREATING, DELETING, ERROR, SUSPENDED
- **Treats externally deleted subaccounts as UNRECOVERABLE ERROR** (audit data permanently lost)
- Updates `ConditionTypeSubaccountReady` condition
- Proper error handling and retry intervals
- **Separates spec updates from status updates to avoid reconciliation loops**
- **Uses two-phase state transition**: update status → requeue → next reconciliation proceeds to next state

**Key Implementation Details:**

1. **Separation of Concerns:**
   - `sFnCreateSubaccount`: Only responsible for creating new subaccounts
   - `sFnVerifySubaccount`: Only responsible for verifying existing subaccounts
   - If SubaccountID exists, create delegates to verify immediately

2. **Spec vs Status Updates:**
   - Spec updates (e.g., setting SubaccountID) trigger automatic reconciliation → return `stop()`
   - Status updates don't trigger reconciliation → can safely return `requeue()` or `switchState()`
   - Never update both spec and status in same reconciliation cycle

3. **State Transition Pattern:**
   - When subaccount becomes OK: Set condition to True → requeue
   - Next reconciliation: Check condition already True → proceed to next FSM state
   - This ensures status is persisted before state transition

4. **Deleted Subaccount Handling:**
   - Detected via GetSubaccount() returning `exists=false`
   - Sets condition to False with reason "SubaccountLost"
   - Stops reconciliation permanently (unrecoverable error)
   - Keeps SubaccountID in spec for audit trail
   - Reason: Cannot recreate because audit log data is permanently lost

**BTP API Methods Used:**
- `CreateSubaccount(ctx, region, displayName) (subaccountGUID, error)`
- `GetSubaccount(ctx, subaccountGUID) (exists, state, error)`

### 🗑️ Removed: Old Monolithic States

The following files were removed as part of the granular provisioning refactoring:
- ❌ `fsm_create_resouces.go` - Replaced by granular subaccount/SM binding/service instance steps
- ❌ `fsm_verify_creation.go` - Replaced by individual verification steps per resource type
- ❌ `fsm_delete_resources.go` - Will be replaced by granular deletion steps
- ❌ `fsm_verify_deletion.go` - Will be replaced by individual deletion verification steps

**Note:** The BTP client methods (`CreateLoggingStack`, `DeleteLoggingStack`, `VerifyLoggingStack`) are kept in the interface for backward compatibility and potential reuse, but are no longer called by the FSM.

### 🚧 TODO: Service Manager Binding

**File:** `internal/controller/fsm/fsm_create_sm_binding.go` (placeholder exists)

**Needs to implement:**
1. Check if Service Manager binding exists (API call)
2. If exists and credentials valid → proceed
3. If not exists → create binding
4. Store SM credentials in spec (or fetch dynamically)
5. Update `ConditionTypeBTPResources` condition

**BTP API Methods Needed:**
```go
type BTPClient interface {
    // ... existing methods ...

    // Check if Service Manager binding exists
    GetServiceManagerBinding(ctx context.Context, subaccountGUID string) (*ServiceManagerCredentials, error)

    // Create Service Manager binding
    CreateServiceManagerBinding(ctx context.Context, subaccountGUID string) (*ServiceManagerCredentials, error)
}
```

### 🚧 TODO: Service Instances Creation

**New FSM State:** `sFnCreateServiceInstances`

**Needs to implement:**
1. Authenticate with Service Manager (using stored credentials)
2. Check if `auditlog-management` instance exists
3. If not exists → create it
4. Poll `GetServiceInstanceStatus()` until ready
5. Repeat for `auditlog` instance
6. Update `ConditionTypeServiceInstanceReady` condition

**BTP API Methods Needed:**
```go
type BTPClient interface {
    // ... existing methods ...

    // Get service instance status
    GetServiceInstanceStatus(ctx, smCreds, instanceName) (status InstanceStatus, err error)

    // InstanceStatus values: "creating", "ready", "failed"
}
```

### 🚧 TODO: Service Bindings Creation

**New FSM State:** `sFnCreateBindings`

**Needs to implement:**
1. Check if bindings exist for both instances
2. Create bindings if needed
3. Extract credentials from binding responses
4. Update `ConditionTypeBindingReady` condition

### 🚧 TODO: Credentials Storage

**New FSM State:** `sFnStoreCredentials`

**Needs to implement:**
1. Store write credentials in Gardener secret
2. Store read credentials in `spec.ReadCredentials`
3. Update `spec.Config` with serviceURL, tenantID, etc.
4. Update `ConditionTypeCredentialsStored` condition

### 🚧 TODO: Deletion Flow

**Files to create:**
- `fsm_delete_subaccount.go`
- `fsm_delete_sm_binding.go`
- `fsm_delete_service_instances.go`

**Should mirror creation flow in reverse:**
1. Delete service bindings
2. Delete service instances (poll until gone)
3. Delete Service Manager binding
4. Delete subaccount (poll until gone)

## Data Flow

### Spec (User Input + Controller State)
```go
type AuditLogSpec struct {
    Region       string              // User input
    SubaccountID string              // Controller sets after creation
    Config       AuditLogConfig      // Controller sets after provisioning
    ReadCredentials CredentialsRef   // Controller sets after binding
    RetentionDays int                // User input
}
```

### Status (Observability)
```go
type AuditLogStatus struct {
    State       State                // High-level state
    Conditions  []metav1.Condition   // Detailed progress
    // ... timestamps ...
}

// Conditions track each step:
// - SubaccountReady: True/False/Unknown
// - BTPResourcesProvisioned: True/False/Unknown (SM binding)
// - ServiceInstanceReady: True/False/Unknown
// - BindingReady: True/False/Unknown
// - CredentialsStored: True/False/Unknown
// - ResourcesReady: True when all above are True
```

## Benefits of This Approach

### 1. **Reliability**
- If step fails, retry only that step
- Detects and recovers from external changes (drift)
- Proper async operation handling (no race conditions)

### 2. **Observability**
```bash
$ kubectl get auditlog my-auditlog -o yaml

status:
  state: Pending
  conditions:
  - type: SubaccountReady
    status: "True"
    reason: Ready
    message: "Subaccount provisioned and ready"
  - type: BTPResourcesProvisioned
    status: "Unknown"
    reason: Provisioning
    message: "Creating Service Manager binding"
  - type: ServiceInstanceReady
    status: "False"
    reason: Pending
    message: "Not started yet"
```

You can see **exactly** which step is in progress and which failed!

### 3. **Debuggability**
- Each FSM state is simple and focused (single responsibility)
- Easy to add logging at each step
- Clear mapping: 1 state = 1 BTP operation = 1 condition

### 4. **Maintainability**
- Easy to add new steps (e.g., "assign entitlement")
- Easy to modify retry logic per step
- Each state is independently testable

## Next Steps

1. **Implement Service Manager Binding Step**
   - Extract existing `serviceManagerBindingExists()` logic
   - Add proper API verification
   - Update condition

2. **Implement Service Instances Step**
   - Extract from existing `CreateLoggingStack()`
   - Add polling for async creation
   - Handle both auditlog-management and auditlog

3. **Implement Bindings Step**
   - Extract credential generation
   - Verify credentials are valid

4. **Implement Credentials Storage**
   - Integrate with Gardener secrets
   - Store in CR spec

5. **Implement Deletion Flow**
   - Mirror creation in reverse
   - Proper cleanup verification

## Testing Strategy

Each FSM state can be tested independently:

```go
func TestSFnCreateSubaccount(t *testing.T) {
    tests := []struct{
        name string
        initialState systemState
        btpResponse  string
        expectedNext stateFn
    }{
        {
            name: "subaccount already exists and ready",
            initialState: systemState{
                instance: AuditLog{
                    Spec: AuditLogSpec{SubaccountID: "existing-guid"},
                },
            },
            btpResponse: "OK",
            expectedNext: sFnCreateServiceManagerBinding,
        },
        // ... more test cases
    }
}
```

## Configuration

**Retry Intervals:**
- `requeueCheckInterval = 10s` - Poll BTP status
- `requeueErrorInterval = 30s` - Retry after error
- `requeueInterval = 5s` - Normal requeue

These can be adjusted per operation if needed (e.g., subaccount creation might need longer polling interval).
