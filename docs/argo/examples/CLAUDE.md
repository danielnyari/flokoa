# CLAUDE.md - Argo Workflows Examples Reference

Quick-lookup guide for ~200 Argo Workflow example YAMLs. Use this to find the right pattern when building or debugging workflows.

## Workflow Types

| Kind | Scope | Example |
|------|-------|---------|
| `Workflow` | Namespaced, single run | `hello-world.yaml` |
| `WorkflowTemplate` | Namespaced, reusable | `workflow-template/templates.yaml` |
| `ClusterWorkflowTemplate` | Cluster-scoped, reusable | `cluster-workflow-template/clustertemplates.yaml` |
| `CronWorkflow` | Scheduled execution | `cron-workflow.yaml` |
| `WorkflowEventBinding` | Event-triggered | `workflow-event-binding/event-consumer-workfloweventbinding.yaml` |

## Execution Models

### Steps (sequential + parallel)
- **`steps.yaml`** - Sequential and parallel step groups
- **`steps-inline-workflow.yaml`** - Inline workflow reference in steps

### DAG (directed acyclic graph)
- **`dag-diamond.yaml`** - Classic A->B,C->D diamond pattern
- **`dag-multiroot.yaml`** - Multiple root tasks (no deps)
- **`dag-diamond-steps.yaml`** - Diamond using steps template
- **`dag-nested.yaml`** - Nested template composition
- **`dag-targets.yaml`** - Selective task execution
- **`dag-enhanced-depends.yaml`** - Complex dependency expressions (e.g., `task.Succeeded || task.Failed`)

### Container Set (multiple containers in single pod)
- **`container-set-template/sequence-workflow.yaml`** - Sequential containers via `dependencies`
- **`container-set-template/parallel-workflow.yaml`** - Parallel containers (no deps)
- **`container-set-template/graph-workflow.yaml`** - DAG-style container graphs
- **`container-set-template/workspace-workflow.yaml`** - Shared volume between containers
- **`container-set-template/outputs-result-workflow.yaml`** - Capture container outputs

## Template Types

| Type | Use Case | Key Example |
|------|----------|-------------|
| `container` | Run a container image | `hello-world.yaml` |
| `script` | Inline code (Python/Bash/JS) | `scripts-python.yaml`, `scripts-bash.yaml`, `scripts-javascript.yaml` |
| `resource` | Create/patch K8s resources | `k8s-orchestration.yaml` |
| `http` | HTTP API calls | `http-hello-world.yaml` |
| `suspend` | Wait for human approval | `suspend-template.yaml` |
| `data` | Data transformations | `data-transformations.yaml` |

## Parameters

| Pattern | File |
|---------|------|
| Basic input parameters | `arguments-parameters.yaml` |
| Parameters from ConfigMap | `arguments-parameters-from-configmap.yaml` |
| Global (workflow-level) parameters | `global-parameters.yaml` |
| Global params from ConfigMap | `global-parameters-from-configmap.yaml` |
| Output parameters (from file) | `output-parameter.yaml` |
| Passing between steps | `intermediate-parameters.yaml` |
| Conditional parameter selection | `conditional-parameters.yaml` |
| Aggregating loop results | `parameter-aggregation.yaml`, `parameter-aggregation-dag.yaml` |
| Expression-based JSON destructuring | `expression-destructure-json.yaml`, `expression-destructure-json-complex.yaml` |
| Expression variable reuse | `expression-reusing-verbose-snippets.yaml` |

## Artifacts

### Input Sources
| Source | File |
|--------|------|
| Git repository | `input-artifact-git.yaml` |
| HTTP/HTTPS download | `input-artifact-http.yaml` |
| S3 | `input-artifact-s3.yaml` |
| GCS | `input-artifact-gcs.yaml` |
| Azure Blob | `input-artifact-azure.yaml` |
| Alibaba OSS | `input-artifact-oss.yaml` |
| Raw/inline content | `input-artifact-raw.yaml` |
| HDFS | `hdfs-artifact.yaml` |
| JFrog Artifactory | `artifactory-artifact.yaml` |

