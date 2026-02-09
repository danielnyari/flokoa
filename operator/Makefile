# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 0.0.2

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# flokoa.ai/operator-bundle:$VERSION and flokoa.ai/operator-catalog:$VERSION.
IMAGE_TAG_BASE ?= ghcr.io/danielnyari/flokoa-operator

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Set the Operator SDK version to use. By default, what is installed on the system is used.
# This is useful for CI or a project to utilize a specific version of the operator-sdk toolkit.
OPERATOR_SDK_VERSION ?= v1.42.0
# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):$(VERSION)

# Server image settings
SERVER_IMAGE_TAG_BASE ?= ghcr.io/danielnyari/flokoa-server
SERVER_IMG ?= $(SERVER_IMAGE_TAG_BASE):$(VERSION)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= operator-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND_CLUSTER=$(KIND_CLUSTER) go test ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Python SDK

PYTHON_SDK_DIR ?= ../sdk/python/src/
PYTHON_MODELS_DIR ?= $(PYTHON_SDK_DIR)/flokoa/types
PYTHON_VENV ?= $(LOCALBIN)/venv
DATAMODEL_CODEGEN ?= $(PYTHON_VENV)/bin/datamodel-codegen

.PHONY: python-venv
python-venv: $(PYTHON_VENV) ## Create Python virtual environment with datamodel-code-generator.
$(PYTHON_VENV):
	@echo "Creating Python virtual environment..."
	@python3 -m venv $(PYTHON_VENV)
	@$(PYTHON_VENV)/bin/pip install --upgrade pip
	@$(PYTHON_VENV)/bin/pip install datamodel-code-generator
	@echo "Python virtual environment created at $(PYTHON_VENV)"

