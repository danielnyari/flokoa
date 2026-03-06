# AgentWorkflow Resource

The `AgentWorkflow` resource defines declarative, multi-agent workflows that are compiled into Argo WorkflowTemplates for execution in your Kubernetes cluster.

## Overview

An AgentWorkflow defines:
- A directed acyclic graph (DAG) of tasks to execute
- Task types including agent calls (A2A), Marvin-powered tasks, containers, HTTP requests, and conditional switches
- Workflow-level parameters that can be referenced in expressions
- Timeout and retry strategies at both workflow and task levels
- Automatic compilation to Argo WorkflowTemplates

AgentWorkflows bridge the gap between high-level agent orchestration and Argo Workflows execution. You declare what your workflow should do using agent-native concepts, and the operator compiles it into a fully functional Argo WorkflowTemplate.

## Basic Structure

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentWorkflow
metadata:
  name: my-workflow
spec:
  description: "A simple workflow that calls an agent"
  tasks:
    - name: ask-agent
      agent:
        name: my-agent
        text: "Hello, what can you help me with?"
```

## Spec Fields

### description

A human-readable description of what the workflow does.

```yaml
spec:
  description: "Research and summarize a topic using multiple agents"
```

### params

Workflow-level parameters that can be referenced in task expressions using `{{params.<name>}}`. Parameters without a default value must be provided at workflow submission time.

```yaml
spec:
  params:
    - name: topic
      description: "The topic to research"
      value: "kubernetes"  # Default value (optional)
    - name: depth
      description: "How deep to research"
      # No default - must be provided at submission
```

### tasks

The list of tasks to execute. At least one task is required. Each task must have a unique name matching the pattern `^[a-zA-Z0-9][-a-zA-Z0-9]*$`. Exactly one task type (agent, agentTask, container, http, or switch) must be specified per task.

```yaml
spec:
  tasks:
    - name: step-1
      agent:
        name: researcher
        text: "Research {{params.topic}}"
    - name: step-2
      dependsOn:
        - step-1
      agent:
        name: summarizer
        text: "Summarize: {{tasks.step-1.output}}"
```

### timeout

Maximum duration for the entire workflow. Uses Go duration format (e.g., `30m`, `2h`, `1h30m`).

```yaml
spec:
  timeout: 1h
```

### retryStrategy

Default retry policy applied to all tasks. Individual tasks can override this with their own retryStrategy.

```yaml
spec:
  retryStrategy:
    limit: 3
    backoff:
      duration: "30s"
      factor: 2
```

| Field | Type | Description |
|-------|------|-------------|
| `limit` | int32 | Maximum number of retry attempts (minimum: 0) |
| `backoff.duration` | string | Initial backoff duration (e.g., `"30s"`, `"5m"`) |
| `backoff.factor` | int32 | Multiplier applied after each retry (optional) |

### serviceAccountName

The Kubernetes ServiceAccount used for workflow pods. Defaults to `flokoa-workflow`.

```yaml
spec:
  serviceAccountName: my-workflow-sa
```

### automountServiceAccountToken

Controls whether the service account token is automatically mounted into workflow pods. Defaults to `true`. Argo executor plugins require this to be `true`.

```yaml
spec:
  automountServiceAccountToken: true
```

## Task Types

Each task must specify exactly one of the following types.

### agent

Calls a deployed Agent CR via the A2A protocol. Use `text` for simple messages or `message` for full A2A messages with multi-part content.

```yaml
tasks:
  - name: call-agent
    agent:
      name: my-agent                    # Agent CR name (required)
      namespace: agents                 # Agent namespace (optional)
      text: "Analyze {{params.topic}}"  # Simple text message
