"""Minimal pydantic-graph -> Hera Workflow converter prototype.

Converts a pydantic-graph Graph into a Hera DAG Workflow.

Handles three cases:
  1. Linear graphs (A -> B -> C) — sequential DAG tasks
  2. Branching graphs (A -> B | C) — DAG tasks with `when` conditions
  3. Cyclic graphs (A <-> B) — bundled as a single in-process task

Strategy:
  - Each acyclic node becomes a Hera DAG task running a @script function.
  - State is passed as a JSON parameter between tasks.
  - Cyclic SCCs are bundled into a single @script that runs the subgraph
    in-process using Graph.run().
  - Conditional branches are modeled with Argo `when` expressions that
    check the upstream task's output parameter `next_node`.

Requirements:
  - hera-workflows >= 5.0
  - pydantic-graph

This is a PROTOTYPE for investigation purposes — not production code.
"""

from __future__ import annotations

import json
import textwrap
from dataclasses import dataclass
from typing import Any

from topology_extractor import (
    GraphTopology,
    classify_for_compilation,
    extract_topology,
)


@dataclass
class HeraDAGTask:
    """Intermediate representation of a Hera DAG task."""

    name: str
    """Task name (node ID, sanitized for Argo)."""

    depends: list[str]
    """Task names this task depends on."""

    when: str | None
    """Argo 'when' condition (for conditional branches)."""

    script_source: str
    """Python script source to run in the container."""

    image: str
    """Container image to use."""

    parameters: dict[str, str]
    """Input parameters for the task."""

    is_bundled: bool
    """Whether this is a bundled SCC (runs subgraph in-process)."""

    bundled_node_ids: list[str] | None
    """If bundled, the node IDs in this SCC."""


@dataclass
class HeraWorkflowSpec:
    """Intermediate representation of a full Hera Workflow."""

    name: str
    dag_tasks: list[HeraDAGTask]
    state_schema: str | None  # JSON schema of the state type


def sanitize_name(name: str) -> str:
    """Sanitize a node ID into a valid Argo task name."""
    return name.lower().replace(" ", "-").replace("_", "-")


def convert_graph_to_hera_spec(
    graph: Any,
    *,
    start_node_id: str,
    image: str = "flokoa/agent-runtime:latest",
    workflow_name: str | None = None,
) -> HeraWorkflowSpec:
    """Convert a pydantic-graph Graph into a Hera Workflow spec.

    Args:
        graph: A pydantic_graph.Graph instance.
        start_node_id: The ID of the node to start execution from.
        image: Container image with the user's code and dependencies.
        workflow_name: Name for the Argo Workflow.

    Returns:
        A HeraWorkflowSpec ready to be rendered to Hera API calls or YAML.
    """
    topology = extract_topology(graph)
    dag_node_ids, bundled_sccs = classify_for_compilation(topology)
    wf_name = workflow_name or (graph.name or "pydantic-graph-workflow")

    tasks: list[HeraDAGTask] = []

    # Track which nodes are in which bundled SCC
    node_to_bundle: dict[str, int] = {}
    for i, scc in enumerate(bundled_sccs):
        for node_id in scc:
            node_to_bundle[node_id] = i

    # Generate tasks for acyclic nodes
    for node_id in dag_node_ids:
        task = _make_node_task(
            node_id=node_id,
            topology=topology,
            start_node_id=start_node_id,
            image=image,
            node_to_bundle=node_to_bundle,
            bundled_sccs=bundled_sccs,
        )
        tasks.append(task)

    # Generate tasks for bundled SCCs
    for i, scc in enumerate(bundled_sccs):
        task = _make_bundled_task(
            scc=scc,
            scc_index=i,
            topology=topology,
            start_node_id=start_node_id,
            image=image,
            node_to_bundle=node_to_bundle,
            bundled_sccs=bundled_sccs,
        )
        tasks.append(task)

    return HeraWorkflowSpec(
        name=sanitize_name(wf_name),
        dag_tasks=tasks,
        state_schema=None,
    )


