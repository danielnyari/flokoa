# Pydantic-Graph / Hera / PydanticAI Durable Execution — Investigation Findings

**Date**: 2026-02-15
**Status**: Complete
**Author**: Investigation for Flokoa integration design

---

## 1. Pydantic-Graph Core API Findings

### 1.1 Graph Storage & Topology

**How Graph stores nodes and edges** (`graph.py:24-104`):

- `Graph` is a `@dataclass(init=False)` that stores nodes in `node_defs: dict[str, NodeDef]`.
- Nodes are keyed by their class name (from `BaseNode.get_node_id()` which returns `cls.__name__`).
- The constructor takes `nodes: Sequence[type[BaseNode[...]]]` — a sequence of **classes**, not instances.
- During `__init__`, each node class is registered via `_register_node()` which calls `node.get_node_def(parent_namespace)` to build a `NodeDef`.
- `_validate_edges()` verifies all referenced next-nodes are in the graph.

**Edge extraction** (`nodes.py:104-136`):

Edges come from **return type hints** on `BaseNode.run()`. The key method is `BaseNode.get_node_def()`:

```python
type_hints = get_type_hints(cls.run, localns=local_ns, include_extras=True)
return_hint = type_hints['return']
for return_type in _utils.get_union_args(return_hint):
    if return_type_origin is End:
        end_edge = edge
    elif issubclass(return_type_origin, BaseNode):
        next_node_edges[return_type.get_node_id()] = edge
```

This means the topology is fully extractable from `graph.node_defs` without running any code.

**Public topology access**:
- `graph.node_defs` is a public attribute — `dict[str, NodeDef]`.
- `graph.get_nodes()` returns a list of node classes.
- Each `NodeDef` has `next_node_edges: dict[str, Edge]`, `end_edge: Edge | None`, and `returns_base_node: bool`.
- **`mermaid.py:41-114` already extracts the topology** using the exact same `node_defs` — we can reuse this approach directly.

### 1.2 Node Structure