```

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Name of the Agent CR to call (required) |
| `namespace` | string | Namespace of the Agent CR (defaults to workflow namespace) |
| `text` | string | Simple text message; supports expressions (mutually exclusive with `message`) |
| `message` | AgentMessage | Full A2A message with parts, context, and metadata (mutually exclusive with `text`) |
| `config` | MessageSendConfig | Controls output modes, blocking behavior, and history length |

For advanced use cases, use the `message` field to send multi-part A2A messages:

```yaml
tasks:
  - name: advanced-call
    agent:
      name: my-agent
      message:
        role: user
        parts:
          - text:
              text: "Analyze this data"
          - data:
              data: '{"key": "value"}'
      config:
        acceptedOutputModes:
          - "application/json"
        blocking: true
```

### agentTask

Runs a Marvin-powered task in an ephemeral container. Supports operations like `run`, `classify`, `extract`, `cast`, and `generate`.

```yaml
tasks:
  - name: classify-input
    agentTask:
      type: classify
      input: "{{tasks.gather-data.output}}"
      labels:
        - positive
        - negative
        - neutral
      model:
        name: gpt-4o-model
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | enum | Marvin operation: `run`, `classify`, `extract`, `cast`, `generate` (required) |
| `instruction` | InstructionEntry | Prompt or guidance (inline or reference) |
| `input` | string | Data to process; supports expressions |
| `resultType` | StructuredIOSchema | JSON Schema constraining the output type |
| `labels` | []string | Classification labels (required for `classify`) |
| `multiLabel` | bool | Enable multi-label classification |
| `count` | int32 | Number of items to generate (for `generate`, minimum: 1) |
| `model` | AgentModelRef | Reference to a Model CR |
| `tools` | []ToolEntry | Tools available to the task |
| `context` | map[string]string | Key-value data passed to the task; values support expressions |
| `image` | string | Override the container image |
| `env` | []EnvVar | Additional environment variables |
| `resources` | ResourceRequirements | Compute resource requirements |

### container

Runs an arbitrary container workload. The container should write its result to `/tmp/result` (plain text) and optional structured output to `/tmp/artifact` (JSON).

```yaml
tasks:
  - name: process-data
    container:
      image: python:3.13-slim
      command: ["python"]
      args: ["-c", "print('processed')"]
      env:
        - name: INPUT
          value: "{{tasks.fetch.output}}"
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
        limits:
          cpu: "500m"
          memory: "512Mi"
```

| Field | Type | Description |
|-------|------|-------------|
| `image` | string | Container image to run (required) |
| `command` | []string | Entrypoint command |
| `args` | []string | Arguments to the command |
| `env` | []EnvVar | Environment variables; values support expressions |
| `resources` | ResourceRequirements | CPU and memory requirements |
| `workingDir` | string | Working directory inside the container |
| `volumeMounts` | []VolumeMount | Volume mounts for the container |

### http

Makes an HTTP request to an external service. The response body is captured as the task output.

```yaml
tasks:
  - name: fetch-data
    http:
      url: "https://api.example.com/data?q={{params.query}}"
      method: GET
      headers:
        - name: Authorization
          valueFrom:
            secretKeyRef:
              name: api-credentials
              key: token
      successCondition: "response.statusCode == 200"
```

| Field | Type | Description |
|-------|------|-------------|
| `url` | string | Request URL; supports expressions (required) |
| `method` | enum | HTTP method: `GET`, `POST`, `PUT`, `DELETE`, `PATCH`, `HEAD`, `OPTIONS` (default: `GET`) |
| `headers` | []HTTPHeader | HTTP headers (value can be inline or from Secret/ConfigMap) |
| `body` | string | Request body; supports expressions |
| `successCondition` | string | Argo expression to determine success (e.g., `"response.statusCode == 200"`) |

### switch

Routes to different tasks based on conditions evaluated against previous task outputs. Include a default case as a fallback.

```yaml
tasks:
  - name: route
    dependsOn:
      - classify
    switch:
      - condition: "'{{tasks.classify.output}}' == 'positive'"
        then: handle-positive
      - condition: "'{{tasks.classify.output}}' == 'negative'"
        then: handle-negative
      - default: handle-neutral
```

