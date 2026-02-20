# AgentWorkflow DSL Design Principles

The AgentWorkflow CRD is a purpose-built DSL for orchestrating AI agent tasks on Kubernetes. It compiles down to Argo Workflows but deliberately hides the mechanical complexity of Argo's execution model. This document defines the design principles that govern the DSL, with side-by-side comparisons showing what the user writes versus what Argo requires.

---

## Principle 1: Use Argo Keywords Where They Make Sense

The DSL borrows terminology from Argo Workflows wherever the concept maps cleanly to agent orchestration. Users who know Argo should feel at home; users who don't shouldn't need to learn Argo to use it.

**Borrowed keywords:**

| AgentWorkflow keyword | Argo equivalent | Purpose |
|---|---|---|
| `tasks` | `dag.tasks` | Define the units of work |
| `dependsOn` | `dag.tasks[].depends` | Declare execution order |
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
      dependsOn: [research]
```

The `tasks`, `dependsOn`, `params`, and `timeout` keywords are immediately recognizable to anyone familiar with Argo. The key difference: in Argo, `tasks` only appears nested under a `dag` template; in AgentWorkflow, `tasks` is the top-level spec because every workflow is an agent DAG by default.

---

## Principle 2: A Simplified Argo for a Specialized Use Case

AgentWorkflow supports exactly two kinds of work: calling a deployed agent via A2A (`agent`) and running an ephemeral LLM task (`agentTask`). There are no arbitrary containers, scripts, or resource operations. This narrow scope allows the DSL to drop most of Argo's generality.

**What the DSL keeps:**
- DAG-based task orchestration with `dependsOn`
- Workflow-level `params` with default values
- Per-task `timeout` and `retryStrategy` (with backoff)
- Conditional execution via `condition` and `switch`
- Expression-based data passing between tasks

**What the DSL drops:**
- Steps-based sequential workflows (unnecessary — `dependsOn` chains express sequences)
- Container, script, resource, suspend, and HTTP template types
- Artifacts (inputs/outputs are text or structured JSON, not files on a volume)
- `withItems` / `withParam` loops
- `exitHandler`, `onExit`, `hooks`
- Volumes, sidecars, init containers, daemon tasks
- Memoization, synchronization, mutex/semaphore
- Node selectors, tolerations, pod metadata

This is not a limitation. Agent workflows don't schedule containers — they send messages to deployed agents or run short-lived LLM function calls. The eliminated features don't apply to this execution model.

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
      dependsOn: [A]
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
            depends: A
            template: echo
            arguments:
              parameters: [{name: message, value: B}]
          - name: C
            depends: A
            template: echo
            arguments:
              parameters: [{name: message, value: C}]
          - name: D
            depends: "B && C"
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
      dependsOn: [technical-review, cost-review, risk-review]
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
      dependsOn: [research]

    - name: report
      agent:
        name: report-writer-agent
        text: "Generate report from: {{tasks.classify.output}}"
      dependsOn: [classify]
```

The expression `{{tasks.research.output}}` becomes `{{tasks.research.outputs.parameters.result}}` in the compiled Argo template. If the user had written `{{tasks.research.output.findings}}`, the compiler would generate:

```
{{=sprig.fromJson(tasks['research'].outputs.parameters['artifact']).parts[0].data.findings}}
```

The user never needs to see or write that.

---

## Principle 6: Two Task Types Cover All Agent Orchestration Needs

Rather than Argo's open-ended template type system (container, script, resource, suspend, HTTP, plugin, DAG, steps), AgentWorkflow provides exactly two task types plus a routing primitive:

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
  dependsOn: [classify]
```

The compiler translates each branch into a separate Argo DAG task with a `when` expression, all depending on the switch task.

---

## Summary: What the Compiler Does So You Don't Have To

| Concern | AgentWorkflow (you write) | Argo (compiler generates) |
|---|---|---|
| Entrypoint | Implicit | `entrypoint: main` with DAG template |
| Templates | None (tasks are inline) | One template per task + one DAG template |
| Output parameters | Implicit | `outputs.parameters` with `result` and `artifact` |
| Parameter references | `{{params.x}}` | `{{workflow.parameters.x}}` |
| Task output references | `{{tasks.y.output}}` | `{{tasks.y.outputs.parameters.result}}` |
| Structured field access | `{{tasks.y.output.field}}` | `{{=sprig.fromJson(tasks['y'].outputs.parameters['artifact']).parts[0].data.field}}` |
| Traceparent propagation | Invisible | Injected as workflow parameter + env var |
| A2A plugin wiring | `agent.name` + `text` | Full plugin JSON with message structure |
| Container configuration | `agentTask.type` + fields | Container spec with image, env vars, volume mounts |
| Model/tool resolution | `model.name: gpt-4o` | ConfigMap volumes, secret env vars, provider config |
| Retry with backoff | `retryStrategy.limit: 3` | Argo `retryStrategy` with `intstr` limit and backoff |
| Timeout | `timeout: 10m` | `activeDeadlineSeconds: 600` |
| Switch routing | `switch` with conditions | Multiple DAG tasks with `when` expressions |
