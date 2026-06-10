# Python SDK

The Flokoa Python SDK provides a CLI and library for building, running, and serving AI agents locally. It integrates with the A2A (Agent-to-Agent) protocol and supports multiple AI frameworks.

## Overview

The SDK lets you:

- Run agents locally with a single command using the `flokoa` CLI
- Serve agents over HTTP with A2A protocol support
- Use OpenAPI specifications to give agents access to REST APIs
- Integrate with pydantic-ai, Google ADK, or your own framework
- Load operator-managed configuration (model, tools, instructions) automatically

The SDK is published as `flokoa` on PyPI and requires Python 3.13 or later.

## Installation

Install the base SDK:

```bash
pip install flokoa
```

Install with a framework integration:

```bash
# pydantic-ai support
pip install flokoa[pydantic-ai]

# Google Agent Development Kit support
pip install flokoa[google-adk]
```

Install with OpenTelemetry tracing:

```bash
pip install flokoa[tracing]
```

You can combine extras:

```bash
pip install flokoa[pydantic-ai,tracing]
```

## Quick Start

1. Create an agent module (e.g., `my_agent.py`):

```python
from pydantic_ai import Agent

agent = Agent("openai:gpt-4o", system_prompt="You are a helpful assistant.")
```

2. Run it with the Flokoa CLI:

```bash
flokoa run -m my_agent:agent --framework pydantic-ai
```

This starts an A2A-compatible HTTP server on `localhost:10001` that serves your agent. The server includes health check endpoints and an auto-generated agent card.

## CLI Reference

The `flokoa` CLI provides the `run` command for starting agent servers.

### `flokoa run`

```
flokoa run -m <module:object> --framework <framework> [--host HOST] [--port PORT]
```

| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `-m`, `--module` | Yes | -- | Module path to the agent, using `module:object` syntax (similar to uvicorn) |
| `--framework` | Yes | -- | AI framework to use (`pydantic-ai` or `google-adk`) |
| `--host` | No | `localhost` | Host to bind the server to |
| `--port` | No | `10001` | Port to bind the server to |

The `--module` argument uses Python import syntax. The part before the colon is the module to import, and the part after is the attribute name of the agent object:

```bash
# Import 'agent' from 'my_module'
flokoa run -m my_module:agent --framework pydantic-ai

# Import from a package
flokoa run -m my_package.agents:customer_agent --framework pydantic-ai

# Bind to all interfaces on port 8000
flokoa run -m my_module:agent --framework pydantic-ai --host 0.0.0.0 --port 8000
```

The CLI adds the current working directory to `sys.path`, so your agent module can be in the current directory or any installed package.

## Framework Integrations

Flokoa supports multiple AI frameworks through a plugin system. Each integration wraps a framework-specific agent into a `FlokoaAgentExecutor` that the A2A server can use.

### pydantic-ai

```bash
pip install flokoa[pydantic-ai]
```

Create a pydantic-ai agent and run it:

```python
# my_agent.py
from pydantic_ai import Agent

agent = Agent("openai:gpt-4o", system_prompt="You are a helpful assistant.")
```

```bash
flokoa run -m my_agent:agent --framework pydantic-ai
```

Requires pydantic-ai >= 1.44.0.

### Google ADK

```bash
pip install flokoa[google-adk]
```

Create a Google ADK agent and run it:

```python
# my_agent.py
from google.adk.agents import LlmAgent

agent = LlmAgent(name="assistant", model="gemini-2.0-flash")
```

```bash
flokoa run -m my_agent:agent --framework google-adk
```

Requires google-adk >= 1.14.1.

### Adding a New Framework Integration

To add support for a new AI framework:

1. Create a directory at `src/flokoa/integrations/<framework_name>/`
2. Implement a subclass of `FlokoaAgentExecutor`:

```python
from flokoa.agent_executor import FlokoaAgentExecutor

class MyFrameworkExecutor(FlokoaAgentExecutor):
    def __init__(self, agent):
        self.agent = agent

    async def execute(self, request):
        # Handle the A2A request using your framework
        pass
```

3. Register the integration in `src/flokoa/integrations/__init__.py`:

```python
# Add to the IntegrationType enum (in flokoa-types)
class IntegrationType(StrEnum):
    MY_FRAMEWORK = "my-framework"

# Add to _EXTRA_NAMES
_EXTRA_NAMES[IntegrationType.MY_FRAMEWORK] = "my-framework"

# Add _try_load() call
_try_load(
    IntegrationType.MY_FRAMEWORK,
    "flokoa.integrations.my_framework.agent_executor",
    "MyFrameworkExecutor",
)
```

4. Add the optional dependency in `pyproject.toml`:

```toml
[project.optional-dependencies]
my-framework = ["my-framework-lib>=1.0.0"]
```

## OpenAPI Tool System

The SDK includes a tool system that lets agents call REST APIs defined by OpenAPI specifications. This is located in `src/flokoa/tools/openapi/` and maps to the `AgentTool` CRD's `openapi` type.

Key components:

