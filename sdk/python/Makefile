.PHONY: install
install: ## Install workspace dependencies and pre-commit hooks
	@echo "🚀 Creating virtual environment using uv"
	@uv sync --all-packages
	@uv run pre-commit install

.PHONY: test
test: test-flokoa ## Run all tests

.PHONY: test-flokoa
test-flokoa: ## Run flokoa tests
	@echo "🚀 Testing flokoa"
	@cd flokoa && uv run pytest tests --cov --cov-config=pyproject.toml --cov-report=xml

.PHONY: test-managed-agent
test-managed-agent: ## Run flokoa-managed-agent tests
	@echo "🚀 Testing flokoa-managed-agent"
	@uv run --package flokoa-managed-agent pytest flokoa-managed-agent/tests

.PHONY: check
check: ## Run code quality tools
	@echo "🚀 Checking lock file consistency with 'pyproject.toml'"
	@uv lock --locked
	@echo "🚀 Linting code: Running pre-commit"
	@uv run pre-commit run -a

.PHONY: check-flokoa
check-flokoa: ## Run flokoa linting and type checking
	@echo "🚀 Linting flokoa"
	@uv run --package flokoa ruff check flokoa/src flokoa/tests
	@echo "🚀 Static type checking: Running ty"
	@cd flokoa && uv run ty check

.PHONY: build-flokoa
build-flokoa: ## Build flokoa wheel
	@echo "🚀 Building flokoa wheel"
	@cd flokoa && uvx --from build pyproject-build --installer uv

.PHONY: clean
clean: ## Clean build artifacts
	@echo "🚀 Removing build artifacts"
	@rm -rf flokoa/dist flokoa-managed-agent/dist flokoa-managed-task/dist flokoa-types/dist

# =============================================================================
# Docker — local build & run for managed-agent and managed-task
# =============================================================================

# Source .env from repo root (two levels up from sdk/python/)
-include ../../.env
export OPENAI_API_KEY

AGENT_IMAGE  ?= flokoa-managed-agent:local
TASK_IMAGE   ?= flokoa-managed-task:local
AGENT_CONFIG ?= samples/agent-config.json
TASK_CONFIG  ?= samples/task-config.json

.PHONY: test-e2e
test-e2e: docker-build ## Run e2e tests (requires OPENAI_API_KEY)
	@echo "🧪 Running e2e tests"
	@uv run --package flokoa-managed-agent pytest tests/e2e -v --timeout=120

.PHONY: docker-build
docker-build: docker-build-agent docker-build-task ## Build all Docker images

.PHONY: docker-build-agent
docker-build-agent: ## Build managed-agent Docker image
	@echo "🐳 Building $(AGENT_IMAGE)"
	@docker build -f flokoa-managed-agent/Dockerfile -t $(AGENT_IMAGE) .

.PHONY: docker-build-task
docker-build-task: ## Build managed-task Docker image
	@echo "🐳 Building $(TASK_IMAGE)"
	@docker build -f flokoa-managed-task/Dockerfile -t $(TASK_IMAGE) .

.PHONY: docker-run-agent
docker-run-agent: docker-build-agent ## Run managed-agent locally (A2A server on :8080)
	@echo "🐳 Running $(AGENT_IMAGE) — http://localhost:8080"
	@docker run --rm -p 8080:8080 \
		-e OPENAI_API_KEY \
		-v $(PWD)/$(AGENT_CONFIG):/etc/flokoa/agent-config.json:ro \
		$(AGENT_IMAGE)

.PHONY: docker-run-task
docker-run-task: docker-build-task ## Run managed-task locally (one-shot)
	@echo "🐳 Running $(TASK_IMAGE)"
	@docker run --rm \
		-e OPENAI_API_KEY \
		-v $(PWD)/$(TASK_CONFIG):/etc/flokoa/agent-config.json:ro \
		$(TASK_IMAGE)

.PHONY: help
help:
	@python -c "import re; \
	[[print(f'\033[36m{m[0]:<20}\033[0m {m[1]}') for m in re.findall(r'^([a-zA-Z_-]+):.*?## (.*)$$', open(makefile).read(), re.M)] for makefile in ('$(MAKEFILE_LIST)').strip().split()]"

.DEFAULT_GOAL := help
