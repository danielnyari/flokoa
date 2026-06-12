---
name: operator-build-verify
description: Verify the Flokoa Kubernetes Operator builds successfully. Use when the user asks to verify the operator build, check compilation, run linting, or validate code generation. Covers manifests, code generation, Go build, protobuf generation, and linting.
---

# Operator Build Verification

All commands must be run from the `operator/` directory.

## Quick Verification

Run the full build pipeline (manifests + codegen + protobuf + fmt + vet + compile):

```bash
make build
```

This single target chains: `manifests`, `generate`, `buf-generate`, `fmt`, `vet`, then compiles `bin/manager` and `bin/server`.

## Step-by-Step Verification

### 1. Code Generation

Generate CRDs, RBAC, webhooks, and DeepCopy methods from kubebuilder markers:

```bash
make manifests generate
```

- `manifests` produces CRD YAML in `config/crd/bases/`
- `generate` produces `zz_generated.deepcopy.go` files

### 2. Protobuf Generation

Generate gRPC code from proto files (requires `buf`):

```bash
make buf-generate
```

Lint and format proto files:

```bash
make buf-lint
make buf-format
```

Check for breaking proto changes against main:

```bash
make buf-breaking
```

### 3. Go Build

Compile the manager and server binaries:

```bash
go build -o bin/manager cmd/main.go
go build -o bin/server cmd/server/main.go
```

Or use the combined target:

```bash
make build
```

### 4. Formatting and Vetting

```bash
make fmt    # go fmt ./...
make vet    # go vet ./...
```

### 5. Linting

Run golangci-lint (downloads binary if needed):

```bash
make lint
```

Auto-fix lint issues:

```bash
make lint-fix
```

Verify lint configuration:

```bash
make lint-config
```

Key linters: errcheck, govet, staticcheck, ginkgolinter, revive, gocyclo, misspell.

### 6. Unit Tests

Run unit tests with envtest (runs full codegen pipeline first):

```bash
make test
```

This chains `manifests`, `generate`, `fmt`, `vet`, `setup-envtest` before running:
```bash
KUBEBUILDER_ASSETS="$(setup-envtest use <version> -p path)" go test $(go list ./... | grep -v /e2e) -coverprofile cover.out
```

## Docker Image Build Verification

Verify Docker images build without pushing:

```bash
make docker-build
```

This builds three images:
- `ghcr.io/danielnyari/flokoa-operator:VERSION` (operator manager)
- `ghcr.io/danielnyari/flokoa-server:VERSION` (gRPC server)
- `ghcr.io/danielnyari/flokoa-a2a-plugin:VERSION` (A2A executor plugin)

Build with a custom version:

```bash
make docker-build VERSION=dev
```

## Full Build Verification Sequence

To verify everything compiles and passes checks:

```bash
make manifests generate buf-generate fmt vet
make build
make lint
make test
make docker-build
```

## Troubleshooting

### "controller-gen: command not found"
Run `make controller-gen` to download the binary to `bin/`.

### "buf: command not found"
Install buf: https://buf.build/docs/installation

### Build fails after type changes
Regenerate all code: `make manifests generate buf-generate`

### Lint failures in generated files
Generated files (`zz_generated.*.go`) are excluded from linting. If they appear, regenerate with `make generate`.
