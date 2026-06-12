# Flokoa monorepo — convenience entrypoints that delegate to operator/Makefile.
# The real targets live in operator/Makefile; these just save you a `cd`.

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show the common monorepo commands.
	@echo "Flokoa — common commands:"
	@echo "  make up          Boot the whole stack on local minikube (build, deploy, port-forward UIs)"
	@echo "  make down        Tear down: stop port-forwards + undeploy (ARGS=--stop-minikube to also stop the VM)"
	@echo "  make urls        Print the local UI URLs"
	@echo ""
	@echo "  'make up' reads OPENAI_API_KEY from ./.env. See operator/Makefile for the full target list."

.PHONY: up
up: ## Boot the whole stack on local minikube.
	@$(MAKE) -C operator local-up

.PHONY: down
down: ## Tear down the local stack. ARGS=--forwards-only | --stop-minikube
	@$(MAKE) -C operator local-down ARGS="$(ARGS)"

.PHONY: urls
urls: ## Print the local UI URLs.
	@$(MAKE) -C operator local-urls
