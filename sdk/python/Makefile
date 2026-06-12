.PHONY: install
install: ## Install workspace dependencies and pre-commit hooks
	@echo "🚀 Creating virtual environment using uv"
	@uv sync --all-packages
	@uv run pre-commit install

.PHONY: test
test: test-flokoa test-runner test-codemode test-common test-openapi ## Run all tests

.PHONY: test-flokoa
test-flokoa: ## Run flokoa tests
	@echo "🚀 Testing flokoa"
	@cd flokoa && uv run pytest tests --cov --cov-config=pyproject.toml --cov-report=xml

.PHONY: test-runner
test-runner: ## Run flokoa-runner tests
	@echo "🚀 Testing flokoa-runner"
	@uv run --package flokoa-runner pytest flokoa-runner/tests

.PHONY: test-codemode
test-codemode: ## Run flokoa-codemode-mcp tests
	@echo "🚀 Testing flokoa-codemode-mcp"
	@uv run --package flokoa-codemode-mcp pytest flokoa-codemode-mcp/tests

.PHONY: test-common
test-common: ## Run flokoa-common tests
	@echo "🚀 Testing flokoa-common"
	@uv run --package flokoa-common pytest flokoa-common/tests

.PHONY: test-openapi
test-openapi: ## Run flokoa-openapi tests
	@echo "🚀 Testing flokoa-openapi"
	@uv run --package flokoa-openapi pytest flokoa-openapi/tests

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

# =============================================================================
# Runtime contract (docs/reference/runtime-contract.md) — pin/lock/manifest/schema
# =============================================================================

.PHONY: runner-lock
runner-lock: ## Export the runner baseline lockfile (runner.lock)
	@echo "🔒 Exporting flokoa-runner lockfile"
	@uv export --package flokoa-runner --no-dev --no-emit-workspace --no-hashes \
		--format requirements.txt -o flokoa-runner/runner.lock -q

.PHONY: gen-agentspec-schema
gen-agentspec-schema: ## Generate the AgentSpec JSON schema for the pinned pydantic-ai
	@echo "📜 Generating AgentSpec JSON schema"
	@uv run --package flokoa-runner python flokoa-runner/hack/gen_agentspec_schema.py

.PHONY: runner-manifest
runner-manifest: ## Generate runner-manifest.json from runner.lock + schema
	@echo "🧾 Generating runner manifest"
	@uv run --package flokoa-runner python flokoa-runner/hack/gen_runner_manifest.py

.PHONY: runner-contract
runner-contract: runner-lock gen-agentspec-schema runner-manifest ## Regenerate lockfile + schema + manifest coherently

.PHONY: verify-runner-contract
verify-runner-contract: runner-contract ## CI gate: fail on drift between pin, lock, manifest, and embedded schema
	@# intent-to-add so a NEW runner version's untracked baseline/schema is
	@# caught by git diff (which otherwise ignores untracked files).
	@git add -N flokoa-runner/runner.lock flokoa-runner/runner-manifest.json \
		../../operator/internal/spec/schemas \
		../../operator/internal/spec/baselines 2>/dev/null || true
	@git diff --exit-code -- flokoa-runner/runner.lock flokoa-runner/runner-manifest.json \
		../../operator/internal/spec/schemas \
		../../operator/internal/spec/baselines \
		|| (echo "❌ runner contract artifacts drifted (or a new one is uncommitted) — run 'make runner-contract' and commit" && exit 1)
	@! grep -i "pydantic-ai-harness" flokoa-runner/runner.lock \
		|| (echo "❌ pydantic-ai-harness must not be in the runner baseline (brief §4)" && exit 1)
	@echo "✅ runner contract artifacts are coherent"

.PHONY: clean
clean: ## Clean build artifacts
	@echo "🚀 Removing build artifacts"
	@rm -rf flokoa/dist flokoa-runner/dist flokoa-types/dist

# =============================================================================
# Docker — local build & run for the generic runner
# =============================================================================

# Source .env from repo root (two levels up from sdk/python/)
-include ../../.env
export OPENAI_API_KEY

RUNNER_IMAGE ?= flokoa-runner:local
AGENT_SPEC   ?= samples/agent-spec.yaml

.PHONY: docker-build
docker-build: docker-build-runner ## Build all Docker images

.PHONY: docker-build-runner
docker-build-runner: ## Build the generic runner Docker image
	@echo "🐳 Building $(RUNNER_IMAGE)"
	@docker build -f flokoa-runner/Dockerfile -t $(RUNNER_IMAGE) .

.PHONY: docker-run-runner
docker-run-runner: docker-build-runner ## Run the runner locally (A2A server on :8080)
	@echo "🐳 Running $(RUNNER_IMAGE) — http://localhost:8080"
	@docker run --rm -p 8080:8080 \
		-e OPENAI_API_KEY \
		-v $(PWD)/$(AGENT_SPEC):/etc/flokoa/agent-spec.yaml:ro \
		-v $(PWD)/samples/agent-card.json:/etc/flokoa/agent-card.json:ro \
		$(RUNNER_IMAGE)

.PHONY: help
help:
	@python -c "import re; \
	[[print(f'\033[36m{m[0]:<20}\033[0m {m[1]}') for m in re.findall(r'^([a-zA-Z_-]+):.*?## (.*)$$', open(makefile).read(), re.M)] for makefile in ('$(MAKEFILE_LIST)').strip().split()]"

.DEFAULT_GOAL := help
