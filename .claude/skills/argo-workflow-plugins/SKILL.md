---
name: argo-workflow-plugins
description: Write and manage Argo Workflows executor plugins for Flokoa. Use when the user asks about writing Argo plugins, creating workflow templates that call agents, deploying executor plugins, or integrating Argo Workflows with Flokoa agents.
---

# Argo Workflows Executor Plugins

## Overview

Argo Workflows executor plugins extend Argo with custom step types. Flokoa uses the **sidecar executor plugin** model where a plugin runs as a sidecar container in workflow pods, exposing an HTTP API that Argo calls.

The existing A2A plugin in `operator/plugins/a2a/` is the reference implementation.

## Plugin Architecture

```
Argo Workflow Pod
├── main container (wait)
└── sidecar: plugin container (HTTP server on port 4355)
    ├── POST /api/v1/template.execute  (required)
    └── GET /healthz                    (recommended)
```

**Key concepts:**
- Plugins are registered as `ConfigMap` resources in the `argo` namespace
- Argo injects plugin sidecars into workflow pods automatically
- Authorization uses a token mounted at `/var/run/argo/token`
- Plugins communicate via JSON over HTTP on port 4355

## Writing a New Plugin

### Step 1: Create Plugin Directory

```
operator/plugins/<name>/
├── main.go              # HTTP server entrypoint
├── plugin/
│   ├── plugin.go        # Core execution logic
│   ├── types.go         # Spec and state types
│   └── plugin_test.go   # Tests
├── config/
│   └── plugin.yaml      # ExecutorPlugin resource definition
├── Dockerfile           # Multi-stage build
├── go.mod               # Go module (can reference parent)
└── go.sum
```

### Step 2: Implement the HTTP Server

The server must handle `POST /api/v1/template.execute`:

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"

    executor "github.com/argoproj/argo-workflows/v3/pkg/plugins/executor"
)

var argoToken string

func init() {
    token, err := os.ReadFile("/var/run/argo/token")
    if err != nil {
        log.Printf("Warning: failed to read token: %v", err)
    } else {
        argoToken = string(token)
    }
}

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/api/v1/template.execute", handleExecute)
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    log.Fatal(http.ListenAndServe(":4355", mux))
}

func handleExecute(w http.ResponseWriter, r *http.Request) {
    // 1. Validate authorization
    if argoToken != "" {
        if r.Header.Get("Authorization") != "Bearer "+argoToken {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }
    }

    // 2. Decode request
    var req executor.ExecuteTemplateRequest
    if err := json.NewDecoder(r.Body).Decode(&req.Body); err != nil {
        writeJSON(w, executor.ExecuteTemplateReply{})
        return
    }

    // 3. Check if this template is for your plugin
    pluginData := parsePluginData(req.Body.Template)
    if _, ok := pluginData["<name>"]; !ok {
        // Not our template - return empty response
        writeJSON(w, struct{}{})
        return
    }

    // 4. Execute and return result
    // ... your logic here
}
```

### Step 3: Define Plugin Spec Types

```go
type MyPluginSpec struct {
    // Required fields for your plugin
    Target  string           `json:"target"`
    Action  string           `json:"action"`
    Timeout *metav1.Duration `json:"timeout,omitempty"`
}
```

### Step 4: Return Proper Responses

**Synchronous completion:**
```go
return &executor.ExecuteTemplateReply{
    Node: &wfv1.NodeResult{
        Phase:   wfv1.NodeSucceeded, // or wfv1.NodeFailed
        Message: "Task completed",
        Outputs: &wfv1.Outputs{
            Parameters: []wfv1.Parameter{
                {Name: "result", Value: wfv1.AnyStringPtr("output value")},
            },
        },
    },
}, nil
```

**Asynchronous (long-running) - requeue pattern:**
```go
// First call: start task, return Running with requeue
return &executor.ExecuteTemplateReply{
    Node: &wfv1.NodeResult{
        Phase:   wfv1.NodeRunning,
        Message: "Task submitted, waiting for completion",
    },
    Requeue: &metav1.Duration{Duration: 5 * time.Second},
}, nil