def _make_node_task(
    node_id: str,
    topology: GraphTopology,
    start_node_id: str,
    image: str,
    node_to_bundle: dict[str, int],
    bundled_sccs: list[list[str]],
) -> HeraDAGTask:
    """Create a DAG task for a single acyclic node."""
    depends = _compute_depends(
        node_id, topology, start_node_id, node_to_bundle, bundled_sccs
    )
    when = _compute_when_condition(node_id, topology)

    script_source = textwrap.dedent(f"""\
        import json
        import asyncio
        from pydantic_graph import GraphRunContext

        def main():
            state_json = '{{{{inputs.parameters.state}}}}'
            state = json.loads(state_json) if state_json else None

            # Import and instantiate the node
            # NOTE: In production, this would use a registry or dynamic import
            from user_graph import {node_id}
            node = {node_id}()  # Populated from upstream output

            ctx = GraphRunContext(state=state, deps=None)
            result = asyncio.get_event_loop().run_until_complete(node.run(ctx))

            # Output the result
            next_node = type(result).__name__
            output = {{"next_node": next_node, "state": json.dumps(state)}}
            print(json.dumps(output))

        main()
    """)

    return HeraDAGTask(
        name=sanitize_name(node_id),
        depends=depends,
        when=when,
        script_source=script_source,
        image=image,
        parameters={"state": "{{inputs.parameters.state}}"},
        is_bundled=False,
        bundled_node_ids=None,
    )


def _make_bundled_task(
    scc: list[str],
    scc_index: int,
    topology: GraphTopology,
    start_node_id: str,
    image: str,
    node_to_bundle: dict[str, int],
    bundled_sccs: list[list[str]],
) -> HeraDAGTask:
    """Create a DAG task for a bundled SCC (runs subgraph in-process)."""
    bundle_name = f"bundle-{'-'.join(sanitize_name(n) for n in scc)}"

    # Find dependencies: nodes outside this SCC that have edges into it
    depends: list[str] = []
    scc_set = set(scc)
    for node_id in scc:
        for other_id in topology.node_ids:
            if other_id in scc_set:
                continue
            if node_id in topology.adjacency.get(other_id, []):
                if other_id in node_to_bundle:
                    dep_bundle_idx = node_to_bundle[other_id]
                    dep_scc = bundled_sccs[dep_bundle_idx]
                    dep_name = f"bundle-{'-'.join(sanitize_name(n) for n in dep_scc)}"
                else:
                    dep_name = sanitize_name(other_id)
                if dep_name not in depends:
                    depends.append(dep_name)

    node_list_str = ", ".join(scc)
    script_source = textwrap.dedent(f"""\
        import json
        import asyncio

        def main():
            state_json = '{{{{inputs.parameters.state}}}}'
            state = json.loads(state_json) if state_json else None

            # Run the subgraph containing nodes: {node_list_str}
            # This bundles the cyclic component into a single in-process run
            from user_graph import graph, {", ".join(scc)}

            # Pick the entry node for this SCC
            entry_node = {scc[0]}()
            result = asyncio.get_event_loop().run_until_complete(
                graph.run(entry_node, state=state)
            )

            output = {{"result": str(result.output), "state": json.dumps(state)}}
            print(json.dumps(output))

        main()
    """)

    return HeraDAGTask(
        name=bundle_name,
        depends=depends,
        when=None,
        script_source=script_source,
        image=image,
        parameters={"state": "{{inputs.parameters.state}}"},
        is_bundled=True,
        bundled_node_ids=scc,
    )


def _compute_depends(
    node_id: str,
    topology: GraphTopology,
    start_node_id: str,
    node_to_bundle: dict[str, int],
    bundled_sccs: list[list[str]],
) -> list[str]:
    """Compute DAG task dependencies for a node."""
    depends: list[str] = []

    # A node depends on all nodes that have edges TO it
    for other_id in topology.node_ids:
        if node_id in topology.adjacency.get(other_id, []):
            if other_id in node_to_bundle:
                bundle_idx = node_to_bundle[other_id]
                scc = bundled_sccs[bundle_idx]
                dep_name = f"bundle-{'-'.join(sanitize_name(n) for n in scc)}"
            else:
                dep_name = sanitize_name(other_id)
            if dep_name not in depends:
                depends.append(dep_name)

    return depends


