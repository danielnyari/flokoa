# CLAUDE.md - Flokoa Python SDK

This document provides guidance for AI assistants working with the Flokoa Python SDK.

## Overview

The Flokoa Python SDK provides a CLI and library for building and running pydantic-ai agents locally. It integrates with the A2A (Agent-to-Agent) protocol. pydantic-ai is the only supported framework.

- **Package**: `flokoa`
- **Python**: >= 3.13
- **Package Manager**: uv

Operating principles for the codebase live in
[`core-beliefs.md`](../../../docs/design-docs/core-beliefs.md); the runtime contract the runner
implements is [`runtime-contract.md`](../../../docs/reference/runtime-contract.md).

## Workspace Structure

The SDK is organized as a **uv workspace**:

```
sdk/python/                          # Workspace root
├── pyproject.toml                   # Workspace definition
├── uv.lock                         # Shared lockfile for all packages
├── flokoa/                          # Public SDK (published to PyPI)
│   ├── pyproject.toml
│   ├── src/flokoa/
│   │   ├── __init__.py
│   │   ├── __main__.py             # CLI: flokoa run -m module:agent | run -f agentspec.yaml
│   │   ├── serving.py              # A2A serving (SpecAgentExecutor + build_app), shared with the runner
│   │   ├── context.py              # Agent/session accessors for capability authors
│   │   ├── telemetry.py            # OTel init + pydantic-ai/FastAPI instrumentation
│   │   └── utils/                  # Agent card builder, health router
│   └── tests/
├── flokoa-types/                    # Auto-generated Pydantic models from CRD schemas (DO NOT EDIT generated files)
│   ├── pyproject.toml
│   └── src/flokoa_types/
│       ├── __init__.py             # Re-exports
│       ├── agentcard.py            # Generated: AgentCard
│       ├── agenttool.py            # Generated: AgentToolSpec (MCP endpoint shape)
│       ├── agentworkflow.py        # Generated: AgentWorkflow
│       └── modelsettings.py        # Generated: ModelSettings
├── flokoa-runner/                  # Generic runner: bootstrap pipeline + runtime-contract artifacts
│   ├── pyproject.toml              # Owns the platform pin (pydantic-ai==X.Y.Z exactly)
│   ├── Dockerfile                  # Bakes runner-manifest.json + version labels
│   ├── runner.lock                 # Exported baseline lockfile ("the platform")
│   ├── runner-manifest.json        # Machine-readable runner identity
│   ├── hack/                       # AgentSpec schema + manifest generators
│   ├── src/flokoa_runner/
│   │   ├── __main__.py             # Pipeline: manifest → spec → secrets → capabilities → agent → serve
│   │   ├── manifest.py             # Runner identity + operator↔image skew detection
│   │   ├── specfile.py             # Loads /etc/flokoa/agent-spec.yaml
│   │   ├── secrets.py              # ${secret:NAME} resolution from FLOKOA_SECRET_* env
│   │   ├── capabilities.py         # Wheelhouse requires-check + install + entrypoint loading
│   │   ├── agent.py                # Agent.from_spec hydration
│   │   ├── serve.py                # Card loading + A2A serving
│   │   └── platform_capabilities/  # flokoa.platform/* (telemetry, …)
│   └── tests/                      # Incl. the 03/04/05 contract tests
├── flokoa-codemode-mcp/            # Code-mode MCP server package
└── flokoa-common/                  # Shared internal helpers
```

### Package Relationships

- `flokoa` — the public SDK, installable via `pip install flokoa`. Core dependencies: a2a-sdk, click, fastapi, flokoa-types, pydantic. Optional extras: `pydantic-ai`, `tracing`.
- `flokoa-types` — auto-generated Pydantic v2 models from Kubernetes CRD schemas. Shared dependency for all packages that need CRD types. Import as `flokoa_types`.
- `flokoa-runner` — internal package, never published to PyPI. Built into the generic runner image the operator deploys. Owns the runtime-contract pin: bumping pydantic-ai means `make runner-contract` (regenerates runner.lock, runner-manifest.json, and the AgentSpec schema embedded in the operator) — a PR-blocking review item.

## Tech Stack

| Component | Purpose |
|-----------|---------|
| uv | Package management (workspace) |
| Ruff | Linting and formatting |
| ty | Static type checking |
| pytest | Testing |
| pre-commit | Git hooks |
| FastAPI | HTTP server |
| a2a-sdk | Agent-to-Agent protocol |

## Development Commands

All commands run from `sdk/python/flokoa/`:

```bash
make install    # Create venv, sync deps, install pre-commit hooks
make check      # Run all quality checks (lock, lint, type check)
make test       # Run pytest with coverage
make build      # Build wheel file
make clean-build # Remove build artifacts
```

Workspace-level commands from `sdk/python/`:

```bash
uv sync --all-packages    # Sync all workspace members
uv lock                   # Update the shared lockfile
```

