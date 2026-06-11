"""Generate the AgentSpec JSON schema for the pinned pydantic-ai version.

Runs inside the runner environment (the runner image in CI, or the workspace
venv locally — both resolve the same pinned pydantic-ai via uv.lock), so the
schema matches the pin exactly. Platform capabilities are included via
``custom_capability_types`` so compiled specs containing operator-injected
entries validate against the same schema the runner loads them with.

Output is deterministic (sorted keys, trailing newline) so CI can diff it.

Usage: python hack/gen_agentspec_schema.py [output-path]
Default output: operator/internal/spec/schemas/agentspec-<runnerVersion>.json
"""

from __future__ import annotations

import json
import sys
import warnings
from pathlib import Path

from flokoa_runner import RUNNER_VERSION
from flokoa_runner.platform_capabilities import platform_capability_types


def generate_schema() -> dict:
    with warnings.catch_warnings():
        warnings.simplefilter("ignore")
        from pydantic_ai.agent.spec import AgentSpec

        return AgentSpec.model_json_schema_with_capabilities(
            custom_capability_types=platform_capability_types(),
        )


def default_output_path() -> Path:
    repo_root = Path(__file__).resolve().parents[4]
    return repo_root / "operator" / "internal" / "spec" / "schemas" / f"agentspec-{RUNNER_VERSION}.json"


def main() -> None:
    out = Path(sys.argv[1]) if len(sys.argv) > 1 else default_output_path()
    schema = generate_schema()
    out.parent.mkdir(parents=True, exist_ok=True)
    content = json.dumps(schema, indent=2, sort_keys=True) + "\n"
    out.write_text(content, encoding="utf-8")
    print(f"wrote {out} ({len(content)} bytes)")


if __name__ == "__main__":
    main()
