# AgentWorkflow DSL Design Principles

The AgentWorkflow CRD is a purpose-built DSL for orchestrating AI agent tasks on Kubernetes. It compiles down to Argo Workflows but deliberately hides the mechanical complexity of Argo's execution model. This document defines the design principles that govern the DSL, with side-by-side comparisons showing what the user writes versus what Argo requires.

---

## Principle 1: Use Argo Keywords Where They Make Sense

The DSL borrows terminology from Argo Workflows wherever the concept maps cleanly to agent orchestration. Users who know Argo should feel at home; users who don't shouldn't need to learn Argo to use it.

**Borrowed keywords:**

| AgentWorkflow keyword | Argo equivalent | Purpose |
|---|---|---|
| `tasks` | `dag.tasks` | Define the units of work |
| `dependencies` | `dag.tasks[].dependencies` | Declare execution order |
| `params` | `arguments.parameters` | Workflow-level inputs |
| `retryStrategy` | `retryStrategy` | Retry failed tasks |
| `timeout` | `activeDeadlineSeconds` | Time-bound execution |
| `condition` | `when` | Conditional task execution |
| `switch` | `when` (compiled per-branch) | Multi-way conditional routing |

**Example — DAG with dependencies:**

```yaml
# AgentWorkflow DSL
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentWorkflow
metadata:
  name: research-and-summarize
spec:
  params:
    - name: topic
      value: "transformer architectures"
  tasks:
    - name: research
      agent:
        name: researcher-agent
        text: "Find recent papers on {{params.topic}}"
      timeout: 10m

    - name: summarize
      agent:
        name: summarizer-agent
        text: "Summarize these findings: {{tasks.research.output}}"
      dependencies: [research]
```

The `tasks`, `dependencies`, `params`, and `timeout` keywords are immediately recognizable to anyone familiar with Argo. The key difference: in Argo, `tasks` only appears nested under a `dag` template; in AgentWorkflow, `tasks` is the top-level spec because every workflow is an agent DAG by default.

---

## Principle 2: A Simplified Argo, Not a Restricted One

AgentWorkflow is purpose-built for AI agent orchestration but not artificially constrained. It supports four task types: calling a deployed agent via A2A (`agent`), running an ephemeral LLM task (`agentTask`), running an arbitrary container (`container`), and making HTTP requests (`http`). The container and HTTP types provide the same capabilities as their Argo equivalents but with the same simplified syntax and automatic I/O handling that `agent` and `agentTask` enjoy.

**What the DSL keeps:**
- DAG-based task orchestration with `dependencies`
- Workflow-level `params` with default values
- Per-task `timeout` and `retryStrategy` (with backoff)
- Conditional execution via `condition` and `switch`
- Expression-based data passing between tasks
- Container execution for arbitrary workloads (simplified syntax)
- HTTP requests for API integration (simplified syntax)

**What the DSL drops:**
- Steps-based sequential workflows (unnecessary — `dependencies` chains express sequences)
- Script, resource, and suspend template types
- Artifacts as a user-facing concept (I/O defaults to parameters; artifact-based I/O is an operator-level opt-in — see Artifact I/O Mode)
- `withItems` / `withParam` loops
- `exitHandler`, `onExit`, `hooks`
- Sidecars, init containers, daemon tasks
- Memoization, synchronization, mutex/semaphore
- Node selectors, tolerations, pod metadata

The container and HTTP task types are **simplified in syntax, not in functionality**. Users write a flat container or HTTP spec at the task level; the compiler synthesizes the Argo template, wires up output parameters, injects the traceparent for observability, and translates all DSL expressions. The user never sees the generated template indirection, input/output parameter declarations, or valueFrom paths.

**What uniform means:** every task type — `agent`, `agentTask`, `container`, `http`, `switch` — shares the same task-level features. `dependencies`, `condition`, `timeout`, and `retryStrategy` work identically regardless of task type. The DAG doesn't care what's inside the task; it only cares about execution order and conditions.

**Example — the same pipeline a user would need in raw Argo:**