.PHONY: generate-python-models
generate-python-models: manifests python-venv ## Generate Python Pydantic v2 models from CRD schemas.
	@echo "Generating Python models from CRD schemas..."
	@mkdir -p $(PYTHON_MODELS_DIR)
	@# Extract AgentToolSpec schema
	@yq eval '.spec.versions[0].schema.openAPIV3Schema.properties.spec' \
		config/crd/bases/agent.flokoa.ai_agenttools.yaml > /tmp/agenttool_spec_schema.json
	@# Generate AgentToolSpec Pydantic v2 models
	@$(DATAMODEL_CODEGEN) \
		--input /tmp/agenttool_spec_schema.json \
		--input-file-type jsonschema \
		--output-model-type pydantic_v2.BaseModel \
		--snake-case-field \
		--use-annotated \
  		--allow-population-by-field-name \
		--class-name AgentToolSpec \
		--output $(PYTHON_MODELS_DIR)/agenttool.py
	@rm /tmp/agenttool_spec_schema.json
	@echo "Generated Python models at $(PYTHON_MODELS_DIR)/agenttool.py"
	@# Extract AgentCard schema
	@yq eval '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.card' \
		config/crd/bases/agent.flokoa.ai_agents.yaml > /tmp/agentcard_schema.json
	@# Generate AgentCard Pydantic v2 models
	@$(DATAMODEL_CODEGEN) \
		--input /tmp/agentcard_schema.json \
		--input-file-type jsonschema \
		--output-model-type pydantic_v2.BaseModel \
		--snake-case-field \
		--use-annotated \
  		--allow-population-by-field-name \
		--class-name AgentCard \
		--output $(PYTHON_MODELS_DIR)/agentcard.py
	@rm /tmp/agentcard_schema.json
	@echo "Generated Python models at $(PYTHON_MODELS_DIR)/agentcard.py"
	@# Build ModelConfig schema (combines provider and model info from CRDs)
	@echo "Building ModelConfig schema..."
	@# Extract provider specs from ModelProvider CRD
	@yq eval -o json '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.openai' \
		config/crd/bases/agent.flokoa.ai_modelproviders.yaml > /tmp/openai_provider.json
	@yq eval -o json '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.anthropic' \
		config/crd/bases/agent.flokoa.ai_modelproviders.yaml > /tmp/anthropic_provider.json
	@yq eval -o json '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.google' \
		config/crd/bases/agent.flokoa.ai_modelproviders.yaml > /tmp/google_provider.json
	@yq eval -o json '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.bedrock' \
		config/crd/bases/agent.flokoa.ai_modelproviders.yaml > /tmp/bedrock_provider.json
	@# Extract parameters from Model CRD
	@yq eval -o json '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.parameters' \
		config/crd/bases/agent.flokoa.ai_models.yaml > /tmp/model_parameters.json
	@# Build the combined ModelConfig schema using yq (work in YAML, convert at end)
	@yq -n '{"type": "object", "title": "ModelConfig", "description": "ModelConfig contains the resolved model configuration from the ConfigMap", "properties": {"provider": {"type": "object", "title": "Provider", "description": "Provider contains provider type and connection configuration", "properties": {"type": {"type": "string", "title": "ProviderType", "enum": ["openai", "anthropic", "google", "bedrock"], "description": "The provider type"}, "defaultHeaders": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Default headers to include in all requests"}}, "required": ["type"]}, "model": {"type": "string", "description": "The model identifier (e.g., gpt-4o, claude-sonnet-4-20250514)"}}, "required": ["provider", "model"]}' > /tmp/modelconfig_schema.yaml
	@# Inject provider specs into the schema with titles to avoid naming collisions
	@yq eval -i '.properties.provider.properties.openai = load("/tmp/openai_provider.json")' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.provider.properties.openai.title = "OpenAIProviderConfig"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.provider.properties.anthropic = load("/tmp/anthropic_provider.json")' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.provider.properties.anthropic.title = "AnthropicProviderConfig"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.provider.properties.google = load("/tmp/google_provider.json")' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.provider.properties.google.title = "GoogleProviderConfig"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.provider.properties.bedrock = load("/tmp/bedrock_provider.json")' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.provider.properties.bedrock.title = "BedrockProviderConfig"' /tmp/modelconfig_schema.yaml
	@# Inject model parameters with titles
	@yq eval -i '.properties.parameters = load("/tmp/model_parameters.json")' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.title = "ModelParameters"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.openai.title = "OpenAIModelParameters"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.anthropic.title = "AnthropicModelParameters"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.google.title = "GoogleModelParameters"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.bedrock.title = "BedrockModelParameters"' /tmp/modelconfig_schema.yaml
	@# Add titles to nested types that would otherwise collide (Type, Type1, Type2, etc.)
	@yq eval -i '.properties.parameters.properties.anthropic.properties.thinking.title = "AnthropicThinking"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.anthropic.properties.thinking.properties.type.title = "AnthropicThinkingType"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.anthropic.properties.container.title = "AnthropicContainer"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.bedrock.properties.serviceTier.title = "BedrockServiceTier"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.bedrock.properties.serviceTier.properties.type.title = "BedrockServiceTierType"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.bedrock.properties.guardrailConfig.title = "BedrockGuardrailConfig"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.bedrock.properties.guardrailConfig.properties.trace.title = "BedrockGuardrailTrace"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.bedrock.properties.performanceConfiguration.title = "BedrockPerformanceConfig"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.bedrock.properties.performanceConfiguration.properties.latency.title = "BedrockLatency"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.google.properties.thinkingConfig.title = "GoogleThinkingConfig"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.google.properties.thinkingConfig.properties.thinkingLevel.title = "GoogleThinkingLevel"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.google.properties.safetySettings.items.title = "GoogleSafetySetting"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.google.properties.safetySettings.items.properties.category.title = "GoogleHarmCategory"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.google.properties.safetySettings.items.properties.method.title = "GoogleSafetyMethod"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.google.properties.safetySettings.items.properties.threshold.title = "GoogleSafetyThreshold"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.google.properties.videoResolution.title = "GoogleVideoResolution"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.openai.properties.serviceTier.title = "OpenAIServiceTier"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.openai.properties.reasoningEffort.title = "OpenAIReasoningEffort"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.openai.properties.reasoningGenerateSummary.title = "OpenAIReasoningGenerateSummary"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.openai.properties.reasoningSummary.title = "OpenAIReasoningSummary"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.openai.properties.truncation.title = "OpenAITruncation"' /tmp/modelconfig_schema.yaml
	@yq eval -i '.properties.parameters.properties.openai.properties.textVerbosity.title = "OpenAITextVerbosity"' /tmp/modelconfig_schema.yaml
	@# Convert final schema to JSON
	@yq eval -o json '.' /tmp/modelconfig_schema.yaml > /tmp/modelconfig_schema.json
	@# Generate ModelConfig Pydantic v2 models with --use-title-as-name for proper class naming
	@$(DATAMODEL_CODEGEN) \
		--input /tmp/modelconfig_schema.json \
		--input-file-type jsonschema \
		--output-model-type pydantic_v2.BaseModel \
		--snake-case-field \
		--use-annotated \
		--allow-population-by-field-name \
		--use-title-as-name \
		--output $(PYTHON_MODELS_DIR)/modelconfig.py
	@rm /tmp/openai_provider.json /tmp/anthropic_provider.json /tmp/google_provider.json \
		/tmp/bedrock_provider.json /tmp/model_parameters.json /tmp/modelconfig_schema.yaml /tmp/modelconfig_schema.json
	@echo "Generated Python models at $(PYTHON_MODELS_DIR)/modelconfig.py"
	@# Build ManagedConfig schema from Agent CRD managed runtime spec
	@echo "Building ManagedConfig schema..."
	@yq eval -o json '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.runtime.properties.managed.properties.outputSchema' \
		config/crd/bases/agent.flokoa.ai_agents.yaml > /tmp/managed_output_schema.json
	@yq -n '{"type": "object", "title": "ManagedConfig", "description": "Configuration for a managed agent runtime, mounted from the operator ConfigMap", "properties": {"outputSchema": {}}, "additionalProperties": false}' > /tmp/managedconfig_schema.yaml
	@yq eval -i '.properties.outputSchema = load("/tmp/managed_output_schema.json")' /tmp/managedconfig_schema.yaml
	@yq eval -i '.properties.outputSchema.title = "OutputSchema"' /tmp/managedconfig_schema.yaml
	@yq eval -i '.properties.outputSchema.description = "JSON Schema constraining the agent response format"' /tmp/managedconfig_schema.yaml
	@yq eval -o json '.' /tmp/managedconfig_schema.yaml > /tmp/managedconfig_schema.json
	@$(DATAMODEL_CODEGEN) \
		--input /tmp/managedconfig_schema.json \
		--input-file-type jsonschema \
		--output-model-type pydantic_v2.BaseModel \
		--snake-case-field \
		--use-annotated \
		--allow-population-by-field-name \
		--use-title-as-name \
		--class-name ManagedConfig \
		--output $(PYTHON_MODELS_DIR)/managedconfig.py
	@rm /tmp/managed_output_schema.json /tmp/managedconfig_schema.yaml /tmp/managedconfig_schema.json
	@echo "Generated Python models at $(PYTHON_MODELS_DIR)/managedconfig.py"

