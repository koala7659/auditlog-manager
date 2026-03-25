# Auditlog Manager - Claude Development Guide

## Project Overview

**Auditlog Manager** is a Kubernetes operator built with Kubebuilder v4.13.0. This project is part of the Kyma ecosystem and manages audit logging functionality for Kubernetes clusters.

- **Domain:** `kyma-project.io`
- **Framework:** Kubebuilder v4 (controller-runtime v0.23.1)
- **Language:** Go 1.25.3
- **Layout:** Single-group (default Kubebuilder layout)
- **Repository:** github.com/kyma-project/auditlog-manager.git

## Project Status

The Auditlog Manager is an active Kubernetes operator project with the core API defined and ready for implementation.

## Architecture & Design

### Current State
- Manager entry point configured (`cmd/main.go`)
- Kubernetes controller-runtime manager set up with:
  - Metrics endpoint (secure HTTPS by default on :8443)
  - Health probes (/healthz, /readyz on :8081)
  - Leader election support
  - TLS certificate management for metrics
  - **No webhook server** - webhooks removed for simplified architecture
- AuditLog Custom Resource Definition (CRD) created
- Controller reconciliation scaffold in place
- E2E test infrastructure ready (uses Kind clusters)

### Key Components
- **cmd/main.go** - Manager initialization with security-first defaults (HTTP/2 disabled, secure metrics, restricted pod security)
- **api/v1beta1/auditlog_types.go** - AuditLog CRD definition
- **internal/controller/auditlog_controller.go** - Controller reconciliation logic
- **config/** - Kustomize manifests for deployment, RBAC, monitoring
- **test/e2e/** - End-to-end test suite using Ginkgo/Gomega
- **test/utils/** - Helper functions for testing (cert-manager, Kind cluster operations)

## Development Workflow

### Essential Commands

```bash
# Generate manifests and code
make manifests          # Regenerate CRDs/RBAC from kubebuilder markers
make generate           # Regenerate DeepCopy methods

# Code quality
make fmt               # Format code with gofmt
make vet               # Run go vet
make lint              # Run golangci-lint
make lint-fix          # Auto-fix linting issues

# Testing
make test              # Run unit tests (uses envtest)
make test-e2e          # Run e2e tests in isolated Kind cluster

# Build and deploy
make build             # Build manager binary to bin/manager
make run               # Run locally against current kubeconfig
make docker-build      # Build container image
make docker-push       # Push container image
make deploy            # Deploy to K8s cluster
make undeploy          # Remove from K8s cluster
```

### Creating New Resources

**ALWAYS use Kubebuilder CLI to scaffold new APIs:**

```bash
# Create a new API and controller
kubebuilder create api --group <group> --version <version> --kind <Kind>
```

**CRITICAL: Group Name Pattern**
- The `--group` parameter should be **only the group name**, NOT the full domain
- Kubebuilder automatically appends the domain from the PROJECT file
- ✅ Correct: `--group auditlogmanager` (results in `auditlogmanager.kyma-project.io`)
- ❌ Wrong: `--group auditlogmanager.kyma-project.io` (results in `auditlogmanager.kyma-project.io.kyma-project.io`)

**Example:**
```bash
# Correct - group name only
kubebuilder create api --group auditlogmanager --version v1beta1 --kind AuditLog

# Wrong - includes domain (will duplicate it)
kubebuilder create api --group auditlogmanager.kyma-project.io --version v1beta1 --kind AuditLog
```

**Adding Webhooks (Optional):**
If you need validation or defaulting webhooks, create them with:
```bash
kubebuilder create webhook --group auditlogmanager --version v1beta1 --kind AuditLog \
  --defaulting --programmatic-validation
```

**Note:** This project currently does not use webhooks. The webhook server was removed for a simpler controller-only architecture.

**NEVER manually create files in `api/` or `internal/controller/` directories.**

### After Any Changes

1. **After editing `*_types.go` or markers:**
   ```bash
   make manifests generate
   ```

2. **Before committing:**
   ```bash
   make lint-fix
   make test
   ```

## Code Style & Conventions

### Logging Style

Follow Kubernetes logging message style guidelines:
- Start from a capital letter, no ending period
- Use active voice (present for ongoing, past for completed actions)
- Specify object type in messages
- Use structured logging with key-value pairs

```go
log.Info("Starting reconciliation")
log.Info("Created Deployment", "name", deploy.Name, "namespace", deploy.Namespace)
log.Error(err, "Failed to create Pod", "name", name)
```

**Reference:** https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md

### Controller Patterns

- **Idempotent reconciliation** - Safe to run multiple times
- **Re-fetch before updates** - `r.Get()` before `r.Update()` to avoid conflicts
- **Use owner references** - Enable automatic garbage collection
- **Implement finalizers** - Clean up external resources properly
- **Status conditions** - Use `metav1.Condition` (not custom string fields)

### RBAC Markers

Add RBAC markers to controller files:

```go
// +kubebuilder:rbac:groups=mygroup.kyma-project.io,resources=mykinds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mygroup.kyma-project.io,resources=mykinds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mygroup.kyma-project.io,resources=mykinds/finalizers,verbs=update
```

## Testing Strategy

### Unit Tests
- Use **envtest** - Real Kubernetes API server + etcd
- Framework: **Ginkgo + Gomega** (BDD style)
- Run with: `make test`
- Coverage tracked via Coveralls

### E2E Tests
- **Must run against isolated Kind cluster** (see `KIND_CLUSTER` variable)
- Automatically builds and loads Docker image
- Installs CertManager if needed (skip with `CERT_MANAGER_INSTALL_SKIP=true`)
- Validates metrics endpoint, pod status, controller logs
- Run with: `make test-e2e`
- Cluster name: `auditlog-manager-test-e2e`

### Test Tags
- E2E tests use build tag: `//go:build e2e`
- Unit tests run without tags

## Security Considerations

### Pod Security Standards
- Default: **Restricted** Pod Security Standards
- Non-root user (UID 65532)
- Read-only root filesystem
- Drop all capabilities
- No privilege escalation
- Seccomp profile: RuntimeDefault

### HTTP/2 Security
- HTTP/2 **disabled by default** to prevent CVEs (GHSA-qppj-fm5r-hxr3, GHSA-4374-p667-p6c8)
- Enable only if needed with `--enable-http2=true`
- Applies to metrics server only (no webhook server in this project)

### Metrics Security
- Secure by default (HTTPS with authn/authz)
- FilterProvider enforces RBAC on metrics endpoint
- Certificate management via cert-manager or self-signed

### No Webhooks
- This project uses a **controller-only architecture**
- Webhook server has been removed for simplicity
- All validation/mutation should be handled in the controller's Reconcile loop
- If webhooks are needed in future, they can be added via `kubebuilder create webhook`

## CI/CD Pipeline

### GitHub Actions Workflows
- **test.yml** - Unit tests on push/PR
- **lint.yml** - Code linting (golangci-lint)
- **test-e2e.yml** - E2E tests with Kind
- **pull-gitleaks.yml** - Secret scanning
- **lint-markdown-links.yml** - Documentation link validation
- **stale.yml** - Stale issue management

### Quality Gates
- All tests must pass
- Linting must pass (`make lint-config && make lint`)
- Code coverage tracked (Coveralls badge)
- Go Report Card integration

## Linting Configuration

### golangci-lint v2.8.0

Enabled linters:
- copyloopvar, dupl, errcheck, ginkgolinter
- goconst, gocyclo, govet, ineffassign
- lll, modernize, misspell, nakedret
- prealloc, revive, staticcheck
- unconvert, unparam, unused
- **logcheck** - Custom module for Kubernetes logging conventions

Formatters: gofmt, goimports

### Custom golangci-lint Plugin
- Config: `.custom-gcl.yml`
- Includes custom `logcheck` plugin for K8s logging validation
- Auto-builds custom binary if `.custom-gcl.yml` exists

## File Structure Rules

### DO NOT EDIT (Auto-Generated)
- `config/crd/bases/*.yaml` - Generated by `make manifests`
- `config/rbac/role.yaml` - Generated by `make manifests`
- `**/zz_generated.*.go` - Generated by `make generate`
- `PROJECT` - Managed by Kubebuilder CLI

### DO NOT DELETE
- `// +kubebuilder:scaffold:*` comments - Used by CLI for code injection