```yaml
# Raw Argo Workflow — what the user would have to write manually
apiVersion: argoproj.io/v1alpha1
kind: WorkflowTemplate
metadata:
  name: research-and-summarize
spec:
  entrypoint: main
  arguments:
    parameters:
      - name: topic
        value: "transformer architectures"
      - name: _flokoa_traceparent
  templates:
    - name: main
      dag:
        tasks:
          - name: research
            template: research
          - name: summarize
            template: summarize
            dependencies: [research]

    - name: research
      activeDeadlineSeconds: 600
      plugin:
        a2a:
          agent: researcher-agent
          message:
            parts:
              - text:
                  text: "Find recent papers on {{workflow.parameters.topic}}"
          traceparent: "{{workflow.parameters._flokoa_traceparent}}"
      outputs:
        parameters:
          - name: result
          - name: artifact

    - name: summarize
      plugin:
        a2a:
          agent: summarizer-agent
          message:
            parts:
              - text:
                  text: "Summarize these findings: {{tasks.research.outputs.parameters.result}}"
          traceparent: "{{workflow.parameters._flokoa_traceparent}}"
      outputs:
        parameters:
          - name: result
          - name: artifact
```

The raw Argo version requires 3 templates, explicit output parameter declarations, a manually injected traceparent parameter, and the full `{{tasks.research.outputs.parameters.result}}` reference syntax. The AgentWorkflow DSL collapses this to a flat list of tasks with `{{tasks.research.output}}`.

---

## Principle 3: No Entrypoint — The Workflow *Is* the DAG

In Argo, every workflow needs an `entrypoint` field that names the top-level template. The entrypoint template then references other templates. This is necessary because Argo supports multiple template types (steps, DAG, container, script) and needs to know which one to start with.

AgentWorkflow has exactly one execution model: a DAG of agent tasks. There is no template selection, no entrypoint indirection. The `tasks` list in the spec *is* the workflow.

**Argo requires:**

```yaml
spec:
  entrypoint: main           # Required: names the starting template
  templates:
    - name: main              # The entrypoint template
      dag:
        tasks:                # The actual work is here
          - name: A
            template: do-A    # References yet another template
          - name: B
            depends: A
            template: do-B
    - name: do-A              # Template definition for A
      # ...
    - name: do-B              # Template definition for B
      # ...
```

**AgentWorkflow requires:**

```yaml
spec:
  tasks:                       # This IS the workflow
    - name: A
      agent:
        name: agent-a
        text: "do the thing"
    - name: B
      agent:
        name: agent-b
        text: "follow up: {{tasks.A.output}}"
      dependencies: [A]
```

The compiler synthesizes the `entrypoint: main` and the wrapping DAG template automatically. The user never sees it.

---

## Principle 4: No Templates — Tasks Are Definitions, Not References

Argo's template system is its most powerful and most confusing feature. A DAG `task` in Argo doesn't contain the work — it *references* a template that contains the work. This creates a two-level indirection:

1. The DAG template lists tasks, each pointing to a template name
2. Separate template definitions contain the actual container/script/plugin specs

This means a simple two-step pipeline requires three templates: one DAG template and two task templates. Adding parameters requires declaring them in the template's `inputs`, passing them via `arguments` on the DAG task, and referencing them via `{{inputs.parameters.x}}` inside the template body.

**Argo's three-template pattern for a diamond DAG:**

```yaml
# 5 templates required for a 4-task diamond
spec:
  entrypoint: diamond
  templates:
    - name: diamond                                # Template 1: DAG
      dag:
        tasks:
          - name: A
            template: echo                         # References template 5
            arguments:
              parameters: [{name: message, value: A}]
          - name: B
            dependencies: A
            template: echo
            arguments:
              parameters: [{name: message, value: B}]
          - name: C
            dependencies: A
            template: echo
            arguments:
              parameters: [{name: message, value: C}]
          - name: D
            dependencies: "B && C"
            template: echo
            arguments:
              parameters: [{name: message, value: D}]

    - name: echo                                   # Template 5: reused
      inputs:
        parameters:
          - name: message
      container:
        image: busybox
        command: [echo, "{{inputs.parameters.message}}"]
```

AgentWorkflow eliminates the indirection entirely. Each task *is* its own definition. There are no template references, no `arguments` blocks, no `inputs.parameters` declarations.

**The same diamond in AgentWorkflow:**