### Output Destinations
| Destination | File |
|-------------|------|
| S3 | `output-artifact-s3.yaml` |
| GCS | `output-artifact-gcs.yaml` |
| Azure Blob | `output-artifact-azure.yaml` |
| WebHDFS | `webhdfs-input-output-artifacts.yaml` |

### Artifact Patterns
| Pattern | File |
|---------|------|
| Pass artifacts between steps | `artifact-passing.yaml` |
| Subpath artifacts | `artifact-passing-subpath.yaml` |
| Artifacts as workflow arguments | `arguments-artifacts.yaml` |
| Disable archiving | `artifact-disable-archive.yaml` |
| Garbage collection | `artifact-gc-workflow.yaml` |
| Dynamic path placeholders | `artifact-path-placeholders.yaml` |
| External artifact repository ref | `artifact-repository-ref.yaml` |
| Key-only artifact | `key-only-artifact.yaml` |
| Conditional artifacts | `conditional-artifacts.yaml` |
| Large output handling | `handle-large-output-results.yaml` |
| Custom archive location | `archive-location.yaml` |

## Control Flow

### Conditionals
- **`conditionals.yaml`** - Basic `when` clauses on steps
- **`conditionals-complex.yaml`** - Multi-condition expressions
- **`coinflip.yaml`** - Random branching (script output + `when`)
- **`coinflip-recursive.yaml`** - Recursive conditional branching
- **`dag-coinflip.yaml`** - DAG-style coin flip

### Loops & Iteration
| Pattern | File |
|---------|------|
| `withItems` basic loop | `loops.yaml` |
| Loop over maps/dicts | `loops-maps.yaml` |
| Loop from parameter argument | `loops-param-argument.yaml` |
| Loop from step result | `loops-param-result.yaml` |
| Loop in DAG | `loops-dag.yaml` |
| Sequence generation | `loops-sequence.yaml` |
| Arbitrary sequential steps | `loops-arbitrary-sequential-steps.yaml` |
| Recursive loop | `recursive-for-loop.yaml` |
| Nested sequence result | `withsequence-nested-result.yaml` |

## Error Handling & Retries

| Pattern | File |
|---------|------|
| Basic retry with limit | `retry-container.yaml` |
| Exponential backoff | `retry-backoff.yaml` (`duration`, `factor`, `maxDuration`) |
| Retry on specific errors | `retry-on-error.yaml` |
| Conditional retry | `retry-conditional.yaml` |
| Retry script templates | `retry-script.yaml` |
| Retry with steps | `retry-with-steps.yaml` |
| Retry until completion | `retry-container-to-completion.yaml` |
| Continue on failure | `continue-on-fail.yaml` |
| DAG continue on failure | `dag-continue-on-fail.yaml` |
| DAG disable failFast | `dag-disable-failFast.yaml` |

## Exit Handlers & Lifecycle Hooks

| Pattern | File |
|---------|------|
| Workflow-level `onExit` | `exit-handlers.yaml` |
| Slack notification on exit | `exit-handler-slack.yaml` |
| Step-level exit hooks | `exit-handler-step-level.yaml` |
| DAG task exit hooks | `exit-handler-dag-level.yaml` |
| Pass params to exit handler | `exit-handler-with-param.yaml` |
| Access artifacts in exit handler | `exit-handler-with-artifacts.yaml` |
| Template `onExit` | `template-on-exit.yaml` |
| Workflow lifecycle hooks | `life-cycle-hooks-wf-level.yaml` |
| Template lifecycle hooks | `life-cycle-hooks-tmpl-level.yaml` |

## Timeouts

- **`timeouts-workflow.yaml`** - Workflow-level `activeDeadlineSeconds`
- **`timeouts-step.yaml`** - Per-step timeout
- **`step-level-timeout.yaml`** - Individual step timeout
- **`dag-task-level-timeout.yaml`** - DAG task timeout

## Concurrency & Synchronization

