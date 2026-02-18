# CLAUDE.md - Flokoa Python SDK

This document provides guidance for AI assistants working with the Flokoa Python SDK.

## Overview

The Flokoa Python SDK provides a CLI and library for building and running AI agents locally. It integrates with the A2A (Agent-to-Agent) protocol and supports multiple AI frameworks.

- **Package**: `flokoa`
- **Python**: >= 3.13
- **Package Manager**: uv

## Workspace Structure

The SDK is organized as a **uv workspace** with four packages:

```
sdk/python/                          # Workspace root
├── pyproject.toml                   # Workspace definition
├── uv.lock                         # Shared lockfile for all packages
├── flokoa/                          # Public SDK (published to PyPI)
│   ├── pyproject.toml
│   ├── src/flokoa/
│   │   ├── __init__.py
│   │   ├── __main__.py             # CLI: flokoa run -m module:agent
│   │   ├── agent_executor/         # Base executor interface
│   │   ├── integrations/           # Framework integrations (pydantic-ai, google-adk)
│   │   ├── tools/                  # Tool implementations (OpenAPI, etc.)
│   │   └── utils/                  # Config loaders, agent card builder
│   └── tests/
├── flokoa-types/                    # Auto-generated Pydantic models from CRD schemas (DO NOT EDIT generated files)
│   ├── pyproject.toml
│   └── src/flokoa_types/
│       ├── __init__.py             # Re-exports + IntegrationType, ToolType, ToolDefinition (hand-maintained)
│       ├── agentcard.py            # Generated: AgentCard
│       ├── agenttool.py            # Generated: AgentToolSpec
│       ├── agentworkflow.py        # Generated: AgentWorkflow
│       ├── modelconfig.py          # Generated: ModelConfig, ProviderType, etc.
│       ├── taskconfig.py           # Generated: TaskConfig, TaskAgentConfig
│       └── templateconfig.py       # Generated: TemplateConfig
├── flokoa-managed-agent/           # Operator-deployed pydantic-ai agent runtime
│   ├── pyproject.toml              # Depends on flokoa[pydantic-ai]
│   ├── Dockerfile
│   ├── src/flokoa_managed_agent/
│   │   ├── __main__.py             # python -m flokoa_managed_agent
│   │   ├── config.py               # Reads mounted ConfigMap/Secret
│   │   ├── bootstrap.py            # Instantiates pydantic-ai agent from config
│   │   └── agent_executor.py       # TemplatedPydanticAIAgentExecutor
│   └── tests/
└── flokoa-managed-task/            # Operator-deployed Marvin task runtime (scaffold)
    ├── pyproject.toml              # Depends on marvin
    ├── Dockerfile
    └── src/flokoa_managed_task/
        ├── __main__.py
        ├── config.py
        └── bootstrap.py
```

### Package Relationships

- `flokoa` — the public SDK, installable via `pip install flokoa`. Core dependencies: a2a-sdk, click, fastapi, flokoa-types, pydantic. Optional extras: `pydantic-ai`, `google-adk`.
- `flokoa-types` — auto-generated Pydantic v2 models from Kubernetes CRD schemas. Shared dependency for all packages that need CRD types. Import as `flokoa_types`.
- `flokoa-managed-agent` — internal package, never published to PyPI. Built into a container image by the operator. Depends on `flokoa[pydantic-ai]`.
- `flokoa-managed-task` — internal package, scaffold only. Will depend on `marvin`.

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
| `taskconfig.py` | Task configuration | `TaskConfig`, `TaskAgentConfig`, `TaskResultType` |
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
# Run an agent with a specific framework
flokoa run -m my_module:my_agent --framework pydantic-ai

# Specify host and port
flokoa run -m my_module:my_agent --host 0.0.0.0 --port 8000
```

The agent argument uses `module:object` syntax (similar to uvicorn).

## Framework Integrations

Integrations are loaded dynamically based on installed extras:

```bash
# Install with pydantic-ai support
pip install flokoa[pydantic-ai]
```

Currently supported:
- **pydantic-ai**: `flokoa.integrations.pydantic_ai`
- **google-adk**: `flokoa.integrations.google_adk`

### Adding a New Integration

1. Create a new directory in `src/flokoa/integrations/`
2. Implement `FlokoaAgentExecutor` subclass
3. Register in `integrations/__init__.py`:
   - Add to `IntegrationType` enum
   - Add to `_EXTRA_NAMES` mapping
   - Add `_try_load()` call

## Code Conventions

### Linting (Ruff)

Configuration in `pyproject.toml`. Key rules enabled:
- `I` - isort (import sorting)
- `E`, `W` - pycodestyle
- `F` - pyflakes
- `S` - flake8-bandit (security) — disabled in tests for assertions/passwords
- `B` - flake8-bugbear
- `C4` - flake8-comprehensions
- `C90` - mccabe complexity
- `UP` - pyupgrade
- `SIM` - flake8-simplify
- `A` - flake8-builtins
- `YTT` - flake8-2020
- `T10` - flake8-debugger
- `PGH` - pygrep-hooks
- `RUF` - ruff-specific rules
- `TRY` - tryceratops (exception handling)

Line length: 120 characters. Target version: py39.
Ignored: `E501` (line too long), `E731` (lambda assignment), `TRY003` (long exception messages).

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

Test files in `tests/` mirror `src/flokoa/` structure. Fixtures in `tests/flokoa_cli/fixtures/`.

## Dependencies

Core dependencies (flokoa):
- `a2a-sdk` - Agent-to-Agent protocol
- `click` - CLI framework
- `fastapi` - HTTP server
- `pydantic` - Data validation

Optional extras:
- `pydantic-ai` - Pydantic AI framework support (>= 1.44.0)
- `google-adk` - Google ADK framework support (>= 1.14.1)
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

### Creating an Agent Executor

```python
from flokoa.agent_executor import FlokoaAgentExecutor

class MyFrameworkExecutor(FlokoaAgentExecutor):
    def __init__(self, agent):
        self.agent = agent

    async def execute(self, request):
        # Handle the request
        pass
```

### Registering an Integration

In `integrations/__init__.py`:

```python
class IntegrationType(StrEnum):
    MY_FRAMEWORK = "my-framework"

_EXTRA_NAMES[IntegrationType.MY_FRAMEWORK] = "my-framework"

_try_load(
    IntegrationType.MY_FRAMEWORK,
    "flokoa.integrations.my_framework.agent_executor",
    "MyFrameworkExecutor",
)
```

## OpenAPI Tool System

The SDK includes a comprehensive OpenAPI tool system in `src/flokoa/tools/openapi/`:

- `openapi_toolset.py` - Creates tool instances from OpenAPI specs
- `openapi_spec_parser.py` - Parses OpenAPI 3.x specifications
- `operation_parser.py` - Converts API operations to tool definitions
- `rest_api_tool.py` - Executes REST API calls as tools
- `auth/` - Authentication subsystem with OAuth2, service account, and auto-auth credential exchangers

This maps to the `AgentTool` CRD's `openapi` type, providing runtime tool execution for agents.

## CI/CD

GitHub Actions workflow `.github/workflows/test-python.yml`:
- Triggered by changes to `sdk/python/**`
- Uses `astral-sh/setup-uv` for package management
- Runs `uv sync --all-packages --all-extras` + `pytest` with coverage
- Uploads coverage to Codecov