```yaml
spec:
  params:
    - name: proposal
      value: "Migrate the billing system to microservices"
  tasks:
    - name: technical-review
      agent:
        name: technical-advisor
        text: "Evaluate the technical feasibility: {{params.proposal}}"

    - name: cost-review
      agent:
        name: cost-analyst
        text: "Estimate costs and ROI: {{params.proposal}}"

    - name: risk-review
      agent:
        name: risk-assessor
        text: "Identify risks and mitigations: {{params.proposal}}"

    - name: synthesize
      agent:
        name: executive-summarizer
        text: |
          Synthesize these three reviews into a recommendation:
          Technical: {{tasks.technical-review.output}}
          Cost: {{tasks.cost-review.output}}
          Risk: {{tasks.risk-review.output}}
      dependencies: [technical-review, cost-review, risk-review]
```

The compiler generates a separate Argo template for each task behind the scenes, wires up the DAG task references, and maps the expression syntax to Argo's parameter system. The user writes 4 tasks; the compiler produces 5 templates (1 DAG + 4 task templates).

This also means there is no template reuse in AgentWorkflow. Every task is a standalone invocation. If you need the same agent called twice with different inputs, you write two tasks. This is a deliberate trade-off: agent workflows are orchestration graphs, not container pipelines. The unit of reuse is the deployed agent, not the workflow template.

---

## Principle 5: Simple JSONPath Expressions Instead of Argo's Reference System

Argo's output parameter references are verbose and require understanding the internal structure of templates, outputs, and parameter names:

```
{{tasks.research.outputs.parameters.result}}
{{workflow.parameters.topic}}
{{steps.generate-parameter.outputs.parameters.hello-param}}
```

You need to know:
- Whether you're in a `dag` (use `tasks.`) or `steps` (use `steps.`) context
- The exact output parameter name declared in the template
- That workflow parameters live under `workflow.parameters.`
- For structured data, you need Argo expression syntax with `sprig.fromJson()`

AgentWorkflow reduces this to three simple patterns:

| Expression | Meaning | Argo equivalent |
|---|---|---|
| `{{params.topic}}` | Workflow parameter | `{{workflow.parameters.topic}}` |
| `{{tasks.research.output}}` | Plain text output of a task | `{{tasks.research.outputs.parameters.result}}` |
| `{{tasks.research.output.score}}` | Field from structured output | `{{=sprig.fromJson(tasks['research'].outputs.parameters['artifact']).parts[0].data.score}}` |

The field-access form (`output.score`) automatically compiles to an Argo expression that parses the A2A artifact JSON and extracts the field from the first data part. The user doesn't need to know about A2A artifact structure, JSON parsing, or Argo's expression language.

**Example — passing structured data between tasks:**

```yaml
# AgentWorkflow: clean field access
spec:
  tasks:
    - name: research
      agentTask:
        type: run
        instruction:
          template: "Research key findings about {{params.topic}}"
        resultType:
          name: Findings
          jsonSchema:
            type: object
            properties:
              findings:
                type: array
                items:
                  type: object
                  properties:
                    title: { type: string }
                    summary: { type: string }

    - name: classify
      agentTask:
        type: classify
        input: "{{tasks.research.output}}"
        labels: ["theoretical", "applied", "benchmark"]
      dependencies: [research]

    - name: report
      agent:
        name: report-writer-agent
        text: "Generate report from: {{tasks.classify.output}}"
      dependencies: [classify]
```

The expression `{{tasks.research.output}}` becomes `{{tasks.research.outputs.parameters.result}}` in the compiled Argo template. If the user had written `{{tasks.research.output.findings}}`, the compiler would generate:

```
{{=sprig.fromJson(tasks['research'].outputs.parameters['artifact']).parts[0].data.findings}}
```

The user never needs to see or write that.

---

## Principle 6: Four Task Types Plus Routing Cover All Orchestration Needs

AgentWorkflow provides four task types plus a routing primitive. Two are agent-native (`agent`, `agentTask`), two are general-purpose (`container`, `http`), and one handles branching (`switch`). All five share the same DAG features: `dependencies`, `condition`, `timeout`, `retryStrategy`.

### `agent` — Call a Deployed Agent via A2A

The simplest case. Send a message to a running agent and get a response.

```yaml
- name: research
  agent:
    name: researcher-agent             # References an Agent CR
    text: "Find papers on {{params.topic}}"
```

For advanced cases, `text` can be replaced with a full `message` supporting multi-part content (text, structured data, files):