| Pattern | File |
|---------|------|
| Workflow parallelism limit | `parallelism-limit.yaml` |
| Template parallelism limit | `parallelism-template-limit.yaml` |
| Nested parallelism | `parallelism-nested.yaml`, `parallelism-nested-dag.yaml` |
| Semaphore (workflow-level) | `synchronization-wf-level.yaml` |
| Semaphore (template-level) | `synchronization-tmpl-level.yaml` |
| Mutex (workflow-level) | `synchronization-mutex-wf-level.yaml` |
| Mutex (template-level) | `synchronization-mutex-tmpl-level.yaml` |
| DB-backed semaphore | `synchronization-db-wf-level.yaml`, `synchronization-db-tmpl-level.yaml` |
| DB-backed mutex | `synchronization-db-mutex-wf-level.yaml`, `synchronization-db-mutex-tmpl-level.yaml` |

## Daemon Containers & Sidecars

| Pattern | File |
|---------|------|
| Basic sidecar | `sidecar.yaml` |
| Nginx sidecar | `sidecar-nginx.yaml` |
| Docker-in-Docker sidecar | `sidecar-dind.yaml` |
| Daemon step (`daemon: true`) | `daemon-step.yaml` (ref via `{{steps.name.ip}}`) |
| Daemon in DAG | `dag-daemon-task.yaml` (ref via `{{tasks.name.ip}}`) |
| Daemon + retry | `steps-daemon-retry-strategy.yaml`, `dag-daemon-retry-strategy.yaml` |
| StatefulSet + Service daemon | `daemoned-stateful-set-with-service.yaml` |

## Volumes & Storage

- **`volumes-pvc.yaml`** - Ephemeral PVC per workflow (`volumeClaimTemplates`)
- **`volumes-emptydir.yaml`** - EmptyDir ephemeral storage
- **`volumes-existing.yaml`** - Mount existing PVC/ConfigMap

## Kubernetes Resource Management

| Pattern | File |
|---------|------|
| Create/manage K8s resources | `k8s-orchestration.yaml` |
| Orchestrate Jobs | `k8s-jobs.yaml` |
| Patch pods (strategic merge) | `k8s-patch-pod.yaml`, `k8s-patch-merge-pod.yaml` |
| Patch pods (JSON) | `k8s-patch-json-pod.yaml` |
| Patch workflows | `k8s-patch-json-workflow.yaml` |
| YAML patch | `pod-spec-yaml-patch.yaml` |
| Set owner references | `k8s-set-owner-reference.yaml`, `k8s-owner-reference.yaml` |
| Wait for resource readiness | `k8s-wait-wf.yaml` |
| Extract logs from resources | `k8s-resource-log-selector.yaml` |
| Resource delete with flags | `resource-delete-with-flags.yaml` |
| Pod GC strategies | `pod-gc-strategy.yaml`, `pod-gc-strategy-with-label-selector.yaml` |
| Dynamic pod spec from step | `pod-spec-from-previous-step.yaml` |
| Pod spec patching | `pod-spec-patch.yaml`, `pod-spec-patch-wf-tmpl.yaml` |
| Node selection | `node-selector.yaml` |
| Pod Disruption Budget | `default-pdb-support.yaml` |
| Resource quota monitoring | `workflow-count-resourcequota.yaml` |

## HTTP Template

- **`http-hello-world.yaml`** - Basic HTTP GET/POST request
- **`http-success-condition.yaml`** - HTTP with response validation

## Suspend (Human Approval)

- **`suspend-template.yaml`** - Pause workflow, resume with `argo resume`
- **`suspend-template-outputs.yaml`** - Suspend with output parameters

## CronWorkflow (Scheduling)

- **`cron-workflow.yaml`** - Basic schedule (`schedules`, `timezone`)
- **`cron-workflow-multiple-schedules.yaml`** - Multiple schedules
- **`cron-when.yaml`** - Scheduled + conditional logic
- **`cron-backfill.yaml`** - Backfill missed runs

## Reusable Templates

### WorkflowTemplate (namespaced)
- **`workflow-template/hello-world.yaml`** - Simple `templateRef` usage
- **`workflow-template/templates.yaml`** - Template library
- **`workflow-template/steps.yaml`** - Reusable steps
- **`workflow-template/dag.yaml`** - Reusable DAG
- **`workflow-template/workflow-template-ref.yaml`** - Reference by name
- **`workflow-template/workflow-template-ref-with-entrypoint-arg-passing.yaml`** - Argument passing
- **`workflow-template/retry-with-steps.yaml`** - Retry in templates
- **`workflow-template/workflow-archive-logs.yaml`** - Log archiving