##@ gRPC Server

.PHONY: buf-generate
buf-generate: buf-dep-update ## Generate gRPC code from proto files.
	cd server && buf generate

.PHONY: buf-lint
buf-lint: ## Lint proto files.
	cd server && buf lint

.PHONY: buf-format
buf-format: ## Format proto files.
	cd server && buf format -w

.PHONY: buf-breaking
buf-breaking: ## Check for breaking proto changes.
	cd server && buf breaking --against '.git#branch=main,subdir=operator/server'

.PHONY: buf-dep-update
buf-dep-update: ## Update buf dependencies.
	cd server && buf dep update

##@ Build

.PHONY: build
build: manifests generate buf-generate fmt vet ## Build manager and server binaries.
	go build -o bin/manager cmd/main.go
	go build -o bin/server cmd/server/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker images for manager and server.
	$(CONTAINER_TOOL) build -t ${IMG} .
	$(CONTAINER_TOOL) build -f server/Dockerfile -t ${SERVER_IMG} .

.PHONY: docker-push
docker-push: ## Push docker images for manager and server.
	$(CONTAINER_TOOL) push ${IMG}
	$(CONTAINER_TOOL) push ${SERVER_IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name operator-builder
	$(CONTAINER_TOOL) buildx use operator-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm operator-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller and server to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	cd config/server && $(KUSTOMIZE) edit set image server=${SERVER_IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.18.0
#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.1.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

.PHONY: operator-sdk
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk
operator-sdk: ## Download operator-sdk locally if necessary.
ifeq (,$(wildcard $(OPERATOR_SDK)))
ifeq (, $(shell which operator-sdk 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK) ;\
	}
else
OPERATOR_SDK = $(shell which operator-sdk)
endif
endif

.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle $(BUNDLE_GEN_FLAGS)
	$(OPERATOR_SDK) bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	$(CONTAINER_TOOL) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM = $(LOCALBIN)/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.55.0/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool $(CONTAINER_TOOL) --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)