- `BaseNode` is **not** a `@dataclass` — it's an `ABC` with `Generic[StateT, DepsT, NodeRunEndT]`. Users must apply `@dataclass` themselves.
- Generic params: `StateT` (graph state), `DepsT` (dependencies, contravariant), `NodeRunEndT` (covariant, what this node's `End` produces; defaults to `Never`).
- **Dataclass fields ARE the arguments** — when a node returns `Review(score=0.8)`, the `score` field is populated and available when `Review.run()` is called.
- **Union returns for branching**: A node can return `NodeA | NodeB | End[str]` — this creates edges to both NodeA and NodeB plus an End edge.
- **Data passing between nodes**: The returned node instance carries data in its dataclass fields. This is the primary mechanism for passing data between nodes.

### 1.3 State Management

- **State is mutable and shared by reference**. All nodes receive the same `state` object via `ctx.state`.
- State can be **any type** — dataclass, BaseModel, `None`, int, etc. No constraint beyond what `StateT` resolves to.
- State is **not automatically JSON-serialized** during execution. For inter-container passing, we must handle serialization ourselves.
- **`GraphRunContext`** (`nodes.py:27-34`) is a simple `@dataclass(kw_only=True)` with two fields: `state: StateT` and `deps: DepsT`.
- **DepsT is typically non-serializable** (DB connections, API clients, etc.). This is a fundamental challenge for container-based execution.

### 1.4 Execution Model

The execution loop (`graph.py:106-157`):

```python
async def run(self, start_node, *, state, deps, persistence):
    async with self.iter(start_node, state=state, deps=deps, persistence=persistence) as graph_run:
        async for _node in graph_run:
            pass
    return graph_run.result
```

- Execution is **async-only** (with a `run_sync` convenience wrapper).
- The `GraphRun.next()` method (`graph.py:663-752`) runs one node at a time:
  1. Validates the node is in the graph
  2. Creates `GraphRunContext(state=self.state, deps=self.deps)`
  3. Calls `await node.run(ctx)`
  4. Snapshots the result to persistence
- **No max iteration limit** — cycles run until `End` is returned.
- **No retry logic** — exceptions propagate directly.
- **Cycles are allowed** and idiomatic. The example in the docstring shows `Increment -> Check42 -> Increment` looping until condition is met.

### 1.5 Persistence

The `BaseStatePersistence` ABC (`persistence/__init__.py:106-226`) provides:
- `snapshot_node()` / `snapshot_end()` — save state at each step
- `record_run()` — track which node is running (prevents concurrent execution)
- `load_next()` / `load_all()` — restore state for resumption
- Built-in `SimpleStatePersistence` (in-memory) and `FileStatePersistence`

Snapshots include `state`, `node` (the actual node instance), timestamps, and status. They use Pydantic `TypeAdapter` for serialization, meaning **state and nodes must be Pydantic-serializable for persistence to work**.

---

## 2. Pydantic-Graph Beta API Findings

### 2.1 New API Shape

The beta API (`beta/`) is a **complete rewrite** with fundamentally different concepts:

| Stable API | Beta API |
|---|---|
| Edges from return type hints | Explicit edge definitions via `GraphBuilder` |
| `BaseNode` subclasses | `Step` functions + `GraphBuilder` |
| Dataclass-based nodes | Functions decorated with `@builder.step` |
| Sequential execution | **Parallel execution with Fork/Join** |
| Mutable shared state | Typed inputs/outputs flowing through edges |

**Key classes** (`beta/graph.py:109-155`):

```python
@dataclass
class Graph(Generic[StateT, DepsT, InputT, OutputT]):
    nodes: dict[NodeID, AnyNode]
    edges_by_source: dict[NodeID, list[Path]]
    parent_forks: dict[JoinID, ParentFork[NodeID]]
```

**GraphBuilder** (`beta/graph_builder.py:65-676`) is the main entry point:

```python
builder = GraphBuilder(state_type=MyState, input_type=str, output_type=str)

@builder.step
async def research(ctx: StepContext) -> str:
    return "findings"

builder.add(builder.edge_from(builder.start_node).to(research))
builder.add(builder.edge_from(research).to(builder.end_node))
graph = builder.build()
```

### 2.2 Parallel Execution (Fan-out / Fan-in)

The beta API has **first-class parallel execution**:

- **`Fork`** (`beta/node.py:60-95`): Splits execution into parallel branches. Two modes:
  - `is_map=True`: `InputT` is `Sequence[OutputT]`, each element runs in parallel (dynamic fan-out)
  - `is_map=False`: Same data broadcast to all branches (static fan-out)

- **`Join`** (`beta/join.py:150-199`): Synchronizes parallel branches with a **reducer pattern**:
  ```python
  join = builder.join(reduce_list_append, initial_factory=list)
  ```
  Built-in reducers: `reduce_list_append`, `reduce_list_extend`, `reduce_dict_update`, `reduce_sum`, `reduce_null`, `ReduceFirstValue` (with early stopping).

- **`map()` on edges**:
  ```python
  builder.add_mapping_edge(source, map_to=destination)
  ```

- **`broadcast()` on edges**: Static fan-out to multiple known destinations.

- Cardinality: **Both static and dynamic** fan-out are supported.
  - Static: `broadcast()` with known destinations at build time
  - Dynamic: `map()` where count depends on runtime iterable length

**Execution uses `anyio.create_task_group()`** for actual concurrent execution (`beta/graph.py:565`).

### 2.3 Decisions

**`Decision`** (`beta/decision.py:41-67`) provides conditional branching:

```python
decision = builder.decision()
decision = decision.branch(builder.match(TypeA).to(step_a))
decision = decision.branch(builder.match(TypeB).to(step_b))
```

Branch matching supports:
- `isinstance` checks (default)
- `Literal` value matching
- Custom `matches` callable
- `Any`/`object` catch-all

Decision functions **are not serializable** — the `matches` parameter is an opaque callable. For Argo compilation, we'd need to convert these to Argo `when` expressions, which requires the decision logic to be expressible as simple value comparisons.

### 2.4 Implications for Flokoa

The beta API changes the integration approach significantly:

1. **Topology extraction is different** — must read `edges_by_source` and `Decision.branches` instead of return type hints.
2. **Parallel execution maps naturally to Argo DAG fan-out** — static `broadcast()` maps to parallel DAG tasks; dynamic `map()` needs special handling.
3. **Join/Reduce is harder** — Argo doesn't have native reduce semantics. We'd need to either:
   - Run the entire fork-join section as a single task
   - Use Argo artifact passing to collect outputs and a final reduce task
4. **The beta API is unstable** — it's in the `beta/` package and may change.

---

## 3. PydanticAI Durable Execution Findings

### 3.1 TemporalAgent

**Architecture** (`temporal/_agent.py:55-1031`):

`TemporalAgent` extends `WrapperAgent` and wraps an agent to work inside Temporal workflows by:

1. **Wrapping the model** → `TemporalModel` replaces `model.request()` with `workflow.execute_activity()` calls
2. **Wrapping toolsets** → `TemporalWrapperToolset` replaces tool calls with activities
3. **Detecting context** → `workflow.in_workflow()` toggles between normal and Temporal behavior

**Activity naming** (`temporal/_agent.py:140`):
```python
activity_name_prefix = f'agent__{self.name}'
# Model: f'{activity_name_prefix}__model_request'
# Tools: f'{activity_name_prefix}__{toolset_id}__call_tool'
```

**Key constraints**:
- `run_stream()` raises `UserError` inside workflows — must use `event_stream_handler` instead
- `run_sync()` raises `UserError` inside workflows
- Model and toolsets frozen at wrap time — can't be changed at runtime in workflows
- Deps must be Pydantic-serializable (raises `PydanticSerializationError` otherwise)

### 3.2 TemporalModel

**How model requests become activities** (`temporal/_model.py:76-326`):

```python
class TemporalModel(WrapperModel):
    async def request(self, messages, model_settings, model_request_parameters):
        if not workflow.in_workflow():
            return await super().request(...)  # Normal path
        # Inside workflow: delegate to activity
        return await workflow.execute_activity(
            activity=self.request_activity,
            args=[_RequestParams(...), deps],
            **activity_config,
        )
```

The `_RequestParams` dataclass bundles messages, settings, and serialized run context for the activity boundary.

### 3.3 Serialization Boundary

**`TemporalRunContext`** (`temporal/_run_context.py:14-62`) handles serialization:

```python
@classmethod
def serialize_run_context(cls, ctx: RunContext[Any]) -> dict[str, Any]:
    return {
        'run_id': ctx.run_id,
        'metadata': ctx.metadata,
        'retries': ctx.retries,
        # ... other serializable fields
    }
```

Users can subclass `TemporalRunContext` to include additional fields.

### 3.4 PrefectAgent

The Prefect integration follows the same pattern but with Prefect tasks instead of Temporal activities. Each model request and tool call becomes a Prefect task with built-in caching.

### 3.5 WrapperAgent Base Class

`WrapperAgent` provides the interface for wrapping agents:
- Delegates all `AbstractAgent` methods to the wrapped agent
- Provides `override()` context manager for temporary overrides
- Subclasses override `model`, `toolsets`, `run()`, etc.

**Flokoa could implement `FlokoaAgent(WrapperAgent)`** following the same pattern, adapting the wrapping strategy based on the detected runtime environment.

### 3.6 Key Constraints

1. **2MB Temporal payload limit** — affects large agent outputs, long message histories, and tool results. Image output is explicitly blocked.
2. **Streaming not supported** in Temporal workflows — must use `event_stream_handler` callback pattern.
3. **Non-serializable deps** — users must handle reconstruction in each container (e.g., from env vars/secrets).
4. **TemporalAgent requires a Temporal worker** — the agent's workflow must run on a Temporal worker. Running in an ephemeral pod means the pod must either:
   - Be a Temporal worker itself (startup overhead)
   - Connect to an external Temporal cluster for activities only

---

## 4. Hera (Argo Workflows Python SDK) Findings

### 4.1 Core API

**`@script` decorator**: Wraps a Python function into an Argo Workflow script template. With `constructor="runner"`, the function is serialized and runs via the Hera Runner, which handles Pydantic BaseModel deserialization.

**Input/Output handling**:
- Function parameters become Argo input parameters
- Pydantic BaseModel inputs are serialized as JSON and deserialized by the Hera Runner
- Output artifacts: `Annotated[T, Artifact(name="output")]` pattern
- Output parameters captured from stdout or explicit annotation

**Image**: The `@script` decorator accepts an `image` parameter. With `constructor="runner"`, code is serialized into the container — but the image must have the same Python dependencies installed.

### 4.2 DAG Construction

```python
with DAG(name="main"):
    task_a = my_func_a(arguments={"input": "value"})
    task_b = my_func_b(arguments={"input": task_a.result})
    task_a >> task_b  # dependency
```

- The `>>` operator creates task dependencies
- `when` conditions: `task.when = "{{tasks.upstream.outputs.result}} == expected"`
- Conditional edges work via Argo's `when` expressions (string comparisons on output parameters)

### 4.3 Function Serialization

Two modes:
1. **Default (`constructor=None`)**: Function body serialized as a Python script string in the Workflow YAML. Dependencies must be in the image.
2. **Runner (`constructor="runner"`)**: Hera Runner deserializes inputs/outputs using Pydantic. The image must have `hera[runner]` installed.

**Custom images**: Fully supported. Set `image="my-registry/my-image:tag"` on the `@script` decorator. The image must have:
- The user's Python code and dependencies
- `hera[runner]` if using `constructor="runner"`

### 4.4 Async Function Support

Hera script functions can be **sync only** by default. Async functions require wrapping in `asyncio.run()` or using the runner with async support. This needs investigation for pydantic-graph nodes which are always async.

### 4.5 YAML Generation Without Argo Server

**Hera can generate Workflow YAML without an Argo server**:
```python
w = Workflow(name="my-workflow", entrypoint="main")
# ... build workflow ...
print(w.to_yaml())  # Get YAML string
w.to_dict()  # Get dict representation
```

This is critical for Flokoa — we can generate Argo Workflow CRDs without connecting to Argo.

### 4.6 Limitations

- **Parameter size**: Argo parameters are limited to ~256KB (configurable). For larger data, use artifacts (S3/MinIO).
- **Retry policies**: Supported per template via `retry_strategy` on the template.
- **Timeout per task**: Supported via `active_deadline_seconds` on the template.
- **Dataclass serialization**: Not directly supported by Hera Runner — only Pydantic BaseModel. Dataclass nodes would need conversion.

---

## 5. Integration Design Analysis

### 5.1 Node → Task Mapping

| Graph Pattern | Argo/Hera Mapping | Complexity |
|---|---|---|
| Linear (A→B→C) | Sequential DAG tasks | Simple |
| Branch (A→B\|C) | DAG tasks with `when` conditions | Medium — need to serialize branch decision as output parameter |
| Cycle (A↔B until cond) | **Bundle as single task** running subgraph in-process | Simple once detected |
| Fan-out (map) | Dynamic: single task with `withItems`/`withParam`. Static: parallel DAG tasks | Medium |
| Fan-in (join/reduce) | Separate reduce task consuming all upstream outputs | Medium-Hard |
| Agent node (uses LLM) | `agentTask` (ephemeral container) or A2A call | Varies |

### 5.2 State Passing Between Containers

**Recommended approach**:
1. **State must be a Pydantic BaseModel** (or at least JSON-serializable) for inter-container passing.
2. **Small state (<256KB)**: Pass as Argo parameters (inline JSON)
3. **Large state (>256KB)**: Pass as Argo artifacts stored in S3/MinIO
4. **Node dataclass fields**: Serialize as JSON parameters between tasks
5. **DepsT**: Reconstruct in each container from environment variables and mounted secrets. Document this as a user requirement.

### 5.3 Code Packaging

**Recommended approach**: User builds a custom image extending Flokoa's runtime image:

```dockerfile
FROM ghcr.io/danielnyari/flokoa/agent-runtime:latest
COPY ./my_graph.py /app/
RUN pip install my-dependencies
```

This image is specified in the Flokoa Agent CR and used for all task containers.

### 5.4 Cycle Handling Strategy

1. Use `classify_for_compilation()` from the topology extractor
2. SCCs with >1 node → bundle as a single Argo task that runs the subgraph in-process via `Graph.run()`
3. This preserves pydantic-graph's native cycle handling while keeping the DAG structure valid for Argo

### 5.5 Conditional Branch Strategy

The challenge: pydantic-graph determines the next node at **runtime** (inside `run()`), but Argo needs conditions as **string expressions** on output parameters.

**Solution**: Each task outputs a `next_node` parameter. Downstream tasks use `when` conditions to check this parameter:

```yaml
- name: tech-support
  depends: classify
  when: "{{tasks.classify.outputs.parameters.next_node}} == TechSupport"
```

This works for simple type-based branching but **cannot express arbitrary Python logic** in Argo `when` conditions. Complex branching must be bundled.

---

## 6. Two-Level Durability Analysis

### 6.1 Level 1: Graph → Workflow

A pydantic-graph can be run as either:
- **Argo Workflow**: Each acyclic node → Argo DAG task. Cyclic SCCs → bundled tasks.
- **Temporal Workflow**: Each node → Temporal activity. The workflow walks the graph.

For Temporal-only mode, this is straightforward — the entire graph becomes a Temporal workflow with node executions as activities. No cycle limitation since Temporal workflows support loops natively.

### 6.2 Level 2: TemporalAgent Inside Tasks

If an `agentTask` uses PydanticAI agents:
- **TemporalAgent requires a Temporal worker process** — the pod needs to run as a Temporal worker, or at minimum connect to Temporal for executing activities.
- **This adds significant overhead** for ephemeral pods: Temporal worker startup, connection to Temporal cluster, worker registration.
- **Alternative**: Use `PrefectAgent` for lighter-weight task-level caching inside Argo tasks, or run agents without durable wrappers and rely on Argo-level retries.

### 6.3 Recommendation

For the initial implementation:
- **Level 1 (Argo)**: Use the converter to map pydantic-graph → Argo DAG. This gives workflow-level durability.
- **Level 1 (Temporal)**: Directly walk the graph as a Temporal workflow for backends that support it.
- **Level 2**: Skip TemporalAgent inside Argo tasks initially. Use PrefectAgent or plain agents with Argo retry policies. Revisit when there's demand for LLM-call-level durability.

---

## 7. FlokoaAgent Wrapper Design

### 7.1 Auto-Detection

The agent can detect its environment via environment variables:

```python
import os

engine = os.environ.get("FLOKOA_ENGINE")  # "argo" | "temporal" | None
temporal_addr = os.environ.get("TEMPORAL_ADDRESS")
task_name = os.environ.get("FLOKOA_TASK_NAME")
# Argo sets: ARGO_CONTAINER_NAME, ARGO_TEMPLATE, etc.
```

### 7.2 Wrapping Strategy

```python
class FlokoaAgent(WrapperAgent[AgentDepsT, OutputDataT]):
    def __init__(self, wrapped, **kwargs):
        super().__init__(wrapped)
        engine = os.environ.get("FLOKOA_ENGINE")
        if engine == "temporal" and TEMPORAL_AVAILABLE:
            self._inner = TemporalAgent(wrapped, **kwargs)
        elif engine == "argo" and PREFECT_AVAILABLE:
            self._inner = PrefectAgent(wrapped, **kwargs)
        else:
            self._inner = wrapped  # No wrapping outside Flokoa
```

### 7.3 Tool Injection

Flokoa Tool CRDs mounted as ConfigMaps:
1. Load tools from `/etc/flokoa/tools.json` at startup
2. Register with agent before wrapping
3. Order: load tools → register → wrap with durable execution

TemporalAgent freezes toolsets at wrap time, so tools must be loaded first.

---

## 8. Architectural Recommendation

### Should we use Hera as the compilation target?

**Yes, with caveats.**

**Pros**:
- Hera generates Argo Workflow YAML without an Argo server — pure library
- Hera handles Pydantic model serialization via Runner
- Python-native API is more maintainable than generating YAML directly
- Hera supports all Argo features (retries, timeouts, artifacts, DAGs)

**Caveats**:
- The Hera Runner requires `hera[runner]` in the container image
- Async functions need explicit wrapping
- Hera's `@script` with `constructor="runner"` expects Pydantic BaseModel inputs, not dataclasses — pydantic-graph nodes are dataclasses
- We can generate YAML directly as a fallback (avoiding Hera Runner entirely)

### Recommended Architecture

```
User's pydantic-graph
        │
        ▼
┌─────────────────────────┐
│  Topology Extractor     │  ← Extract edges, detect cycles
└─────────┬───────────────┘
          │
          ▼
┌─────────────────────────┐
│  Graph Compiler         │  ← Classify SCCs, plan task mapping
└─────────┬───────────────┘
          │
    ┌─────┴─────┐
    ▼           ▼
┌────────┐ ┌──────────┐
│  Argo  │ │ Temporal │
│ Backend│ │ Backend  │
└───┬────┘ └────┬─────┘
    │           │
    ▼           ▼
  Hera       Temporal
  YAML       Workflow
  (DAG)      (activities)
```

### What about Temporal-only mode?

Temporal-only is simpler and should be the first implementation:
- No cycle limitation (Temporal workflows support loops)
- No serialization issues for branch conditions
- TemporalAgent integration is natural
- No container image requirements

Argo mode is the second phase, using Hera for YAML generation.

---

## 9. Risk Register

### High Risk

| Risk | Impact | Mitigation |
|---|---|---|
| **State serialization** — user state may contain non-serializable objects (DB connections, file handles) | State can't be passed between containers | Document requirement: state must be JSON-serializable. Provide `FlokaoState` base class with validation. |
| **DepsT is non-serializable** — pydantic-graph deps are typically runtime objects | Can't reconstruct deps in new containers | Provide `FlokaoDepFactory` that reconstructs deps from env vars/secrets in each container. |
| **Conditional branch logic is opaque** — Python `if` statements inside `run()` can't be translated to Argo `when` expressions | Complex branching must be bundled as single tasks, reducing parallelism | Support simple type-based branching in Argo; bundle complex logic. Document the limitation. |
| **Beta API instability** — the beta API may change significantly before stabilization | Our integration code may break | Build against the stable API first. Add beta API support behind a feature flag. |

### Medium Risk

| Risk | Impact | Mitigation |
|---|---|---|
| **Dataclass vs BaseModel** — pydantic-graph uses dataclasses, Hera Runner expects BaseModel | Serialization mismatch at container boundary | Convert dataclass nodes to Pydantic models at the serialization boundary, or generate YAML directly without Hera Runner. |
| **Large state/messages** — Argo parameters limited to ~256KB | LLM message histories can exceed this | Use Argo artifacts (S3/MinIO) for states exceeding threshold. Auto-detect size and switch. |
| **Async in Argo containers** — pydantic-graph nodes are `async def` | Hera `@script` expects sync functions | Wrap with `asyncio.run()` in generated scripts. |
| **TemporalAgent in ephemeral pods** — requires Temporal worker startup | Adds latency and memory overhead to each pod | Defer to PrefectAgent or plain agents for Argo backend. Only use TemporalAgent in Temporal-only mode. |

### Low Risk

| Risk | Impact | Mitigation |
|---|---|---|
| **Cycle detection performance** — Tarjan's algorithm on large graphs | Negligible for expected graph sizes (< 100 nodes) | None needed. |
| **Hera version compatibility** — Hera API may change | Build breaks | Pin Hera version. |
| **`returns_base_node` wildcard** — nodes returning `BaseNode` type directly | Can't determine edges at compile time | Treat as needing runtime resolution; bundle in single task. |

---

## 10. Deliverables Summary

1. **This findings document**: Key insights, gotchas, and constraints from reading source code of pydantic-graph, Hera, and PydanticAI durable execution.

2. **Topology extractor prototype**: `topology_extractor.py`
   - `extract_topology()` — extracts edges from stable `Graph.node_defs`
   - `extract_beta_topology()` — extracts edges from beta `Graph.edges_by_source`
   - `find_cycles()` — DFS-based cycle detection
   - `find_strongly_connected_components()` — Tarjan's algorithm
   - `classify_for_compilation()` — separates DAG-safe nodes from bundled SCCs

3. **Hera converter prototype**: `hera_converter.py`
   - `convert_graph_to_hera_spec()` — converts a pydantic-graph to a `HeraWorkflowSpec`
   - `render_hera_code()` — renders to Hera Python code
   - `render_argo_yaml()` — renders to raw Argo Workflow YAML dict
   - Handles linear, branching, and cyclic graphs

4. **Architectural recommendation**: Use Hera as Argo compilation target; implement Temporal-only mode first; build FlokoaAgent following WrapperAgent pattern.

5. **Risk register**: 4 high-risk items, 4 medium-risk items, 3 low-risk items with mitigations.