```yaml
- name: analyze
  agent:
    name: document-analyzer
    message:
      role: user
      contextId: "project-{{params.project}}"
      parts:
        - text:
            text: "Analyze this architecture document"
        - file:
            file:
              uri: "s3://docs/architecture.pdf"
              mimeType: "application/pdf"
        - data:
            data:
              constraints:
                budget: 500000
                timeline: "6 months"
```

### `agentTask` — Run an Ephemeral LLM Task

For stateless LLM operations that don't need a deployed agent. The task runs in a short-lived container using Marvin. Five operation types are supported: `run`, `classify`, `extract`, `cast`, `generate`.

```yaml
# Classify sentiment
- name: classify
  agentTask:
    type: classify
    input: "This product exceeded all my expectations!"
    labels: ["positive", "negative", "neutral"]

# Generate structured test data
- name: generate
  agentTask:
    type: generate
    instruction:
      instructionRef:
        name: test-generation-prompt
    resultType:
      name: TestCase
      jsonSchema:
        type: object
        properties:
          input: { type: string }
          expected: { type: string }
    count: 10
    model:
      name: gpt-4o
```

### `container` — Run an Arbitrary Container

For general-purpose compute that doesn't fit the agent or LLM task model: data preprocessing, format conversion, custom scripts, or any containerized workload. The syntax is flat — image, command, args, env — and the compiler handles template creation, output parameter wiring, and traceparent injection.

The container writes its output to `/tmp/result` (plain text) and optionally `/tmp/artifact` (structured JSON). The compiler wires these to output parameters automatically, making the result available to downstream tasks via `{{tasks.<name>.output}}`.

```yaml
# AgentWorkflow: flat container spec
- name: preprocess
  container:
    image: python:3.13-slim
    command: ["python", "-c"]
    args:
      - |
        import json
        data = {"cleaned": True, "records": 42}
        with open("/tmp/result", "w") as f:
            f.write(json.dumps(data))
    env:
      - name: INPUT_DATA
        value: "{{tasks.fetch.output}}"
  dependencies: [fetch]
  timeout: 5m
```

**What the user writes vs. what raw Argo requires:**

```yaml
# Raw Argo — what this compiles to
spec:
  templates:
    - name: main
      dag:
        tasks:
          - name: preprocess
            template: preprocess
            dependencies: [fetch]

    - name: preprocess
      activeDeadlineSeconds: 300
      container:
        image: python:3.13-slim
        command: ["python", "-c"]
        args:
          - |
            import json
            data = {"cleaned": True, "records": 42}
            with open("/tmp/result", "w") as f:
                f.write(json.dumps(data))
        env:
          - name: INPUT_DATA
            value: "{{tasks.fetch.outputs.parameters.result}}"
          - name: FLOKOA_TRACEPARENT
            value: "{{workflow.parameters._flokoa_traceparent}}"
      outputs:
        parameters:
          - name: result
            valueFrom:
              path: /tmp/result
          - name: artifact
            valueFrom:
              path: /tmp/artifact
              default: "{}"
```

The user never declares output parameters, valueFrom paths, or the traceparent env var. They write a container spec; the compiler generates the template with full I/O wiring.

**Container with volumes and resources:**

```yaml
- name: train-model
  container:
    image: ghcr.io/myorg/trainer:v2
    command: ["python", "train.py"]
    args: ["--epochs=10", "--output=/tmp/result"]
    env:
      - name: DATASET
        value: "{{tasks.prepare-data.output}}"
      - name: MODEL_NAME
        valueFrom:
          secretKeyRef:
            name: training-secrets
            key: model-name
    resources:
      requests:
        memory: "4Gi"
        cpu: "2"
      limits:
        memory: "8Gi"
  dependencies: [prepare-data]
  timeout: 30m
  retryStrategy:
    limit: 2
    backoff:
      duration: "1m"
      factor: 2
```

### `http` — Make an HTTP Request

For calling external APIs, webhooks, or any HTTP endpoint. The DSL provides a clean request spec with method, URL, headers, and body. The compiler translates this into an Argo HTTP template, parses the response into output parameters, and handles expression translation in all fields.

The HTTP response body is available as `{{tasks.<name>.output}}` and the full response (status code, headers, body) as `{{tasks.<name>.artifact}}`.