### Standard Locations
- API types: `api/<version>/*_types.go`
- Controllers: `internal/controller/*_controller.go`
- Webhooks: `internal/webhook/<version>/*_webhook.go`
- Test suites: `internal/controller/suite_test.go`
- Sample CRs: `config/samples/`

## Deployment

### Local Development
```bash
make install  # Install CRDs
make run      # Run manager locally
```

### Kubernetes Cluster
```bash
export IMG=<registry>/auditlog-manager:tag
make docker-build docker-push IMG=$IMG
make deploy IMG=$IMG
```

### Kind Cluster (for testing)
```bash
export IMG=auditlog-manager:dev
make docker-build IMG=$IMG
kind load docker-image $IMG --name <cluster>
make deploy IMG=$IMG
```

## Documentation Structure

### docs/user/
End-user documentation displayed on Kyma website. Use numbering scheme:
- `00-xx-overview`
- `01-xx-tutorial/configuration`
- `02-xx-usage`
- `03-xx-troubleshooting`

Create `_sidebar.md` to list documents for website rendering.

### docs/contributor/
Developer documentation for manual installation and operation.

### docs/operator/
Operator documentation (required for restricted markets only).

## Dependencies

### Core Dependencies
- **controller-runtime v0.23.1** - Kubernetes operator framework
- **k8s.io/client-go v0.35.0** - Kubernetes client
- **k8s.io/apimachinery v0.35.0** - Kubernetes API machinery