- **OpenAPI Toolset** (`openapi_toolset.py`) -- Creates tool instances from OpenAPI specs, making each API operation available as a tool the agent can call
- **REST API Tool** (`rest_api_tool.py`) -- Executes individual REST API calls as agent tools
- **Auth subsystem** (`auth/`) -- Handles authentication for API calls, supporting OAuth2 and service account credentials

When deployed with the Flokoa operator, tool definitions are loaded from `/etc/flokoa/tools/*.json`. Each JSON file follows the `AgentTool` CRD structure:

```json
{
  "name": "weather-api",
  "spec": {
    "type": "openapi",
    "description": "Get weather data",
    "openApi": {
      "url": "https://api.weather.example.com",
      "openApiSchema": {
        "endpointPath": "/openapi.json"
      }
    }
  }
}
```

## A2A Protocol Support

The SDK serves agents using the [Agent-to-Agent (A2A) protocol](https://github.com/google/A2A), built on FastAPI and the `a2a-sdk` library.

When you run `flokoa run`, the CLI:

1. Imports your agent object
2. Wraps it in a framework-specific `FlokoaAgentExecutor`
3. Creates an A2A-compatible FastAPI application with `A2AFastAPIApplication`
4. Generates or loads an agent card (A2A metadata describing the agent's capabilities)
5. Starts a Uvicorn HTTP server

The server exposes:

- A2A protocol endpoints for agent communication
- Health check endpoints at `/health` and `/ready`
- An agent card endpoint describing the agent's skills and capabilities

### Agent Card

The agent card can be provided in two ways:

- **From file**: Place an `agent-card.json` at `/etc/flokoa/agent-card.json` (used when deployed by the operator)
- **Auto-generated**: If no file is found, the SDK builds a card automatically from the agent object using `AgentCardBuilder`

## Generated Types (`flokoa-types`)

The `flokoa-types` package contains Pydantic v2 models auto-generated from the Kubernetes CRD schemas. It is a dependency of the SDK and should not be edited manually.

Import types using the `flokoa_types` module name:

```python
from flokoa_types import ModelConfig, ToolDefinition, IntegrationType
from flokoa_types.modelconfig import ProviderType
from flokoa_types.agenttool import AgentToolSpec
from flokoa_types.agentcard import AgentCard
```

| Module | Key Classes |
|--------|-------------|
| `modelconfig` | `ModelConfig`, `ProviderType`, provider-specific configs |
| `agenttool` | `AgentToolSpec` |
| `agentcard` | `AgentCard` |
| `agentworkflow` | `AgentWorkflow` |
| `taskconfig` | `TaskConfig`, `TaskAgentConfig` |
| `templateconfig` | `TemplateConfig` |

To regenerate types after changing CRD Go types:

```bash
cd operator/
make manifests generate generate-python-models
```

## Development Setup

The SDK uses [uv](https://docs.astral.sh/uv/) for package management. It is organized as a uv workspace with four packages:

| Package | Purpose | Published |
|---------|---------|-----------|
| `flokoa` | Public SDK (CLI, integrations, tools) | Yes (PyPI) |
| `flokoa-types` | Auto-generated Pydantic v2 models from CRD schemas | Yes (PyPI) |
| `flokoa-managed-agent` | Operator-deployed pydantic-ai agent runtime | No (internal) |
| `flokoa-managed-task` | Operator-deployed Marvin task runtime (scaffold) | No (internal) |

### Getting Started

From `sdk/python/flokoa/`:

```bash
# Install dependencies, create venv, set up pre-commit hooks
make install

# Run tests with coverage
make test

# Run linting (ruff) and type checking (ty)
make check

# Build wheel
make build
```

From `sdk/python/` (workspace root):

```bash
# Sync all workspace members
uv sync --all-packages --all-extras

# Update the shared lockfile
uv lock
```

### Testing

Tests use pytest with coverage and live in the `tests/` directory:

```bash
make test
```

### Linting and Type Checking

Ruff handles linting and formatting. The `ty` tool handles static type checking:

```bash
make check
```

## Configuration

When deployed by the Flokoa operator, the SDK automatically loads configuration from mounted files. For local development, these files are optional and the agent falls back to its own defaults.

### Configuration Files

| Path | Purpose | Loader Function |
|------|---------|-----------------|
| `/etc/flokoa/agent-card.json` | Agent card (A2A metadata) | `load_agent_card()` |
| `/etc/flokoa/model.json` | Model and provider configuration | `load_model_config()` |
| `/etc/flokoa/instruction.txt` | System prompt / instruction text | `load_instruction()` |
| `/etc/flokoa/tools/*.json` | Tool definitions (one file per tool) | `load_tools()` |

### Caching

The SDK caches loaded configuration with automatic invalidation:

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `FLOKOA_CACHE_TTL_SECONDS` | `60` | TTL for cached configs in seconds |
| `FLOKOA_CACHE_ENABLED` | `true` | Enable or disable config caching |

### Other Environment Variables

| Variable | Description |
|----------|-------------|
| `FLOKOA_AGENT_URL` | Override URL for the agent card |
| `FLOKOA_INSTRUCTION_PATH` | Override path for instruction file |
