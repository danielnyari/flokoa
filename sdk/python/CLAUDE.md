# CLAUDE.md - Flokoa Python SDK

This document provides guidance for AI assistants working with the Flokoa Python SDK.

## Overview

The Flokoa Python SDK provides a CLI and library for building and running AI agents locally. It integrates with the A2A (Agent-to-Agent) protocol and supports multiple AI frameworks.

- **Package**: `flokoa`
- **Python**: >= 3.13
- **Package Manager**: uv

## Tech Stack

| Component | Purpose |
|-----------|---------|
| uv | Package management |
| Ruff | Linting and formatting |
| ty | Static type checking |
| pytest | Testing |
| pre-commit | Git hooks |
| FastAPI | HTTP server |
| a2a-sdk | Agent-to-Agent protocol |

## Directory Structure

```
sdk/python/
├── src/flokoa/
│   ├── __init__.py
│   ├── __main__.py            # CLI entrypoint
│   ├── exceptions.py          # Custom exceptions
│   ├── agent_executor/        # Base executor interface
│   ├── integrations/          # Framework integrations
│   │   ├── __init__.py        # Integration registry
│   │   └── pydantic_ai/       # Pydantic AI integration
│   ├── tools/                 # Tool implementations
│   │   └── http_api.py        # HTTP API tools
│   ├── types/                 # Type definitions
│   └── utils/                 # Utility functions
├── tests/
│   └── flokoa_cli/
│       ├── agent_executor/    # Executor tests
│       ├── integrations/      # Integration tests
│       └── fixtures/          # Test fixtures
├── pyproject.toml             # Project configuration
├── Makefile                   # Development commands
├── tox.ini                    # Multi-version testing
└── uv.lock                    # Dependency lock file
```

## Development Commands

All commands run from `sdk/python/`:

```bash
make install    # Create venv, sync deps, install pre-commit hooks
make check      # Run all quality checks (lock, lint, type check)
make test       # Run pytest with coverage
make build      # Build wheel file
make clean-build # Remove build artifacts
```

## CLI Usage

The `flokoa` CLI runs agents locally:

```bash
# Run an agent with framework auto-detection
flokoa my_module:my_agent

# Specify host and port
flokoa my_module:my_agent --host 0.0.0.0 --port 8000

# Specify framework explicitly
flokoa my_module:my_agent --framework pydantic-ai
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
- `S` - flake8-bandit (security)
- `B` - flake8-bugbear
- `C4` - flake8-comprehensions
- `UP` - pyupgrade
- `SIM` - flake8-simplify

Line length: 120 characters

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

Core dependencies:
- `a2a-sdk` - Agent-to-Agent protocol
- `click` - CLI framework
- `fastapi` - HTTP server
- `pydantic` - Data validation

Optional extras:
- `pydantic-ai` - Pydantic AI framework support

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