| Field | Type | Description |
|-------|------|-------------|
| `condition` | string | Expression to evaluate |
| `then` | string | Task name to run if condition is true |
| `default` | string | Fallback task name if no condition matches |

## Task-Level Fields

In addition to a task type, each task supports:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique task identifier (required, pattern: `^[a-zA-Z0-9][-a-zA-Z0-9]*$`) |
| `dependsOn` | []string | Task names that must complete before this task starts |
| `timeout` | Duration | Maximum duration for this task |
| `retryStrategy` | WorkflowRetryStrategy | Override the workflow-level retry policy |
| `condition` | string | Expression that must evaluate to true for the task to run; if false, the task is skipped |

## Expressions and Parameters

AgentWorkflows support expressions for dynamic values throughout task definitions. Expressions use the `{{...}}` syntax.

### Parameter References

Reference workflow parameters defined in `spec.params`:

```yaml
spec:
  params:
    - name: topic
      value: "kubernetes"
  tasks:
    - name: research
      agent:
        name: researcher
        text: "Research the topic: {{params.topic}}"
```

### Task Output References

Reference the output of a previously completed task:

```yaml
tasks:
  - name: step-1
    agent:
      name: researcher
      text: "Research kubernetes"
  - name: step-2
    dependsOn:
      - step-1
    agent:
      name: writer
      text: "Write an article based on: {{tasks.step-1.output}}"
```

### Expression Locations

Expressions can be used in:
- `agent.text` - Agent text messages
- `agent.message.parts[].text.text` - A2A message text parts
- `agentTask.input` - Marvin task input
- `agentTask.context` values - Marvin task context
- `container.env[].value` - Container environment variables
- `http.url` - HTTP request URL
- `http.body` - HTTP request body
- `http.headers[].value` - HTTP header values
- `switch[].condition` - Switch conditions
- `condition` - Task-level conditions

## Status Fields

The operator updates these fields automatically:

```yaml
status:
  ready: true
  workflowTemplateName: my-workflow-template
  specHash: "a1b2c3d4"
  observedGeneration: 1

  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2026-01-15T10:30:00Z"
      reason: WorkflowTemplateReady
      message: "Argo WorkflowTemplate compiled and applied"
```

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Whether the WorkflowTemplate has been compiled and applied |
| `workflowTemplateName` | string | Name of the generated Argo WorkflowTemplate CR |
| `specHash` | string | Hash of the spec for drift detection; recompilation is skipped if unchanged |
| `conditions` | []Condition | Standard Kubernetes conditions |
| `observedGeneration` | int64 | Most recent generation observed by the controller |

Print columns: `Ready`, `Template` (workflowTemplateName), `Age`. Short name: `awf`.

## Examples

### Simple Sequential Workflow

Two tasks running in sequence, where the second depends on the first:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentWorkflow
metadata:
  name: simple-sequential
spec:
  description: "Research a topic and then summarize the findings"
  tasks:
    - name: research
      agent:
        name: researcher-agent
        text: "Research the latest developments in container orchestration"
    - name: summarize
      dependsOn:
        - research
      agent:
        name: summarizer-agent
        text: "Summarize the following research: {{tasks.research.output}}"
```

### Parallel Tasks with Dependencies

Multiple tasks running in parallel, followed by a task that depends on all of them:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentWorkflow
metadata:
  name: parallel-research
spec:
  description: "Research multiple topics in parallel and combine results"
  timeout: 30m
  params:
    - name: topic-1
      value: "kubernetes security"
    - name: topic-2
      value: "kubernetes networking"
    - name: topic-3
      value: "kubernetes storage"
  tasks:
    - name: research-security
      agent:
        name: researcher-agent
        text: "Research {{params.topic-1}}"
    - name: research-networking
      agent:
        name: researcher-agent
        text: "Research {{params.topic-2}}"
    - name: research-storage
      agent:
        name: researcher-agent
        text: "Research {{params.topic-3}}"
    - name: combine-results
      dependsOn:
        - research-security
        - research-networking
        - research-storage
      agent:
        name: writer-agent
        text: >-
          Combine these research findings into a comprehensive report:
          Security: {{tasks.research-security.output}}
          Networking: {{tasks.research-networking.output}}
          Storage: {{tasks.research-storage.output}}
```

