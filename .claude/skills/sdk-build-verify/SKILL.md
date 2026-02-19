---
name: sdk-build-verify
description: Verify the Flokoa Python SDK builds successfully. Use when the user asks to verify the SDK build, check Python code quality, run SDK tests, or validate the wheel builds. Covers dependency installation, linting, type checking, tests, and wheel packaging.
---

# SDK Build Verification

Commands run from the `sdk/python/` directory unless otherwise noted.

## Quick Verification

Run checks and tests for the entire workspace:

```bash
make check
make test
```

## Step-by-Step Verification

### 1. Install Dependencies

Install all workspace packages and pre-commit hooks using uv:

```bash
make install
```

This runs `uv sync --all-packages` and installs pre-commit hooks.

### 2. Lock File Consistency

Verify `uv.lock` is consistent with `pyproject.toml`:

```bash
uv lock --locked
```

This is included in `make check`.

### 3. Linting and Formatting

Run all pre-commit hooks (ruff, formatting, etc.):

```bash
make check
```

Run linting specifically for the flokoa package:

```bash
make check-flokoa
```

This runs:
- `ruff check flokoa/src flokoa/tests` - Linting
- `ty check` (from `flokoa/`) - Static type checking

### 4. Tests

Run all workspace tests:

```bash
make test
```

Run tests for individual packages:

```bash
make test-flokoa          # flokoa SDK tests with coverage
make test-managed-agent   # flokoa-managed-agent tests
```

Under the hood, `test-flokoa` runs:
```bash
cd flokoa && uv run pytest tests --cov --cov-config=pyproject.toml --cov-report=xml
```

### 5. Build Wheel

Build the flokoa SDK wheel:

```bash
make build-flokoa
```

This runs `uvx --from build pyproject-build --installer uv` in the `flokoa/` directory.

Clean build artifacts:

```bash
make clean
```

### 6. Docker Image Build (CLI)

Build the Flokoa CLI Docker image (from `operator/` directory):

```bash
make docker-build-flokoa-cli
```

This builds `ghcr.io/danielnyari/flokoa-cli:VERSION` from `sdk/python/flokoa-managed-agent/Dockerfile`.

## Full Build Verification Sequence

To verify everything compiles and passes checks:

```bash
make install
make check
make check-flokoa
make test
make build-flokoa
```

## Package-Level Verification

For the `flokoa` package specifically (from `sdk/python/flokoa/`):

```bash
make check    # Lock file + pre-commit + type checking
make test     # pytest with coverage
make build    # Build wheel (cleans first)
```

## Python Types Verification

If CRD schemas have changed, regenerate Python types (from `operator/` directory):

```bash
make manifests generate generate-python-models
```

Then verify the SDK still builds:

```bash
cd sdk/python && make check && make test
```

## Workspace Structure

The SDK is a uv workspace with multiple packages:

| Package | Path | Description |
|---------|------|-------------|
| `flokoa` | `sdk/python/flokoa/` | Public SDK package and CLI |
| `flokoa-types` | `sdk/python/flokoa-types/` | Auto-generated Pydantic models from CRDs |
| `flokoa-managed-agent` | `sdk/python/flokoa-managed-agent/` | Operator-deployed pydantic-ai agent runtime |
| `flokoa-managed-task` | `sdk/python/flokoa-managed-task/` | Operator-deployed Marvin task runtime (scaffold) |

## Troubleshooting

### "uv: command not found"
Install uv: `curl -LsSf https://astral.sh/uv/install.sh | sh`

### Lock file out of sync
Run `uv lock` to regenerate the lock file, then `make check`.

### Type checking errors with ty
Run `cd flokoa && uv run ty check` for detailed output.

### Pre-commit hook failures
Run `uv run pre-commit run -a` to see all hook results and fix issues.
