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
│   ├── Dockerfile                # Agent container image
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

## Running the Test

From the `operator` directory:

```bash
# Run all e2e tests (including this one)
make test-e2e

# Or run just this test
make setup-test-e2e
go test ./test/e2e -v -ginkgo.focus="Template Agent E2E Test"
```

## Key Features Tested

1. **Templated Instructions**: The Instruction CRD contains variables like `${role}` and `${task_type}` that can be substituted at runtime
2. **Python SDK Integration**: The agent is built using the Flokoa Python SDK with pydantic-ai
3. **Cross-Resource References**: Agent references Model, Instruction, and AgentTool across CRDs
4. **Mock Dependencies**: Only LLM calls are mocked; all other components are real

## Cleanup

The test includes cleanup logic that deletes all created resources after completion or failure.
