# E2E Test: Template Agent

This e2e test validates the integration between the Kubernetes Operator and Python SDK by deploying a template-based agent.

## Test Scenario

The test implements scenario #2 from the e2e test plan (`docs/e2e-test-plan.md`): **Template Agent (Instruction templates)**

## What It Tests

1. **Mock LLM Service**: Deploys a simple HTTP service that mimics OpenAI's API
2. **Mock Tool Service**: Deploys a simple HTTP service for AgentTool testing
3. **ModelProvider**: Creates a ModelProvider pointing to the mock LLM service
4. **Model**: Creates a Model referencing the ModelProvider
5. **Instruction**: Creates an Instruction resource with template variables (e.g., `${role}`, `${task_type}`)
6. **AgentTool**: Creates an AgentTool for testing tool integration
7. **Agent**: Creates an Agent that:
   - Uses the Python SDK (pydantic-ai framework)
   - References the templated Instruction
   - References the AgentTool
   - Uses the Model

## Assertions

The test verifies:

- All CRDs are successfully applied
- LLM stub and tool services reach Running state
- Instruction ConfigMap is created (containing the template)
- Agent reaches `Ready=True` condition
- Agent pod is running
- Agent service is created

## Files Structure

```
test/e2e/
├── fixtures/
│   ├── template_agent.py         # Python SDK agent module
│   ├── Dockerfile                # Agent container image (builds from repo root)
│   ├── llm_stub.py               # Mock LLM service
│   ├── Dockerfile.llm-stub       # LLM stub container image
│   ├── tool_service.py           # Mock tool service
│   └── Dockerfile.tool-service   # Tool service container image
└── testdata/
    ├── secret.yaml               # API key secret for ModelProvider
    ├── modelprovider.yaml        # ModelProvider pointing to LLM stub
    ├── model.yaml                # Model configuration
    ├── instruction.yaml          # Instruction with template variables
    ├── agenttool.yaml            # AgentTool configuration
    ├── agent.yaml                # Agent referencing the Instruction
    ├── llm-stub.yaml            # LLM stub deployment and service
    └── tool-service.yaml        # Tool service deployment and service
```

**Note**: The template agent Dockerfile builds from the repository root to access the SDK source code at `sdk/python`.

## Running the Test

From the `operator` directory:

```bash
# Run all e2e tests (including this one)
make test-e2e

# Or run just this test
make setup-test-e2e
go test ./test/e2e -v -ginkgo.focus="Template Agent E2E Test"
```

## Environment Requirements

- `OPENAI_API_KEY` must be set. The e2e suite performs a real model call and fails fast if the variable is missing.
- The suite uses a randomized namespace per run (for example `flokoa-system-a1b2c3d4`) to avoid collisions with previous failed runs.
- Optional: set `E2E_NAMESPACE` to force a specific namespace name for debugging.

## Key Features Tested

1. **Templated Instructions**: The Instruction CRD contains variables like `${role}` and `${task_type}` that can be substituted at runtime
2. **Python SDK Integration**: The agent is built using the Flokoa Python SDK with pydantic-ai
3. **Cross-Resource References**: Agent references Model, Instruction, and AgentTool across CRDs
4. **Mock Dependencies**: Only LLM calls are mocked; all other components are real

## Cleanup

The test includes cleanup logic that deletes all created resources after completion or failure.

## Capability Delivery E2E (`capability_test.go`)

Validates roadmap 09 (capability artifacts & delivery): an Agent attaching two
digest-pinned Capability CRs (`fixtures/capabilities/{echo,upper}`, built by
`make build-e2e-capability-artifacts`) becomes Ready with the wheelhouses
delivered into the runner pod, an A2A `message/send` exercises the echo
capability tool (asserting the configured `e2e-cap:` prefix), and a tampered
artifact (wheel bytes corrupted after manifest assembly) fails bootstrap with
the structured `wheelhouse integrity check failed` error.

### Delivery modes

The same spec serves both CI jobs, gated on `CAPABILITY_DELIVERY_EXPECT`:

- unset / `initContainer` (default job): asserts two `cap-*` copy
  initContainers and the shared read-only emptyDir mount.
- `imageVolume` (advisory job in `test-e2e.yml`): the test switches the
  operator to `--capability-delivery-mode=auto`, then asserts the
  `flokoa-capability-delivery` state ConfigMap records
  `effectiveMode: imageVolume`, pods carry image volumes with zero `cap-*`
  initContainers, and the agent still answers. Requires the cluster from
  `kind-config-imagevolume.yaml` (ImageVolume feature gate) on a
  containerd 2.x node image (kind >= v0.32.0, node image 1.35+).

### Digest resolution without a registry

Capability CRs must be digest-pinned, but locally-built images have no
RepoDigest until a manifest exists. `utils.LoadImageAndGetDigest` loads the
image with `kind load` and reads the containerd-recorded digest back from the
node (`docker exec <node> crictl inspecti`; when CRI reports empty
repoDigests — archive imports store the digest without a repo@digest
reference — it falls back to the `ctr images ls` DIGEST column), then
registers the canonical `repo@digest` reference on every node so the
kubelet's by-digest lookup resolves. This is the suite's one coupling to Kind node internals, isolated in
that util. **Fallback** if a Kind/containerd upgrade breaks it: the standard
[local registry pattern](https://kind.sigs.k8s.io/docs/user/local-registry/) —
run a registry container next to the cluster, `docker push` the fixture
images, and take the digest from the push output.