### Testing
- **Ginkgo v2.27.2** - BDD test framework
- **Gomega v1.38.2** - Matcher/assertion library

### Tools (installed to bin/)
- **kustomize v5.8.1** - Manifest customization
- **controller-gen v0.20.1** - Code/manifest generation
- **setup-envtest** - K8s API server for testing
- **golangci-lint v2.8.0** - Linting

## Common Tasks

### Adding a New API

1. **Scaffold the API (use group name only, not full domain):**
   ```bash
   # Correct: group name only
   kubebuilder create api --group auditlogmanager --version v1alpha1 --kind AuditLog

   # This creates the full group: auditlogmanager.kyma-project.io
   # (domain is automatically appended from PROJECT file)
   ```

2. **Edit the types** (`api/v1alpha1/auditlog_types.go`):
   - Define Spec and Status fields
   - Add kubebuilder validation markers
   - Add printcolumn markers for kubectl output

3. **Regenerate:**
   ```bash
   make manifests generate
   ```

4. **Implement controller logic** (`internal/controller/auditlog_controller.go`):
   - Add RBAC markers
   - Implement Reconcile() method
   - Add event recording, finalizers as needed

5. **Test:**
   ```bash
   make test
   make test-e2e
   ```

### Adding Webhooks

```bash
kubebuilder create webhook --group auditlog --version v1alpha1 --kind AuditLog \
  --defaulting --programmatic-validation
```

Then implement validation/defaulting logic in generated webhook file.

### Watching External Types

To watch resources from other operators (cert-manager, Istio, etc.):

```bash
# Example: Watch cert-manager Certificate resources
# Note: --group should be the external group name only (e.g., "cert-manager")
# NOT the full domain like "cert-manager.io"
kubebuilder create api \
  --group cert-manager --version v1 --kind Certificate \
  --controller=true --resource=false \
  --external-api-path=github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1 \
  --external-api-domain=io \
  --external-api-module=github.com/cert-manager/cert-manager
```

**Important:** The `--external-api-domain` parameter specifies the external resource's domain (e.g., `io` for cert-manager.io), which is different from your project's domain in the PROJECT file.

## Git & Release Workflow

### Commit Style
Follow repository commit message patterns (check `git log` for style).

### Never
- Push to `main` directly without PR
- Skip hooks with `--no-verify`
- Use `--force` push to main/master
- Commit without running tests first

### Security
- Gitleaks scanning enabled (`.gitleaks.toml`)
- REUSE compliance required
- Never commit secrets or credentials

## Kyma-Specific Guidelines

### Branding
- Project is part of Kyma ecosystem
- Follow Kyma community guidelines
- Namespace convention: `<project>-system` (e.g., `auditlog-manager-system`)

