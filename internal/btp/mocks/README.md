# BTPClient Mock

This directory contains the auto-generated mock for the `BTPClient` interface, created using [mockery](https://github.com/vektra/mockery).

## Mockery Version

- **Current version**: v2.53.4+
- **Compatibility**: Config is forward-compatible with mockery v3
- The config includes v3 deprecation fixes (`resolve-type-alias: false`, `disable-version-string: true`, `issue-845-fix: true`)

## Regenerating the Mock

**IMPORTANT**: Always run mockery from the **project root directory** to avoid creating duplicate mocks in incorrect locations.

There are **two ways** to regenerate mocks:

### 1. Using `make mocks` (Recommended)
```bash
# From project root:
make mocks
```

### 2. Using `mockery` directly
```bash
# From project root:
mockery
```

Both commands use the centralized `.mockery.yaml` configuration file.

## Configuration

All mock settings are defined in `.mockery.yaml` at the project root:
- Output directory: `internal/btp/mocks/`
- Package name: `mocks`
- Expecter pattern enabled
- v3 compatibility settings

**Note**: We don't use `//go:generate` annotations because they can cause path issues if run from subdirectories. The centralized config file approach is more reliable.

## Usage

### Basic Example

```go
import (
    "testing"
    "context"
    "github.com/kyma-project/auditlog-manager/internal/btp/mocks"
    "github.com/stretchr/testify/mock"
)

func TestMyFunction(t *testing.T) {
    // Create mock
    mockBTPClient := mocks.NewBTPClient(t)

    // Setup expectations using the expecter pattern
    mockBTPClient.EXPECT().
        CreateSubaccount(mock.Anything, "us-east-1", "test-name").
        Return("subaccount-guid", nil).
        Once()

    // Use the mock in your code
    guid, err := mockBTPClient.CreateSubaccount(context.Background(), "us-east-1", "test-name")

    // Assertions
    assert.NoError(t, err)
    assert.Equal(t, "subaccount-guid", guid)

    // mockery automatically verifies all expectations were met
}
```

### Advanced Patterns

See `internal/controller/fsm/fsm_subaccount_example_test.go` for comprehensive examples including:

- Basic mock setup with `.EXPECT()` pattern
- Using `RunAndReturn` for custom logic
- Using argument matchers with `mock.MatchedBy()`
- Testing multiple scenarios (ready, creating, deleted)
- Testing unrecoverable error conditions

### Available Methods

All `BTPClient` interface methods are mocked:

- `CreateLoggingStack(ctx, tenantID) error`
- `DeleteLoggingStack(ctx, tenantID) error`
- `VerifyLoggingStack(ctx, tenantID) (InstallStatus, error)`
- `CreateSubaccount(ctx, region, displayName) (string, error)`
- `GetSubaccount(ctx, subaccountGUID) (exists bool, state string, err error)`
- `DeleteSubaccount(ctx, subaccountGUID) error`

## Mock Features

The generated mock includes:

- **Expecter pattern**: Type-safe method expectations with `.EXPECT()`
- **Argument matchers**: Use `mock.Anything`, `mock.MatchedBy()`, etc.
- **Return values**: Set return values with `.Return()`
- **Custom behavior**: Implement custom logic with `.RunAndReturn()`
- **Call counts**: Verify call counts with `.Once()`, `.Times(n)`, `.Maybe()`
- **Automatic verification**: Mockery automatically verifies all expectations were met when test ends

## Documentation

- [Mockery documentation](https://vektra.github.io/mockery/)
- [Testify mock documentation](https://pkg.go.dev/github.com/stretchr/testify/mock)