### Conditional Workflow with Switch

Use classification to route to different agents based on input:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentWorkflow
metadata:
  name: conditional-routing
spec:
  description: "Classify a request and route to the appropriate handler"
  params:
    - name: user-request
  tasks:
    - name: classify
      agentTask:
        type: classify
        input: "{{params.user-request}}"
        labels:
          - billing
          - technical
          - general
        model:
          name: gpt-4o-model
    - name: route
      dependsOn:
        - classify
      switch:
        - condition: "'{{tasks.classify.output}}' == 'billing'"
          then: handle-billing
        - condition: "'{{tasks.classify.output}}' == 'technical'"
          then: handle-technical
        - default: handle-general
    - name: handle-billing
      dependsOn:
        - route
      agent:
        name: billing-agent
        text: "Handle this billing request: {{params.user-request}}"
    - name: handle-technical
      dependsOn:
        - route
      agent:
        name: technical-agent
        text: "Handle this technical request: {{params.user-request}}"
    - name: handle-general
      dependsOn:
        - route
      agent:
        name: general-agent
        text: "Handle this general request: {{params.user-request}}"
```

### Parameterized Workflow with Retry

A workflow with parameters, timeouts, and retry strategies:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentWorkflow
metadata:
  name: robust-pipeline
spec:
  description: "A robust data processing pipeline with retries"
  timeout: 2h
  retryStrategy:
    limit: 2
    backoff:
      duration: "1m"
      factor: 2
  params:
    - name: data-url
      description: "URL of the data source to process"
    - name: output-format
      description: "Desired output format"
      value: "json"
  tasks:
    - name: fetch-data
      timeout: 5m
      retryStrategy:
        limit: 3
        backoff:
          duration: "10s"
          factor: 2
      http:
        url: "{{params.data-url}}"
        method: GET
        successCondition: "response.statusCode == 200"
    - name: process-data
      dependsOn:
        - fetch-data
      timeout: 30m
      agent:
        name: data-processor
        text: >-
          Process the following data and return results in
          {{params.output-format}} format: {{tasks.fetch-data.output}}
    - name: validate-output
      dependsOn:
        - process-data
      container:
        image: python:3.13-slim
        command: ["python", "-c"]
        args:
          - |
            import json, sys
            data = '''{{tasks.process-data.output}}'''
            try:
                json.loads(data)
                with open('/tmp/result', 'w') as f:
                    f.write('valid')
            except Exception as e:
                with open('/tmp/result', 'w') as f:
                    f.write(f'invalid: {e}')
                sys.exit(1)
```

### Mixed Task Types

A workflow combining agent calls, HTTP requests, containers, and Marvin tasks:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentWorkflow
metadata:
  name: mixed-pipeline
spec:
  description: "Pipeline using multiple task types"
  params:
    - name: query
      description: "Search query"
  tasks:
    - name: search-api
      http:
        url: "https://api.example.com/search?q={{params.query}}"
        method: GET
        headers:
          - name: Accept
            value: "application/json"
        successCondition: "response.statusCode == 200"
    - name: extract-entities
      dependsOn:
        - search-api
      agentTask:
        type: extract
        input: "{{tasks.search-api.output}}"
        resultType:
          schema:
            type: object
            properties:
              entities:
                type: array
                items:
                  type: string
        model:
          name: gpt-4o-model
    - name: analyze
      dependsOn:
        - extract-entities
      agent:
        name: analyst-agent
        text: "Analyze these entities: {{tasks.extract-entities.output}}"
    - name: generate-report
      dependsOn:
        - analyze
      container:
        image: ghcr.io/example/report-generator:v1.0.0
        env:
          - name: ANALYSIS
            value: "{{tasks.analyze.output}}"
          - name: FORMAT
            value: "pdf"