// Subsequent calls: check status, return final result or requeue again
```

**Not our template - return empty:**
```go
writeJSON(w, struct{}{})  // Empty JSON {}
```

### Step 5: Create ExecutorPlugin Resource

`config/plugin.yaml`:
```yaml
apiVersion: argoproj.io/v1alpha1
kind: ExecutorPlugin
metadata:
  name: <plugin-name>
  namespace: argo
spec:
  sidecar:
    automountServiceAccountToken: true
    container:
      name: <plugin-name>-executor-plugin
      image: ghcr.io/danielnyari/flokoa-<plugin-name>-plugin:latest
      command:
        - /plugin-binary
      ports:
        - containerPort: 4355
          protocol: TCP
      resources:
        requests:
          memory: "64Mi"
          cpu: "100m"
        limits:
          memory: "128Mi"
          cpu: "500m"
      securityContext:
        runAsNonRoot: false
        allowPrivilegeEscalation: false
        capabilities:
          drop:
            - ALL
        readOnlyRootFilesystem: true
```

### Step 6: Build and Deploy

Build the plugin ConfigMap:
```bash
cd operator/plugins/<name>/config
argo executor-plugin build .
```

Deploy to the cluster:
```bash
kubectl -n argo apply -f <name>-executor-plugin-configmap.yaml
```

Or add make targets to the operator Makefile following the pattern of the A2A plugin.

## Using Plugins in Workflows

### Simple Workflow
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: my-plugin-test-
spec:
  entrypoint: run-plugin
  serviceAccountName: argo-workflow
  automountServiceAccountToken: true
  templates:
    - name: run-plugin
      plugin:
        <plugin-name>:
          target: "my-target"
          action: "do-something"
          timeout: 2m
```

### Parameterized WorkflowTemplate
```yaml
apiVersion: argoproj.io/v1alpha1
kind: WorkflowTemplate
metadata:
  name: my-plugin-template
spec:
  entrypoint: run-plugin
  serviceAccountName: argo-workflow
  automountServiceAccountToken: true
  arguments:
    parameters:
      - name: target
      - name: action
      - name: timeout
        value: "2m"
  templates:
    - name: run-plugin
      plugin:
        <plugin-name>:
          target: "{{workflow.parameters.target}}"
          action: "{{workflow.parameters.action}}"
          timeout: "{{workflow.parameters.timeout}}"
```

### Using Output Parameters
```yaml
templates:
  - name: pipeline
    dag:
      tasks:
        - name: step1
          template: call-plugin
        - name: step2
          template: use-result
          dependencies: [step1]
          arguments:
            parameters:
              - name: input
                value: "{{tasks.step1.outputs.parameters.result}}"
```

## Deploying Argo Workflows

Deploy Argo Workflows with executor plugins enabled:
```bash
make deploy-argo-workflows       # From operator/ directory
```

This installs Argo Workflows and patches the workflow-controller to enable executor plugins (`ARGO_EXECUTOR_PLUGINS=true`).

Deploy the A2A executor plugin:
```bash
make deploy-executor-plugins     # From operator/ directory
```

Deploy everything at once:
```bash
make deploy-full                 # Operator + Argo + plugins
```

## Existing A2A Plugin Reference

The A2A plugin in `operator/plugins/a2a/` demonstrates:

- Token-based authorization (`/var/run/argo/token`)
- Plugin spec parsing from template JSON
- Agent endpoint resolution (Kubernetes CR lookup or convention-based)
- Async task lifecycle with requeue polling
- Multiple output parameters (`result` text + `taskResponse` JSON)
- Timeout handling
- Multiple endpoint candidate fallback

Key files:
- `main.go` - HTTP server, routing, auth
- `plugin/plugin.go` - Core execution, A2A client, task lifecycle
- `plugin/resolver.go` - Agent endpoint discovery
- `plugin/types.go` - A2ASpec, ProgressState types