### Documentation
- User docs published to https://kyma-project.io
- Follow [Kyma content guidelines](https://github.com/kyma-project/community/blob/main/docs/guidelines/content-guidelines/01-user-docs.md)

### Community
- Contributing rules: https://github.com/kyma-project/community/blob/main/docs/contributing/02-contributing.md
- Code of Conduct: See CODE_OF_CONDUCT.md

## Important Notes

### Current Project State
- **API Created:** AuditLog (group: `auditlogmanager.kyma-project.io`, version: `v1beta1`)
- **Controller Generated:** Basic reconciliation scaffold in place at `internal/controller/auditlog_controller.go`
- **CRD Generated:** `auditlogs.auditlogmanager.kyma-project.io`
- **Tests:** Controller test suite scaffolded with 66.7% initial coverage
- **Architecture:** Controller-only (no webhooks)

### Implemented Resources
- **AuditLog (v1beta1):** Custom Resource for audit logging management
  - Location: `api/v1beta1/auditlog_types.go`
  - Controller: `internal/controller/auditlog_controller.go`
  - CRD: `config/crd/bases/auditlogmanager.kyma-project.io_auditlogs.yaml`
  - Sample: `config/samples/auditlogmanager_v1beta1_auditlog.yaml`

### Next Steps for Development
1. Define Spec and Status fields in `api/v1beta1/auditlog_types.go`
2. Implement controller reconciliation logic in `internal/controller/auditlog_controller.go`
3. Add any required RBAC permissions via kubebuilder markers
4. Create realistic sample manifests in `config/samples/`
5. Write comprehensive unit and e2e tests
6. Update README.md with actual project description and usage examples
7. Consider if webhooks are needed (validation/defaulting) - if so, add them later

### Best Practices
- Always use `make manifests generate` after changing types or markers
- Run `make lint-fix` before committing
- Keep reconciliation logic idempotent
- Use structured logging with proper key-value pairs
- Follow Kubernetes API conventions
- Test against isolated Kind clusters for e2e tests
- Never edit auto-generated files
- All validation/mutation logic should be in the controller (no webhooks currently)

## Troubleshooting

### Common Kubebuilder Mistakes

**Wrong Group Name Pattern:**
```bash
# ❌ WRONG - includes domain
kubebuilder create api --group mygroup.kyma-project.io --version v1 --kind MyKind
# Results in: mygroup.kyma-project.io.kyma-project.io (duplicate domain)

# ✅ CORRECT - group name only
kubebuilder create api --group mygroup --version v1 --kind MyKind
# Results in: mygroup.kyma-project.io (domain auto-appended from PROJECT file)
```

If you accidentally created an API with the wrong group name:
1. Delete the API directory: `rm -rf api/<version>`
2. Delete controller files: `rm internal/controller/*_controller.go internal/controller/*_controller_test.go`
3. Delete generated CRDs: `rm config/crd/bases/*`
4. Edit PROJECT file to remove the incorrect resource entry
5. Recreate with correct group name: `kubebuilder create api --group <correct-group> ...`
6. Regenerate: `make manifests generate`

### Build Issues
```bash
go mod tidy              # Fix dependency issues
make manifests generate  # Regenerate manifests if out of sync
```

### Test Issues
```bash
make setup-envtest      # Ensure envtest binaries are installed
kubectl cluster-info    # Verify cluster connectivity for e2e tests
```

### Deployment Issues
```bash
kubectl logs -n auditlog-manager-system deployment/auditlog-manager-controller-manager -c manager -f
kubectl get events -n auditlog-manager-system --sort-by=.lastTimestamp
```

## References

### Essential Resources
- **Kubebuilder Book:** https://book.kubebuilder.io
- **controller-runtime FAQ:** https://github.com/kubernetes-sigs/controller-runtime/blob/main/FAQ.md
- **Kubernetes API Conventions:** https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- **Kyma Community:** https://github.com/kyma-project/community
- **AGENTS.md** - Detailed Kubebuilder CLI patterns and examples

### Internal Documentation
- See `AGENTS.md` for comprehensive Kubebuilder patterns
- See `docs/README.md` for documentation structure
- See `.golangci.yml` for linting rules
