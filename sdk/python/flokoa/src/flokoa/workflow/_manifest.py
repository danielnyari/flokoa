"""AgentWorkflow manifest models and serialization.

Defines the Pydantic models that represent an ``AgentWorkflow`` custom
resource.  The manifest can be serialized to a Python dict, YAML string,
or written to disk — no running cluster required.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from pydantic import BaseModel, ConfigDict, Field


class WorkflowBundle(BaseModel):
    """A cyclic subgraph that runs in-process as a single step."""

    model_config = ConfigDict(populate_by_name=True)

    node_classes: list[str] = Field(alias="nodeClasses")
    """Fully-qualified class paths of the nodes in the cycle."""

    entrypoint: str
    """Node to enter the cycle from."""


class WorkflowStep(BaseModel):
    """One step in an AgentWorkflow.

    A step is *either* a single-node step (``node_class`` set) or a
    bundled cycle (``bundle`` set), never both.
    """

    model_config = ConfigDict(populate_by_name=True)

    name: str
    """Step name (derived from the node class name)."""

    node_class: str | None = Field(default=None, alias="nodeClass")
    """Fully-qualified class path for single-node steps."""

    bundle: WorkflowBundle | None = None
    """Present when this step wraps a cyclic subgraph."""

    next: list[str] = []
    """Names of steps this step can transition to."""

    end: bool = False
    """Whether this step can terminate the workflow."""


class AgentWorkflowSpec(BaseModel):
    """Spec section of an AgentWorkflow manifest."""

    model_config = ConfigDict(populate_by_name=True)

    entrypoint: str
    """Step name to begin execution from."""

    image: str
    """Container image containing the user code + dependencies."""

    steps: list[WorkflowStep]
    """All steps in the workflow."""


class AgentWorkflowManifest(BaseModel):
    """A complete AgentWorkflow custom-resource manifest.

    Mirrors the shape of a Kubernetes CR so it can be applied directly
    with ``kubectl apply -f``.
    """

    model_config = ConfigDict(populate_by_name=True)

    api_version: str = Field(default="agent.flokoa.ai/v1alpha1", alias="apiVersion")
    kind: str = "AgentWorkflow"
    metadata: dict[str, Any]
    spec: AgentWorkflowSpec

    # ------------------------------------------------------------------
    # Serialization helpers
    # ------------------------------------------------------------------

    def to_dict(self) -> dict[str, Any]:
        """Return the manifest as a plain dict (camelCase keys)."""
        return self.model_dump(by_alias=True, exclude_none=True)

    def to_yaml(self) -> str:
        """Render the manifest as a YAML string.

        Requires ``pyyaml`` to be installed (included in the
        ``flokoa[workflow]`` extra).
        """
        try:
            import yaml
        except ImportError as exc:
            raise ImportError(
                "pyyaml is required for YAML output. "
                "Install it with: pip install flokoa[workflow]"
            ) from exc

        return yaml.dump(
            self.to_dict(),
            default_flow_style=False,
            sort_keys=False,
        )

    def to_json(self, *, indent: int = 2) -> str:
        """Render the manifest as a JSON string."""
        return json.dumps(self.to_dict(), indent=indent)

    def to_file(self, path: str | Path) -> Path:
        """Write the manifest to a YAML file.

        Returns the resolved path that was written.
        """
        dest = Path(path)
        dest.write_text(self.to_yaml())
        return dest.resolve()
