# CLAUDE.md - Flokoa

This document provides guidance for AI assistants working with the Flokoa codebase.

## Project Overview

Flokoa is a platform for managing AI Agents in Kubernetes clusters. It consists of:

1. **Kubernetes Operator** - Declarative deployment and lifecycle management of AI agents through CRDs
2. **Python SDK** - Client library and CLI for building and running agents locally

- **Domain**: `flokoa.ai`
- **Repository**: `github.com/danielnyari/flokoa`
- **License**: Apache 2.0

## Monorepo Structure

```
flokoa/
├── CLAUDE.md              # This file (monorepo overview)
├── operator/              # Kubernetes Operator (Go)
│   └── CLAUDE.md          # Operator-specific guidance
├── sdk/
│   └── python/            # Python SDK (uv workspace)
│       ├── pyproject.toml         # Workspace root
│       ├── flokoa/                # Public SDK package
│       │   └── CLAUDE.md          # SDK-specific guidance
│       ├── flokoa-managed-agent/  # Operator-deployed pydantic-ai agent runtime
│       └── flokoa-managed-task/   # Operator-deployed Marvin task runtime (scaffold)
├── docs/                  # Documentation
│   └── examples/          # Example configurations
└── .github/workflows/     # CI/CD pipelines
```

## Module-Specific Guidance

Each module has its own CLAUDE.md with detailed instructions:

- **Operator**: See `operator/CLAUDE.md` for Go/Kubebuilder development
- **Python SDK**: See `sdk/python/flokoa/CLAUDE.md` for Python development

## Cross-Module Concepts

### Agent CRD and SDK Alignment

The Python SDK is designed to work with the Kubernetes Operator:

- Agents built with the SDK can be deployed via the Operator
- The `flokoa` CLI can run agents locally for development
- Framework detection (pydantic-ai, langchain, etc.) is shared between components

### API Group

- **API Group**: `agent.flokoa.ai`
- **Current Version**: `v1alpha1`

## Project Status

This project is in early development (v0.0.1).
