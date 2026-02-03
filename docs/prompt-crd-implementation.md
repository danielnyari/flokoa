# Prompt CRD Implementation Summary

This document summarizes the Prompt CRD implementation added to the Flokoa operator.

## Overview

The Prompt CRD enables declarative management of AI prompts with support for:
- **Inline prompts**: Content defined directly in the manifest
- **Langfuse integration**: Fetch prompts from Langfuse with version control
- **Langsmith integration**: Fetch prompts from Langsmith with commit-based versioning
- **Automatic sync**: Periodic refresh from external sources
- **Agent integration**: Reference prompts from Agent resources

## Architecture

### API Design (v1alpha1)

**Prompt CRD (`agent.flokoa.ai/v1alpha1`)**

The Prompt resource has:
- **Spec**: Defines the prompt source and sync configuration
  - `source`: Union of langfuse, langsmith, or inline
  - `sync`: Optional refresh interval for external sources
- **Status**: Contains resolved prompt state
  - `resolvedContent`: Current prompt content
  - `resolvedAt`: Last resolution timestamp
  - `sourceVersion`: Version/commit from source
  - `checksum`: SHA256 hash of content
  - `conditions`: Ready status

### Source Types

1. **Inline Source**
   ```yaml
   source:
     inline:
       content: "Your prompt here"
   ```

2. **Langfuse Source**
   ```yaml
   source:
     langfuse:
       endpoint: https://cloud.langfuse.com
       credentialsSecretRef:
         name: langfuse-credentials
       promptName: my-prompt
       version: "3"
   ```

3. **Langsmith Source**
   ```yaml
   source:
     langsmith:
       credentialsSecretRef:
         name: langsmith-credentials
       promptName: my-prompt
       commitHash: abc123
   ```

### Agent Integration

Agents can reference prompts using `promptRefs`:

```yaml
spec:
  promptRefs:
    - name: system-prompt
      as: system
    - name: guidelines
      as: safety
```

## Implementation Details

### Files Created

1. **API Types**: `operator/api/v1alpha1/prompt_types.go`
   - Defines Prompt, PromptSpec, PromptStatus, and source types
   - Uses discriminated union pattern for sources
   
2. **Controller**: `operator/internal/controller/prompt_controller.go`
   - Reconciles Prompt resources
   - Validates source configuration (exactly one must be set)
   - Resolves content based on source type
   - Implements sync logic with configurable intervals
   - Calculates SHA256 checksum for content
   
3. **Tests**: `operator/internal/controller/prompt_controller_test.go`
   - Basic test for inline prompts
   - Test for invalid multi-source configuration
   
4. **Examples**: `docs/examples/prompt/`
   - Inline prompt example
   - Langfuse prompt with sync
   - Langsmith prompt with sync
   - Agent with prompt references
   - Comprehensive README

### Files Modified

1. **Agent Types**: `operator/api/v1alpha1/agent_types.go`
   - Added `PromptRef` type with `name`, `as`, and `namespace` fields
   - Added `promptRefs` field to `AgentSpec`

2. **Main**: `operator/cmd/main.go`
   - Registered PromptReconciler with the manager

3. **Generated Files**:
   - `api/v1alpha1/zz_generated.deepcopy.go` - DeepCopy methods
   - `config/crd/bases/agent.flokoa.ai_prompts.yaml` - Prompt CRD manifest
   - `config/crd/bases/agent.flokoa.ai_agents.yaml` - Updated Agent CRD
   - `config/rbac/role.yaml` - RBAC permissions for prompts

## Controller Behavior

### Reconciliation Logic

1. **Validation**: Ensures exactly one source is specified
2. **Resolution**: Fetches/resolves content based on source type
3. **Status Update**: Updates resolved content, version, checksum, timestamp
4. **Conditions**: Sets Ready condition (True/False) with reason
5. **Requeue**: Schedules next reconciliation based on sync interval

### Sync Configuration

- Only applies to external sources (Langfuse, Langsmith)
- Format: `30s`, `5m`, `1h`, `24h`
- Controller requeues reconciliation after the interval
- Inline prompts never requeue (static content)

### Status Conditions

The controller sets a `Ready` condition:
- `True` when content is successfully resolved
- `False` on validation or resolution errors
- Includes reason and message for troubleshooting

## API Specification

### Prompt.spec.source

Discriminated union - exactly one of:
- `inline.content` (string, required)
- `langfuse` (object with endpoint, credentialsSecretRef, promptName, version)
- `langsmith` (object with credentialsSecretRef, promptName, commitHash)

### Prompt.spec.sync

Optional for external sources:
- `interval` (string, pattern: `^([0-9]+(\.[0-9]+)?(s|m|h))+$`)

### Prompt.status

Read-only fields:
- `resolvedContent` (string) - Current prompt content
- `resolvedAt` (timestamp) - Last resolution time
- `sourceVersion` (string) - Version/commit identifier
- `checksum` (string) - SHA256 hash (format: `sha256:...`)
- `conditions` (array) - Standard Kubernetes conditions
- `observedGeneration` (int64) - Last observed spec generation

## Credentials Management

### Langfuse Secret Format
```yaml
apiVersion: v1
kind: Secret
type: Opaque
data:
  publicKey: <base64>
  secretKey: <base64>
```

### Langsmith Secret Format
```yaml
apiVersion: v1
kind: Secret
type: Opaque
data:
  apiKey: <base64>
```

## Future Enhancements

1. **API Implementation**: Currently, Langfuse and Langsmith fetch logic returns placeholders. Actual HTTP API calls need to be implemented.

2. **Caching**: Consider caching resolved prompts to reduce API calls.

3. **Webhooks**: Add validation webhook to enforce source exclusivity at admission time.

4. **Metrics**: Expose metrics for:
   - Prompt resolution success/failure rates
   - Sync latency
   - API call counts

5. **Events**: Emit Kubernetes events for:
   - Successful resolution
   - Sync updates
   - Resolution failures

6. **Agent Runtime Integration**: Update agent runtime to:
   - Inject resolved prompts into containers
   - Watch for prompt updates
   - Reload configuration when prompts change

## Testing

### Unit Tests
- `prompt_controller_test.go` contains basic tests
- Tests validate inline prompts and multi-source rejection
- Full test suite requires kubebuilder test environment

### Manual Testing
```bash
# Apply a prompt
kubectl apply -f docs/examples/prompt/inline-prompt.yaml

# Check status
kubectl get prompts
kubectl describe prompt customer-support-system

# View resolved content
kubectl get prompt customer-support-system -o jsonpath='{.status.resolvedContent}'
```

## Design Rationale

### Why Single CRD vs Provider/Resource Split?

The implementation uses a single Prompt CRD with embedded provider config rather than separate Provider and Prompt resources because:

1. **Scope**: Prompt management tools (Langfuse, Langsmith) are project-scoped, not multi-project like LLM providers
2. **Credentials**: Each project/environment typically has its own API keys
3. **Simplicity**: Fewer resources to manage
4. **Sharing**: Credentials can still be shared by referencing the same Secret

This aligns with the actual usage patterns of prompt management platforms.

### API Group Alignment

Uses the same API group (`agent.flokoa.ai`) as other agent-related resources for consistency.

## References

- [Prompt CRD Types](../../operator/api/v1alpha1/prompt_types.go)
- [Prompt Controller](../../operator/internal/controller/prompt_controller.go)
- [Examples](../../docs/examples/prompt/)
- [Agent CRD with PromptRefs](../../operator/api/v1alpha1/agent_types.go)