```

## Operations

### Viewing AgentWorkflows

```bash
# List all workflows (short name: awf)
kubectl get awf

# Get detailed information
kubectl describe awf my-workflow

# Check compilation status
kubectl get awf my-workflow -o jsonpath='{.status.ready}'

# View the generated Argo WorkflowTemplate
kubectl get awf my-workflow -o jsonpath='{.status.workflowTemplateName}'
```

### Running a Workflow

AgentWorkflows compile to Argo WorkflowTemplates. To run a workflow, submit it through Argo:

```bash
# Submit a workflow from the template
argo submit --from workflowtemplate/my-workflow

# Submit with parameter overrides
argo submit --from workflowtemplate/my-workflow \
  -p topic="artificial intelligence" \
  -p depth="detailed"

# Watch workflow execution
argo watch @latest

# View workflow logs
argo logs @latest
```

### Updating a Workflow

When you update an AgentWorkflow, the operator recompiles the Argo WorkflowTemplate automatically:

```bash
# Edit the workflow
kubectl edit awf my-workflow

# Or apply an updated manifest
kubectl apply -f my-workflow.yaml

# Verify recompilation
kubectl get awf my-workflow
```

### Deleting a Workflow

```bash
# Delete the workflow (also removes the generated WorkflowTemplate)
kubectl delete awf my-workflow
```

## Best Practices

1. **Use descriptive task names** that clearly indicate what each task does
2. **Set timeouts** at both workflow and task levels to prevent runaway executions
3. **Configure retry strategies** for tasks that call external services or APIs
4. **Use parameters** for values that change between runs instead of hardcoding
5. **Keep workflows focused** on a single logical process; split large workflows into smaller ones
6. **Add conditions** to skip tasks that are not needed based on prior outputs
7. **Use `dependsOn`** to explicitly declare task dependencies for correct execution order
8. **Prefer `text` over `message`** for simple agent calls to keep workflows readable
9. **Write container outputs to `/tmp/result`** so downstream tasks can reference them
10. **Version your workflow manifests** in Git alongside your agent definitions

## Troubleshooting

### Workflow Not Ready

```bash
# Check workflow status and conditions
kubectl describe awf <name>

# Common issues:
# - Invalid task names (must match ^[a-zA-Z0-9][-a-zA-Z0-9]*$)
# - Missing required fields (task name, at least one task type per task)
# - Circular dependencies in dependsOn
# - Referenced agents do not exist
```

### WorkflowTemplate Not Created

- Verify Argo Workflows is installed and running: `kubectl get pods -n argo`
- Check operator logs for compilation errors: `kubectl logs -l app.kubernetes.io/name=flokoa-operator`
- Ensure the operator has RBAC permissions for Argo WorkflowTemplate resources

### Tasks Failing at Runtime

```bash
# View Argo Workflow status
argo list
argo get <workflow-name>

# Check task logs
argo logs <workflow-name> <task-name>

# Common issues:
# - Agent not reachable (check Agent CR status and service)
# - Expression syntax errors in {{params.*}} or {{tasks.*}}
# - HTTP tasks returning non-success status codes
# - Container tasks not writing to /tmp/result
```

### Expression Errors

- Ensure referenced parameters exist in `spec.params`
- Ensure referenced tasks exist and are listed in `dependsOn`
- Check that task names in expressions match exactly (case-sensitive)
- Verify expressions use the correct syntax: `{{params.<name>}}` or `{{tasks.<name>.output}}`

### Timeout Issues

- Increase workflow-level timeout if the entire pipeline takes longer than expected
- Set per-task timeouts for tasks with variable execution time
- Check if agent endpoints are healthy and responsive
- Review retry strategies that may extend total execution time