## Generated Types (from Operator CRDs)

The `flokoa-types` package (`sdk/python/flokoa-types/`) contains **auto-generated** Pydantic v2 models from the Kubernetes Operator CRD schemas. Do not edit the generated files manually.

To regenerate, run from the `operator/` directory:
```bash
make generate-python-models
```

This uses `datamodel-codegen` to extract JSON schemas from CRD YAML files and produce Pydantic v2 BaseModel classes:

| Generated File | Source CRD | Key Classes |
|---------------|-----------|-------------|
| `agenttool.py` | `agent.flokoa.ai_agenttools` | `AgentToolSpec` |
| `agentcard.py` | `agent.flokoa.ai_agents` (card field) | `AgentCard` |
| `agentworkflow.py` | `agent.flokoa.ai_agentworkflows` | `AgentWorkflow` |
| `modelconfig.py` | Combined from `Models` + `ModelProviders` | `ModelConfig`, `ProviderType`, provider-specific configs |
| `templateconfig.py` | `agent.flokoa.ai_agents` (runtime.template.config) | `TemplateConfig` |

The generation pipeline:
1. `make manifests` in operator/ generates CRD YAML from Go types
2. `yq` extracts specific JSON schemas from CRD YAML
3. `datamodel-codegen` converts JSON schemas to Pydantic v2 models
4. Output goes to `sdk/python/flokoa-types/src/flokoa_types/`

Import types using `from flokoa_types import ...` (not `from flokoa.types`).

**If you change Go types in `operator/api/v1alpha1/`**, run `make manifests generate generate-python-models` from the operator directory to keep the SDK types in sync.

## CLI Usage

The `flokoa` CLI runs agents locally:

```bash
# Run an agent (requires the pydantic-ai extra)
flokoa run -m my_module:my_agent

# Specify host and port
flokoa run -m my_module:my_agent --host 0.0.0.0 --port 8000
```

The agent argument uses `module:object` syntax (similar to uvicorn).

## Framework Integration

flokoa targets **pydantic-ai** exclusively. A2A serving lives in
`flokoa.serving` and requires the `pydantic-ai` extra:

```bash
pip install flokoa[pydantic-ai]
```

```python
from flokoa.serving import SpecAgentExecutor, build_app
```

`flokoa.context` exposes the agent identity and the in-flight A2A
`contextId`/`taskId` to capability authors.

## Code Conventions

### Linting (Ruff)

Config in `pyproject.toml` (with per-package overrides). The enabled rule sets cover imports
(`I`), pycodestyle/pyflakes (`E`/`W`/`F`), security (`S`, relaxed in tests), bugbear (`B`),
comprehensions (`C4`), complexity (`C90`), pyupgrade (`UP`), simplify (`SIM`), builtins (`A`),
and exception handling (`TRY`). Line length 120; `E501`/`E731`/`TRY003` ignored. Run `make check`.

### Type Checking

Uses `ty` for static type checking. Configure in `pyproject.toml`:

```toml
[tool.ty.environment]
python = ".venv"
python-version = "3.10"
```

### Testing

pytest with coverage:

```bash
make test
```

Test files in `tests/` mirror `src/flokoa/` structure.

## Dependencies

Core dependencies (flokoa):
- `a2a-sdk` - Agent-to-Agent protocol
- `click` - CLI framework
- `fastapi` - HTTP server
- `pydantic` - Data validation

Optional extras:
- `pydantic-ai` - Pydantic AI framework support (>= 1.44.0)
- `tracing` - OpenTelemetry tracing support (opentelemetry-sdk, OTLP exporter, FastAPI instrumentation)

Dev dependencies (in `dependency-groups`):
- `pre-commit`
- `pytest`, `pytest-cov`
- `ruff`

## Pre-commit Hooks

Configured in `.pre-commit-config.yaml`:
- `pre-commit-hooks` - Basic file checks
- `ruff-pre-commit` - Linting and formatting

Install hooks:
```bash
uv run pre-commit install
```

## Multi-Version Testing

tox.ini supports Python 3.10-3.14:

```bash
tox -e py313  # Test specific version
tox           # Test all versions
```

## Common Patterns

### Serving an agent locally

```bash
flokoa run -m my_module:my_agent     # a user-constructed pydantic-ai Agent
flokoa run -f agentspec.yaml         # an AgentSpec file — the local mirror of the cluster runner
```

Tools reach agents as **MCP endpoints** (AgentTool CRs compile to MCP
capability entries); the former OpenAPI toolset machinery is retired.

## CI/CD

GitHub Actions workflow `.github/workflows/test-python.yml`:
- Triggered by changes to `sdk/python/**`
- Uses `astral-sh/setup-uv` for package management
- Runs `uv sync --all-packages --all-extras` + `pytest` with coverage
- Uploads coverage to Codecov
