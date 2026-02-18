---
name: operator-deploy-e2e
description: Deploy the Flokoa operator to a Kubernetes cluster and run e2e test workflows. Use when the user asks to deploy the operator, deploy e2e testdata, deploy Argo Workflows, set up a full local test environment, or run build-deploy combos. Covers CRD installation, operator deployment, Argo Workflows, executor plugins, and e2e test fixture deployment.
---

# Operator Deploy and E2E Testdata

All commands must be run from the `operator/` directory.

## Build + Deploy Combos

### Build and Deploy Operator (No Registry Push)

Build images locally and deploy to the current cluster:

```bash
make docker-build
make deploy
```

Override the version for dev builds:

```bash
make docker-build VERSION=dev
make deploy IMG=ghcr.io/danielnyari/flokoa-operator:dev SERVER_IMG=ghcr.io/danielnyari/flokoa-server:dev
```

### Build and Deploy Everything (Operator + Argo + Plugins)

Full local test environment setup:

```bash
make docker-build
make deploy-full
```

`deploy-full` chains: `deploy`, `deploy-argo-workflows`, `deploy-executor-plugins`.

### Build, Push, and Deploy

Build, push to registry, then deploy:

```bash
make docker-build
make docker-push
make deploy
```

## CRD Installation

Install CRDs only (no controller deployment):

```bash
make install
```

Uninstall CRDs:

```bash
make uninstall
```

## Operator Deployment

Deploy the operator controller and server:

```bash
make deploy
```

This applies kustomize overlays from `config/default-no-certmanager/` by default.

With cert-manager enabled:

```bash
make deploy DEPLOY_WITH_CERT_MANAGER=true
```

Undeploy:

```bash
make undeploy
```

## Argo Workflows Deployment

Deploy Argo Workflows with executor plugins enabled:

```bash
make deploy-argo-workflows
```

This installs Argo Workflows v3.7.9, enables the `ARGO_EXECUTOR_PLUGINS` env var on the workflow controller, and waits for readiness.

Deploy executor plugins (A2A plugin):

```bash
make deploy-executor-plugins
```

Undeploy Argo:

```bash
make undeploy-argo-workflows
make undeploy-executor-plugins
```

## Deploy E2E Testdata

Deploy test fixtures including sample Agents, AgentTools, and Argo workflows:

```bash
OPENAI_API_KEY=<your-key> make deploy-e2e-testdata
```

**Requirements:**
- `OPENAI_API_KEY` environment variable must be set
- Minikube must be running (`minikube` binary available)
- Operator must already be deployed (`make deploy`)

**What it does:**
1. Builds the `petstore:test` Docker image using Minikube's Docker daemon
2. Creates an `openai-api-key` secret in the `flokoa-system` namespace
3. Applies kustomized test fixtures from `test/e2e/testdata/`
4. Submits an Argo workflow from `test/e2e/testdata/argo/workflow.yaml`

### Image Build Mode

By default, images are built inside Minikube. To use Docker and load into Minikube instead:

```bash
OPENAI_API_KEY=<key> E2E_IMAGE_BUILD_MODE=docker make deploy-e2e-testdata
```

### Customizable Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENAI_API_KEY` | (required) | API key for the OpenAI secret |
| `E2E_NAMESPACE` | `flokoa-system` | Namespace for test resources |
| `PETSTORE_IMAGE` | `petstore:test` | Test image name |
| `E2E_IMAGE_BUILD_MODE` | `minikube` | `minikube` or `docker` |

## Full Local Test Environment

### Setup from Scratch

Complete sequence for setting up a local environment with test data:

```bash
# 1. Build all images
make docker-build

# 2. Deploy operator + Argo + plugins
make deploy-full

# 3. Deploy test fixtures
OPENAI_API_KEY=<key> make deploy-e2e-testdata
```

### Teardown

```bash
make undeploy-full
```

This chains: `undeploy-executor-plugins`, `undeploy-argo-workflows`, `undeploy`.

## Kind Cluster E2E Tests

For automated e2e testing with a disposable Kind cluster:

```bash
make test-e2e
```

This creates a Kind cluster named `operator-test-e2e`, runs e2e tests, then cleans up.

Manage the cluster manually:

```bash
make setup-test-e2e      # Create Kind cluster
make cleanup-test-e2e    # Delete Kind cluster
```

Skip cert-manager during e2e:

```bash
CERT_MANAGER_INSTALL_SKIP=true make test-e2e
```

## Installer YAML

Generate a single consolidated YAML for deployment:

```bash
make build-installer
```

Output: `dist/install.yaml` containing CRDs + deployment manifests.

## Troubleshooting

### "OPENAI_API_KEY is not set"
Export the env var: `export OPENAI_API_KEY=sk-...`

### "Minikube is not installed"
Install Minikube: https://minikube.sigs.k8s.io/docs/start/

### Operator pods not starting
Check logs: `kubectl -n flokoa-system logs deployment/flokoa-operator-controller-manager`

### CRDs not applied
Run `make install` before `make deploy`.

### Argo workflow submission fails
Ensure Argo is deployed: `make deploy-argo-workflows` and the argo CLI is available: `make argo-cli`.
