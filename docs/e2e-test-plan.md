# End-to-End Test Plan (Operator + Python SDK)

## Goals

- Validate that the Kubernetes Operator and Python SDK work together end-to-end.
- Exercise both **standard Agents** (runtime type `standard`) and **template Agents** (Agents that rely on inline or referenced Instruction templates).
- Cover real dependencies (ModelProvider, Model, Instruction, AgentTool, MCPServer, StateBackend, and in-cluster services).
- **Only mock LLM calls**. Every other dependency must be real and running in the test cluster.

## Selected Framework

**Ginkgo v2 + Gomega on Kind** (extend the existing `operator/test/e2e` suite).

Why:

- The operator already ships with Ginkgo-based e2e tests (`make test-e2e`).
- Ginkgo can orchestrate cluster lifecycle, build/load images, and validate Kubernetes resources via `kubectl`.
- We can call Python SDK commands (via `uv run` or `flokoa` CLI) as part of the same e2e flow without adding a second test harness.

## Test Environment

1. **Kind cluster** created by the existing e2e tooling.
2. **Operator + Server images** built and loaded into the Kind cluster.
3. **Python SDK agent images** built from minimal SDK examples (see `sdk/python/tests/fixtures` or add a small example module under `sdk/python/tests/fixtures/e2e`).
4. **LLM stub service** (the only mock):
   - Simple HTTP service that emulates the configured provider endpoint (OpenAI/Anthropic-compatible JSON).
   - Deterministic responses for assertions.
5. **Real dependencies** deployed in cluster:
   - Tool service (HTTP API) for `AgentTool` reference tests.
   - MCP server container for `MCPServer` connectivity.
   - State backend (Redis/Postgres) container for stateful agents.

## Core Test Scenarios

### 1) Standard Agent (SDK-built container)

**Setup**

- Build a small agent container using the Python SDK (e.g., `flokoa` CLI + `pydantic-ai` integration).
- Create `ModelProvider` + `Model` pointing to the LLM stub.
- Create `AgentTool` backed by a real HTTP service.

**Assertions**

- `Agent` reaches `Ready=True` with `status.phase=Running`.
- Service endpoint is reachable and returns responses.
- Tool invocation calls the real HTTP service (validate response).
- LLM calls go only to the stub provider.

### 2) Template Agent (Instruction templates)

**Setup**

- Create an `Instruction` resource with a template string (e.g., `${ticket_type}` placeholders).
- Create an `Agent` that references the Instruction (or inline instruction template in the Agent spec).
- Provide prompt variables via request payloads.

**Assertions**

- Instruction ConfigMap is created and mounted in the Agent pod.
- Agent responds using the templated instruction with the provided variables.
- LLM calls hit the stub provider, no other mocks.

### 3) Dependency Coverage

For both standard and template agents, validate:

- **ModelProvider/Model** CRDs resolve and are referenced across namespaces.
- **AgentTool** references work for both inline and referenced tools.
- **MCPServer** connection configuration is propagated to the pod.
- **StateBackend** (e.g., Redis) is reachable and persists state across requests.

## Test Execution Flow (Ginkgo)

1. Create namespace, install CRDs, deploy operator/server (existing setup).
2. Deploy LLM stub, tool service, MCP server, and state backend.
3. Build & load Python SDK agent images into Kind.
4. Apply `ModelProvider`, `Model`, `Instruction`, `AgentTool`, and `Agent` manifests.
5. Wait for `Ready` conditions and validate endpoints.
6. Collect logs on failure (existing e2e harness behavior).

## Non-Goals

- Mocking anything besides the LLM provider.
- Unit-testing internal components (covered elsewhere).

## Success Criteria

- Standard and template agents both reach `Ready=True`.
- All dependencies (ModelProvider, Model, Instruction, AgentTool, MCPServer, StateBackend) work together in a live cluster.
- No mocks beyond LLM calls.