```yaml
# AgentWorkflow: flat HTTP spec
- name: fetch-data
  http:
    url: "https://api.example.com/data/{{params.dataset_id}}"
    method: GET
    headers:
      - name: Accept
        value: "application/json"
      - name: Authorization
        valueFrom:
          secretKeyRef:
            name: api-credentials
            key: token
    successCondition: "response.statusCode >= 200 && response.statusCode < 300"
  timeout: 2m
```

**What the user writes vs. what raw Argo requires:**

```yaml
# Raw Argo — what this compiles to
spec:
  templates:
    - name: main
      dag:
        tasks:
          - name: fetch-data
            template: fetch-data

    - name: fetch-data
      activeDeadlineSeconds: 120
      http:
        url: "https://api.example.com/data/{{workflow.parameters.dataset_id}}"
        method: GET
        headers:
          - name: Accept
            value: "application/json"
          - name: Authorization
            valueFrom:
              secretKeyRef:
                name: api-credentials
                key: token
        successCondition: "response.statusCode >= 200 && response.statusCode < 300"
      outputs:
        parameters:
          - name: result
            valueFrom:
              expression: "response.body"
          - name: artifact
            valueFrom:
              expression: "toJson({statusCode: response.statusCode, headers: response.headers, body: response.body})"
```

Headers use the same `name`/`value`/`valueFrom` pattern as Kubernetes `env` vars. Three forms are supported:

```yaml
headers:
  # Inline value
  - name: Accept
    value: "application/json"

  # Secret reference (compiled to Argo's native HTTPHeader.ValueFrom.SecretKeyRef)
  - name: Authorization
    valueFrom:
      secretKeyRef:
        name: api-credentials
        key: token

  # ConfigMap reference (compiled to a workflow parameter with valueFrom.configMapKeyRef,
  # then referenced in the header value)
  - name: X-Api-Version
    valueFrom:
      configMapKeyRef:
        name: api-config
        key: version
```

`secretKeyRef` maps directly to Argo's native HTTP header secret support — no intermediate parameters needed. `configMapKeyRef` is not natively supported by Argo HTTP headers, so the compiler creates a workflow parameter sourced from the ConfigMap and injects the resolved value into the header. Expressions in the URL, headers, and body are translated from DSL syntax to Argo syntax. Output parameters are wired automatically.

**HTTP POST with body and chained output:**

```yaml
- name: trigger-pipeline
  http:
    url: "https://ci.example.com/api/pipelines"
    method: POST
    headers:
      - name: Content-Type
        value: "application/json"
      - name: X-Token
        valueFrom:
          secretKeyRef:
            name: ci-secrets
            key: token
    body: |
      {
        "ref": "main",
        "variables": {
          "REPORT": "{{tasks.generate-report.output}}"
        }
      }
    successCondition: "response.statusCode == 201"
  dependencies: [generate-report]
  retryStrategy:
    limit: 3
    backoff:
      duration: "10s"
      factor: 2

- name: notify
  agent:
    name: notification-agent
    text: "Pipeline triggered: {{tasks.trigger-pipeline.output}}"
  dependencies: [trigger-pipeline]
```

### `switch` — Conditional Routing

Routes execution to different tasks based on prior task outputs:

```yaml
- name: route
  switch:
    - condition: "{{tasks.classify.output}} == positive"
      then: celebrate
    - condition: "{{tasks.classify.output}} == negative"
      then: escalate
    - default: review
  dependencies: [classify]
```

The compiler translates each branch into a separate Argo DAG task with a `when` expression, all depending on the switch task.

### Uniform DAG Features Across All Task Types

Every task type participates in the same DAG execution model. The following features work identically regardless of whether the task is an `agent`, `agentTask`, `container`, `http`, or `switch`:

| Feature | Description | Example |
|---|---|---|
| `dependencies` | Declares execution order | `dependencies: [step-a, step-b]` |
| `condition` | Conditional execution | `condition: "{{tasks.check.output}} == proceed"` |
| `timeout` | Per-task time limit | `timeout: 10m` |
| `retryStrategy` | Retry on failure | `retryStrategy: {limit: 3, backoff: {duration: "30s"}}` |
| `switch` | Multi-way routing after any task | Routes to different downstream tasks |
| Output references | Downstream tasks reference outputs | `{{tasks.<name>.output}}` / `{{tasks.<name>.output.field}}` |

**Example — mixed task types in a single workflow:**