### ClusterWorkflowTemplate (cluster-scoped)
- **`cluster-workflow-template/clustertemplates.yaml`** - Cluster-scope definitions
- **`cluster-workflow-template/cluster-wftmpl-dag.yaml`** - Cluster-scope DAG
- **`cluster-workflow-template/mixed-cluster-namespaced-wftmpl-steps.yaml`** - Mix cluster + namespaced

### Inline Templates in DAG
- **`dag-inline-workflow.yaml`** - Inline workflow in DAG
- **`dag-inline-workflowtemplate.yaml`** - Inline template ref
- **`dag-inline-cronworkflow.yaml`** - Inline CronWorkflow
- **`dag-inline-clusterworkflowtemplate.yaml`** - Inline ClusterWorkflowTemplate

## Workflow Composition

- **`nested-workflow.yaml`** - Workflow-in-workflow (parameter/artifact passing)
- **`workflow-of-workflows.yaml`** - Submit workflows from a workflow

## Event-Driven Workflows

- **`workflow-event-binding/event-consumer-workflowtemplate.yaml`** - Template for event consumers
- **`workflow-event-binding/event-consumer-workfloweventbinding.yaml`** - Event source mapping
- **`workflow-event-binding/github-path-filter-workflowtemplate.yaml`** - GitHub webhook template
- **`workflow-event-binding/github-path-filter-workfloweventbinding.yaml`** - GitHub path-based triggers

## Advanced Patterns

| Pattern | File |
|---------|------|
| Map-reduce | `map-reduce.yaml` |
| Work avoidance (skip redundant) | `work-avoidance.yaml` |
| Memoization/caching | `memoize-simple.yaml` (ConfigMap backend) |
| Data transformation pipeline | `data-transformations.yaml` |
| Global output aggregation | `global-outputs.yaml` |
| Template defaults | `template-defaults.yaml` |
| Dynamic labels from workflow | `label-value-from-workflow.yaml` |
| Fibonacci with recursion | `fibonacci-seq-conditional-param.yaml` |

## CI/CD Examples

- **`ci.yaml`** - Full CI pipeline (git checkout, build, test, artifacts)
- **`ci-output-artifact.yaml`** - CI with artifact storage
- **`ci-workflowtemplate.yaml`** - Reusable CI template
- **`influxdb-ci.yaml`** - CI with InfluxDB metrics collection
- **`buildkit-template.yaml`** - BuildKit image build (no Docker daemon)

## Metrics & Observability

- **`custom-metrics.yaml`** - Prometheus gauges/counters
- **`dag-custom-metrics.yaml`** - DAG task-level metrics
- **`grafana-dashboard.json`** - Grafana dashboard config

## Secrets & Security

- **`secrets.yaml`** - Secret mounting and env vars
- **`image-pull-secrets.yaml`** - Private registry auth

## Pod Configuration

- **`init-container.yaml`** - Init containers
- **`pod-metadata.yaml`** - Custom pod labels/annotations
- **`dns-config.yaml`** - Custom DNS configuration
- **`node-selector.yaml`** - Node placement

## Misc

- **`forever.yaml`** - Infinite loop (testing)
- **`colored-logs.yaml`** - ANSI color output
- **`title-and-description-with-markdown.yaml`** - Markdown in workflow annotations
- **`resubmit.yaml`** - Workflow resubmission
- **`fun-with-gifs.yaml`** - GIF processing (ImageMagick)
- **`gc-ttl.yaml`** - TTL-based workflow garbage collection
- **`status-reference.yaml`** - K8s status object handling
- **`exit-code-output-variable.yaml`** - Capture script exit codes

## Supporting Files

- **`configmaps/simple-parameters-configmap.yaml`** - Example ConfigMap for parameter sourcing
- **`example-golang/main.go`** - Go code example
- **`validator.go`** / **`validation_test.go`** - YAML validation utilities
