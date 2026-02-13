---
name: generate-python-types
description: Generate Python Pydantic v2 models from Kubernetes Operator CRD schemas. Use when the user changes Go CRD types, asks to sync Python SDK types, or needs to add new generated types to the Python SDK.
---

# Generate Python Types from CRD Schemas

## Quick Start

From the `operator/` directory, regenerate all Python types:

```bash
make manifests generate generate-python-models
```

This three-step process ensures:
1. `make manifests` - CRD YAML files are up to date with Go types
2. `make generate` - DeepCopy methods are generated
3. `make generate-python-models` - Python Pydantic models are extracted from CRD schemas

## What Gets Generated

| Output File | Source CRD | Root Class | Description |
|------------|-----------|------------|-------------|
| `sdk/python/src/flokoa/types/agenttool.py` | `agent.flokoa.ai_agenttools` | `AgentToolSpec` | Tool definitions (OpenAPI spec, service refs) |
| `sdk/python/src/flokoa/types/agentcard.py` | `agent.flokoa.ai_agents` (card) | `AgentCard` | A2A protocol card (skills, capabilities) |
| `sdk/python/src/flokoa/types/modelconfig.py` | Combined Models + Providers | `ModelConfig` | Full model config (provider + parameters) |
| `sdk/python/src/flokoa/types/templateconfig.py` | `agent.flokoa.ai_agents` (template) | `TemplateConfig` | Managed runtime configuration |

## How It Works

The generation pipeline uses `yq` (YAML processor) and `datamodel-codegen`:

1. **Extract schema**: `yq eval` pulls the relevant JSON schema section from the CRD YAML
2. **Add titles**: `yq eval -i` adds `title` fields to nested types to prevent naming collisions (e.g., `Type`, `Type1`, `Type2`)
3. **Generate Python**: `datamodel-codegen` converts JSON schema to Pydantic v2 BaseModel classes

Key `datamodel-codegen` flags used:
- `--output-model-type pydantic_v2.BaseModel` - Generate Pydantic v2 models
- `--snake-case-field` - Convert camelCase to snake_case
- `--use-annotated` - Use `Annotated` type hints
- `--allow-population-by-field-name` - Allow both JSON and Python field names
- `--use-title-as-name` - Use YAML `title` for class naming (avoids collisions)

## Prerequisites

- **yq**: Install with `brew install yq` (macOS) or see https://github.com/mikefarah/yq
- **Python 3**: The Makefile creates a venv at `operator/bin/venv` automatically
- **datamodel-code-generator**: Installed automatically in the venv

## Adding a New Generated Type

To generate a new Python type from an existing or new CRD:

### 1. Ensure the Go types and CRD YAML exist

Edit `operator/api/v1alpha1/<name>_types.go` and run:
```bash
make manifests generate
```

### 2. Add extraction to the Makefile

Add a new section to the `generate-python-models` target in `operator/Makefile`. Follow the existing pattern:

```makefile
	@# Extract <NewType> schema
	@yq eval '.spec.versions[0].schema.openAPIV3Schema.properties.spec' \
		config/crd/bases/agent.flokoa.ai_<plural>.yaml > /tmp/<newtype>_schema.json
	@# Add title to avoid naming collisions (if needed)
	@yq eval -i -o json '.title = "<NewType>"' /tmp/<newtype>_schema.json
	@# Generate Pydantic models
	@$(DATAMODEL_CODEGEN) \
		--input /tmp/<newtype>_schema.json \
		--input-file-type jsonschema \
		--output-model-type pydantic_v2.BaseModel \
		--snake-case-field \
		--use-annotated \
		--allow-population-by-field-name \
		--class-name <NewType> \
		--output $(PYTHON_MODELS_DIR)/<newtype>.py
	@rm /tmp/<newtype>_schema.json
```

### 3. Handle nested type naming collisions

If the schema has multiple objects with a `type` field (common in Kubernetes), add explicit titles:

```makefile
	@yq eval -i '.properties.<nested>.title = "<UniqueName>"' /tmp/<schema>.json
	@yq eval -i '.properties.<nested>.properties.type.title = "<UniqueTypeName>"' /tmp/<schema>.json
```

Use `--use-title-as-name` flag when types would otherwise collide.

### 4. For combined schemas (like ModelConfig)

When combining fields from multiple CRDs:

1. Extract individual schemas with `yq`
2. Build a composite schema YAML with `yq -n '...'`
3. Inject sub-schemas with `yq eval -i '.properties.<field> = load("/tmp/<sub>.json")'`
4. Convert to JSON and generate

See the `modelconfig` section in the Makefile for the full pattern.

### 5. Run generation

```bash
make generate-python-models
```

### 6. Verify output

Check the generated file in `sdk/python/src/flokoa/types/`. The generated code should:
- Have proper Pydantic v2 BaseModel classes
- Use snake_case field names with camelCase aliases
- Have `Optional` types for optional fields
- Have `Annotated` types with `Field` metadata

## Important Notes

- **Never edit files in `sdk/python/src/flokoa/types/` manually** - they will be overwritten
- Always run `make manifests` before `make generate-python-models` to ensure CRD YAML is current
- The `modelconfig.py` generation is the most complex due to combining multiple CRDs
- If you add new enum types in Go, ensure the `title` is set in the extraction to get proper Python enum names