```yaml
spec:
  params:
    - name: api_url
      value: "https://data.example.com/v2"
  tasks:
    # HTTP: fetch raw data from external API
    - name: fetch
      http:
        url: "{{params.api_url}}/records"
        method: GET
        headers:
          - name: Accept
            value: "application/json"
          - name: Authorization
            valueFrom:
              secretKeyRef:
                name: data-api-credentials
                key: token
      timeout: 2m

    # Container: preprocess the raw data
    - name: preprocess
      container:
        image: python:3.13-slim
        command: ["python", "preprocess.py"]
        env:
          - name: RAW_DATA
            value: "{{tasks.fetch.output}}"
      dependencies: [fetch]
      timeout: 5m

    # AgentTask: classify the preprocessed data
    - name: classify
      agentTask:
        type: classify
        input: "{{tasks.preprocess.output}}"
        labels: ["actionable", "informational", "noise"]
      dependencies: [preprocess]

    # Switch: route based on classification
    - name: route
      switch:
        - condition: "{{tasks.classify.output}} == actionable"
          then: act
        - condition: "{{tasks.classify.output}} == informational"
          then: archive
        - default: discard
      dependencies: [classify]

    # Agent: take action on actionable items
    - name: act
      agent:
        name: action-agent
        text: "Process this actionable item: {{tasks.preprocess.output}}"

    # HTTP: archive informational items
    - name: archive
      http:
        url: "{{params.api_url}}/archive"
        method: POST
        headers:
          - name: Content-Type
            value: "application/json"
          - name: Authorization
            valueFrom:
              secretKeyRef:
                name: data-api-credentials
                key: token
        body: '{"data": "{{tasks.preprocess.output}}", "label": "informational"}'

    # Container: log discarded items
    - name: discard
      container:
        image: alpine:3.20
        command: ["sh", "-c"]
        args: ["echo 'Discarded: {{tasks.preprocess.output}}' >> /tmp/result"]
```

This workflow chains HTTP, container, agentTask, switch, agent, and more HTTP/container tasks together. Every task uses the same `dependencies` and expression syntax. The compiler handles the template type differences.

---

## Artifact I/O Mode

By default, AgentWorkflow compiles task inputs and outputs as Argo **parameters** — lightweight string values stored in the Workflow CR. This works well for text results and small structured data, but parameters are stored in etcd (subject to the ~1 MB Workflow CR size limit) and are not suited for large payloads or binary files.

**Artifact I/O mode** is an operator-level opt-in that switches all task I/O to Argo **artifacts** backed by object storage. When enabled, the compiler emits artifacts instead of parameters, with automatic garbage collection on workflow completion. The DSL syntax does not change — `{{tasks.<name>.output}}` and `{{tasks.<name>.output.field}}` work exactly the same way. Only the compilation target changes.

### Enabling Artifact I/O

This is an operator/Helm chart level setting, not a per-workflow option. It applies to all AgentWorkflow compilations in the cluster:

```yaml
# values.yaml (Helm chart)
agentWorkflow:
  artifactIO:
    enabled: false                        # Default: parameters mode
    artifactGC:
      strategy: OnWorkflowCompletion      # Clean up artifacts when the workflow finishes
```

When `artifactIO.enabled: true`, the compiler changes how task outputs are wired:

### Default Mode (Parameters)

```yaml
# Compiled Argo template — parameter outputs (default)
- name: research
  plugin:
    a2a: { ... }
  outputs:
    parameters:
      - name: result
      - name: artifact
```

```yaml
# Compiled Argo template — container outputs (default)
- name: preprocess
  container: { ... }
  outputs:
    parameters:
      - name: result
        valueFrom:
          path: /tmp/result
      - name: artifact
        valueFrom:
          path: /tmp/artifact
          default: "{}"
```

### Artifact I/O Mode

```yaml
# Compiled Argo — workflow-level artifact GC
spec:
  artifactGC:
    strategy: OnWorkflowCompletion

# Compiled Argo template — A2A plugin outputs (artifact mode)
# The A2A plugin writes result/artifact to files; outputs are artifacts
- name: research
  plugin:
    a2a: { ... }
  outputs:
    artifacts:
      - name: result
        path: /tmp/result
        artifactGC:
          strategy: OnWorkflowCompletion
      - name: artifact
        path: /tmp/artifact
        artifactGC:
          strategy: OnWorkflowCompletion
```

