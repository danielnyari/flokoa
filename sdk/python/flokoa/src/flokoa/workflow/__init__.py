"""Compile pydantic-graph graphs into AgentWorkflow manifests.

Usage::

    from pydantic_graph import Graph, BaseNode, End
    from flokoa.workflow import compile_graph

    graph = Graph(nodes=[FetchData, ProcessData])

    manifest = compile_graph(
        graph,
        name="my-pipeline",
        image="my-registry/my-agent:1.0",
    )

    print(manifest.to_yaml())
    manifest.to_file("my-pipeline.yaml")
"""

from flokoa.workflow._compiler import compile_graph
from flokoa.workflow._manifest import (
    AgentWorkflowManifest,
    AgentWorkflowSpec,
    WorkflowBundle,
    WorkflowStep,
)

__all__ = [
    "AgentWorkflowManifest",
    "AgentWorkflowSpec",
    "WorkflowBundle",
    "WorkflowStep",
    "compile_graph",
]
