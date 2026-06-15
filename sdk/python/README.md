# Flokoa Python SDK (uv workspace)

This directory is the Python side of [Flokoa](https://github.com/danielnyari/flokoa):
a [uv](https://docs.astral.sh/uv/) workspace whose published member is the
`flokoa` package (CLI + A2A serving library), alongside the internal packages the
in-cluster runtime is built from.

Only **`flokoa`** is published to PyPI. See its [README](flokoa/README.md) for
user-facing install and usage, and the
[Python SDK & CLI guide](../../docs/sdk.md) for the full surface.

## Packages

| Package | Published? | Role |
|---|---|---|
| [`flokoa`](flokoa/) | ✅ PyPI | Public SDK: the `flokoa` CLI (`run`, `capability`) and the A2A serving/context/telemetry library. |
| [`flokoa-types`](flokoa-types/) | — | Auto-generated Pydantic v2 models from the CRD schemas (`make generate-python-models` in `operator/`). Do not edit by hand. |
| [`flokoa-runner`](flokoa-runner/) | — | The generic runner baked into the operator's runner image; hydrates a compiled spec via `Agent.from_spec`. Owns the `pydantic-ai==1.107.0` baseline pin. |
| [`flokoa-codemode-mcp`](flokoa-codemode-mcp/) | — | Code-mode MCP server (LLM-generated code executed against OpenAPI specs in a sandbox). |
| [`flokoa-openapi`](flokoa-openapi/) | — | Turns an OpenAPI document into typed pydantic-ai tools; ships as a Capability artifact. |
| [`flokoa-common`](flokoa-common/) | — | Shared internals (OpenAPI parsing, auth, URL/SSRF validation). Not a public API. |

## Requirements

- **Python ≥ 3.13**
- [uv](https://docs.astral.sh/uv/) for dependency management

## Common commands

```bash
# Workspace-wide
uv sync --all-packages --all-extras   # sync every member with all extras
uv lock                               # update the shared lockfile

# The public package (from sdk/python/flokoa/)
make install   # venv, deps, pre-commit hooks
make check     # lock + lint + type check
make test      # pytest with coverage
make build     # build the wheel
```

## Regenerating CRD types

`flokoa-types` is generated from the operator's CRD schemas. After changing Go
types in `operator/api/v1alpha1/`, run from `operator/`:

```bash
make manifests generate generate-python-models
```

## Contributing

Development notes for AI assistants and engineers live in
[`flokoa/CLAUDE.md`](flokoa/CLAUDE.md) (symlinked as `AGENTS.md`). The normative
operator↔runner interface is the
[runtime contract](../../docs/reference/runtime-contract.md).
