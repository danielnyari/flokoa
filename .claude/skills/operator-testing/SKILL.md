---
name: operator-testing
description: Run and manage tests for the Flokoa Kubernetes Operator. Use when the user asks to run tests, add tests, fix test failures, or verify operator changes. Covers unit tests (Ginkgo/envtest), e2e tests (Kind cluster), and linting.
---

# Operator Testing

## Prerequisites

All commands must be run from the `operator/` directory.

Before running tests, ensure code generation is up to date:

```bash
make manifests generate
```

## Unit Tests

Run unit tests with envtest (embedded Kubernetes API server):

```bash
make test
```

This target automatically runs `manifests`, `generate`, `fmt`, and `vet` before executing tests.

Under the hood it runs:
```bash
KUBEBUILDER_ASSETS="$(setup-envtest use <version> -p path)" go test $(go list ./... | grep -v /e2e) -coverprofile cover.out
```

### Writing Unit Tests

Tests use **Ginkgo v2 + Gomega** (BDD-style). Place test files alongside the code they test.

Controller test pattern:
```go
var _ = Describe("Agent Controller", func() {
    Context("When reconciling a resource", func() {
        const resourceName = "test-resource"

        ctx := context.Background()
        typeNamespacedName := types.NamespacedName{
            Name:      resourceName,
            Namespace: "default",
        }

        BeforeEach(func() {
            // Create test resources
        })

        AfterEach(func() {
            // Clean up test resources
        })

        It("should successfully reconcile the resource", func() {
            controllerReconciler := &AgentReconciler{
                Client: k8sClient,
                Scheme: k8sClient.Scheme(),
            }

            _, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
                NamespacedName: typeNamespacedName,
            })
            Expect(err).NotTo(HaveOccurred())
        })
    })
})
```

### Envtest Setup

If envtest binaries are missing:
```bash
make setup-envtest
```

## E2E Tests

Run end-to-end tests in an isolated Kind cluster:

```bash
make test-e2e
```

This creates a Kind cluster named `operator-test-e2e`, runs tests, then cleans up.

### E2E Test Management

```bash
make setup-test-e2e     # Create Kind cluster only
make cleanup-test-e2e   # Delete Kind cluster
make deploy-e2e-testdata  # Deploy test fixtures (requires OPENAI_API_KEY env var)
```

To skip CertManager installation:
```bash
CERT_MANAGER_INSTALL_SKIP=true make test-e2e
```

E2E test files are in `test/e2e/`. Test fixtures (sample CRs, Argo workflows) are in `test/e2e/testdata/`.

## Linting

```bash
make lint          # Run golangci-lint
make lint-fix      # Auto-fix lint issues
make lint-config   # Verify lint configuration
```

Key linters: errcheck, govet, staticcheck, ginkgolinter, revive, gocyclo, misspell.

## Troubleshooting

### "envtest binaries not found"
Run `make setup-envtest` to download the required Kubernetes API server binaries.

### "CRD not registered" in tests
Ensure `make manifests generate` has been run after any type changes.

### Tests fail with "context deadline exceeded"
Increase test timeout or check if the reconciler has an infinite loop.

### Lint errors in generated files
Generated files (`zz_generated.*.go`) are excluded from linting. If you see errors in them, regenerate with `make generate`.
