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

.PHONY: help
help:
	@python -c "import re; \
	[[print(f'\033[36m{m[0]:<20}\033[0m {m[1]}') for m in re.findall(r'^([a-zA-Z_-]+):.*?## (.*)$$', open(makefile).read(), re.M)] for makefile in ('$(MAKEFILE_LIST)').strip().split()]"

.DEFAULT_GOAL := help
