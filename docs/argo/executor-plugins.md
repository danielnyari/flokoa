# Argo Workflows Executor Plugins

The operator ships an **A2A (Agent-to-Agent) executor plugin** for Argo Workflows so that
Argo workflows can call Flokoa agents. This page is the architecture and authoring reference;
for end-to-end workflow YAML patterns see [`examples/`](examples/), and for hands-on tasks use
the `argo-workflow-plugins` skill.

## Architecture

- **Plugin type**: sidecar executor plugin (HTTP server on port `4355`)
- **API endpoint**: `POST /api/v1/template.execute`
- **Auth**: Bearer token read from `/var/run/argo/token`
- **Protocol**: A2A via the `a2a-go` client library

Source lives in `operator/plugins/a2a/` (`main.go`, `plugin/plugin.go`, `plugin/resolver.go`,
`plugin/types.go`, `config/plugin.yaml`, `Dockerfile`).

## How it works

1. Argo injects the plugin as a sidecar container into workflow pods.
2. When a template has a `plugin.a2a` spec, Argo calls the plugin's HTTP API.
3. The plugin resolves the agent endpoint (via the Agent CR or convention-based naming).
4. It sends an A2A message and polls for task completion with requeue.
5. It returns outputs: `result` (text) and `taskResponse` (full JSON).

### Plugin spec (`A2ASpec`)

```yaml
plugin:
  a2a:
    agent: my-agent         # Agent CR name (required)
    namespace: default      # Agent namespace (optional, defaults to the workflow namespace)
    message: "Do something" # Message to send (required)
    timeout: 5m             # Task timeout (optional, default 5m)
```

### Response lifecycle

- **Synchronous**: return `NodeSucceeded`/`NodeFailed` immediately.
- **Asynchronous**: return `NodeRunning` with a `Requeue` duration; Argo calls back after the
  interval. Track state via an in-memory map keyed by workflow UID + template name.

## Workflow examples

Simple workflow:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: a2a-test-
spec:
  entrypoint: call-agent
  serviceAccountName: flokoa-workflow
  automountServiceAccountToken: true
  templates:
    - name: call-agent
      plugin:
        a2a:
          agent: my-agent
          message: "What are the benefits of Kubernetes?"
          timeout: 2m
```

Parameterized `WorkflowTemplate`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: WorkflowTemplate
metadata:
  name: a2a-agent-template
spec:
  entrypoint: call-agent
  serviceAccountName: flokoa-workflow
  automountServiceAccountToken: true
  arguments:
    parameters:
      - name: agent-name
      - name: agent-namespace
      - name: message
      - name: timeout
        value: "2m"
  templates:
    - name: call-agent
      plugin:
        a2a:
          agent: "{{workflow.parameters.agent-name}}"
          namespace: "{{workflow.parameters.agent-namespace}}"
          message: "{{workflow.parameters.message}}"
          timeout: "{{workflow.parameters.timeout}}"
```

## Writing a new executor plugin

1. **Create a plugin directory**: `operator/plugins/<name>/`.
2. **Implement an HTTP server** listening on port `4355`:
   - `POST /api/v1/template.execute` — main execution endpoint
   - `GET /healthz` — health check
3. **Handle authorization**: read the token from `/var/run/argo/token`, validate the
   `Authorization: Bearer <token>` header.
4. **Define the plugin spec type** (the YAML under `plugin.<name>` in workflow templates).
5. **Return proper responses**:
   - empty `{}` for templates this plugin doesn't handle
   - `ExecuteTemplateReply` with `Node.Phase` and optional `Node.Outputs`
   - use `Requeue` with a `metav1.Duration` for long-running tasks
6. **Create `config/plugin.yaml`** (an `ExecutorPlugin` CR):

   ```yaml
   apiVersion: argoproj.io/v1alpha1
   kind: ExecutorPlugin
   metadata:
     name: <plugin-name>
   spec:
     sidecar:
       automountServiceAccountToken: true
       container:
         name: <plugin-name>-executor-plugin
         image: ghcr.io/danielnyari/flokoa-<plugin-name>-plugin:latest
         command: ["/plugin-binary"]
         ports:
           - containerPort: 4355
         resources:
           requests: { memory: "64Mi", cpu: "100m" }
           limits: { memory: "128Mi", cpu: "500m" }
         securityContext:
           runAsNonRoot: false
           allowPrivilegeEscalation: false
           capabilities: { drop: [ALL] }
           readOnlyRootFilesystem: true
   ```

7. **Build and deploy**:

   ```bash
   cd operator/plugins/<name>/config && argo executor-plugin build .
   kubectl -n argo apply -f <name>-executor-plugin-configmap.yaml
   ```

   Or, from `operator/`: `make deploy-executor-plugins` (builds and deploys the bundled A2A plugin).

## See also

- [`docs/argo/examples/`](examples/) — navigator for upstream Argo Workflows example YAMLs
- `argo-workflow-plugins` skill — write/manage plugins and workflow templates
- `operator/internal/controller/agentworkflow_compiler.go` — how `AgentWorkflow` CRs compile
  into Argo `WorkflowTemplate`s that call this plugin