```yaml
# Compiled Argo template — container outputs (artifact mode)
- name: preprocess
  container: { ... }
  outputs:
    artifacts:
      - name: result
        path: /tmp/result
        artifactGC:
          strategy: OnWorkflowCompletion
      - name: artifact
        path: /tmp/artifact
        optional: true
        artifactGC:
          strategy: OnWorkflowCompletion
```

```yaml
# Compiled Argo template — HTTP outputs (artifact mode)
# The compiler wraps the HTTP response into a file via a script template
- name: fetch-data
  http: { ... }
  outputs:
    artifacts:
      - name: result
        path: /tmp/result
        artifactGC:
          strategy: OnWorkflowCompletion
      - name: artifact
        path: /tmp/artifact
        artifactGC:
          strategy: OnWorkflowCompletion
```

### Expression Translation in Artifact Mode

In parameter mode, `{{tasks.research.output}}` compiles to `{{tasks.research.outputs.parameters.result}}`. In artifact mode, the compiler adjusts references to load from artifacts instead. Argo supports reading artifact contents inline via `{{tasks.research.outputs.artifacts.result}}` when the artifact is small enough, or the compiler can use expression syntax to parse artifact content for field access.

The key guarantee: **the DSL expressions are identical in both modes.** The user writes `{{tasks.research.output}}` regardless of whether the operator is configured for parameter or artifact I/O. The compiler handles the translation.

### When to Enable Artifact I/O

| Scenario | Recommended mode |
|---|---|
| Short text results, small JSON payloads | Parameters (default) |
| Large LLM outputs, documents, multi-KB structured data | Artifact I/O |
| Workflows that produce files (reports, datasets) | Artifact I/O |
| Compliance requirement to not store data in etcd | Artifact I/O |
| Minimal infrastructure (no object storage configured) | Parameters (default) |

Artifact I/O requires an artifact repository (S3, GCS, Azure Blob, or Minio) configured in the Argo Workflows controller. The Helm chart setting tells the Flokoa operator to compile workflows for artifact-based I/O; the actual storage backend is configured on the Argo side.

---

## Summary: What the Compiler Does So You Don't Have To

| Concern | AgentWorkflow (you write) | Argo (compiler generates) |
|---|---|---|
| Entrypoint | Implicit | `entrypoint: main` with DAG template |
| Templates | None (tasks are inline) | One template per task + one DAG template |
| Output parameters | Implicit for all task types | `outputs.parameters` with `result` and `artifact` |
| Parameter references | `{{params.x}}` | `{{workflow.parameters.x}}` |
| Task output references | `{{tasks.y.output}}` | `{{tasks.y.outputs.parameters.result}}` |
| Structured field access | `{{tasks.y.output.field}}` | `{{=sprig.fromJson(tasks['y'].outputs.parameters['artifact']).parts[0].data.field}}` |
| Traceparent propagation | Invisible | Injected as workflow parameter + env var (all task types) |
| A2A plugin wiring | `agent.name` + `text` | Full plugin JSON with message structure |
| AgentTask configuration | `agentTask.type` + fields | Container spec with image, env vars, volume mounts |
| Container wiring | `container.image` + `command` | Template with output valueFrom paths, default artifact |
| HTTP wiring | `http.url` + `method` | HTTP template with `[]HTTPHeader`, response expression outputs |
| HTTP headers | `[]Header` with `value` / `valueFrom.secretKeyRef` / `valueFrom.configMapKeyRef` | Argo `[]HTTPHeader` with name/value/valueFrom; configMapKeyRef via workflow parameter |
| Model/tool resolution | `model.name: gpt-4o` | ConfigMap volumes, secret env vars, provider config |
| Retry with backoff | `retryStrategy.limit: 3` | Argo `retryStrategy` with `intstr` limit and backoff |
| Timeout | `timeout: 10m` | `activeDeadlineSeconds: 600` |
| Switch routing | `switch` with conditions | Multiple DAG tasks with `when` expressions |
| DAG features | Uniform across all task types | `dependencies`, `condition`, `timeout`, `retryStrategy` |
| Artifact I/O mode | Invisible (operator-level Helm setting) | `outputs.artifacts` with `artifactGC.strategy: OnWorkflowCompletion`, workflow-level `artifactGC` |