def _compute_when_condition(
    node_id: str, topology: GraphTopology
) -> str | None:
    """Compute an Argo 'when' condition for conditional branches.

    If a node has multiple predecessors that are different branches
    of the same parent, we need a `when` condition based on the
    parent's `next_node` output parameter.
    """
    # Find predecessors of this node
    predecessors: list[str] = []
    for other_id in topology.node_ids:
        neighbors = topology.adjacency.get(other_id, [])
        if node_id in neighbors and len(neighbors) > 1:
            # This predecessor has multiple outgoing edges (branching)
            predecessors.append(other_id)

    if not predecessors:
        return None

    # Generate when condition: the predecessor's next_node output must match
    # this node's ID
    conditions = []
    for pred in predecessors:
        pred_task = sanitize_name(pred)
        conditions.append(
            f"{{{{tasks.{pred_task}.outputs.parameters.next_node}}}} == {node_id}"
        )

    return " || ".join(conditions) if conditions else None


def render_hera_code(spec: HeraWorkflowSpec) -> str:
    """Render a HeraWorkflowSpec to Hera Python code.

    This generates the Python code that uses the Hera API
    to create the Argo Workflow.
    """
    lines = [
        "from hera.workflows import DAG, Workflow, script, Parameter",
        "",
        f'with Workflow(name="{spec.name}", entrypoint="main") as w:',
        '    with DAG(name="main"):',
    ]

    for task in spec.dag_tasks:
        task_lines = [
            f"",
            f"        # Task: {task.name}" + (
                f" (bundled SCC: {task.bundled_node_ids})" if task.is_bundled else ""
            ),
        ]

        deps_str = (
            f'depends="{" && ".join(task.depends)}"'
            if task.depends
            else ""
        )
        when_str = f'when="{task.when}"' if task.when else ""

        params = [f'name="{task.name}"']
        if deps_str:
            params.append(deps_str)
        if when_str:
            params.append(when_str)
        params.append(f'image="{task.image}"')

        task_lines.append(
            f"        @script({', '.join(params)})"
        )
        task_lines.append(f"        def {task.name.replace('-', '_')}():")
        for line in task.script_source.strip().split("\n"):
            task_lines.append(f"            {line}")

        lines.extend(task_lines)

    lines.extend([
        "",
        "# Generate YAML without submitting to Argo",
        "print(w.to_yaml())",
    ])

    return "\n".join(lines)


def render_argo_yaml(spec: HeraWorkflowSpec) -> dict[str, Any]:
    """Render a HeraWorkflowSpec to an Argo Workflow YAML dict.

    This generates the raw Argo Workflow spec that can be serialized
    to YAML and applied with `kubectl apply -f`.
    """
    dag_tasks = []
    templates = []

    for task in spec.dag_tasks:
        # DAG task entry
        dag_task: dict[str, Any] = {
            "name": task.name,
            "template": task.name,
        }
        if task.depends:
            dag_task["dependencies"] = task.depends
        if task.when:
            dag_task["when"] = task.when
        if task.parameters:
            dag_task["arguments"] = {
                "parameters": [
                    {"name": k, "value": v}
                    for k, v in task.parameters.items()
                ]
            }
        dag_tasks.append(dag_task)

        # Template definition
        template: dict[str, Any] = {
            "name": task.name,
            "inputs": {
                "parameters": [
                    {"name": k} for k in task.parameters
                ]
            },
            "script": {
                "image": task.image,
                "command": ["python"],
                "source": task.script_source,
            },
        }
        templates.append(template)

    # Main DAG template
    templates.insert(0, {
        "name": "main",
        "dag": {"tasks": dag_tasks},
    })

    return {
        "apiVersion": "argoproj.io/v1alpha1",
        "kind": "Workflow",
        "metadata": {"generateName": f"{spec.name}-"},
        "spec": {
            "entrypoint": "main",
            "templates": templates,
        },
    }
