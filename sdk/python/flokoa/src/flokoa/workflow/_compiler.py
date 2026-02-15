"""Compile a pydantic-graph ``Graph`` into an ``AgentWorkflowManifest``.

The compiler:
1. Extracts the graph topology (nodes + edges).
2. Classifies nodes into DAG-safe singles and cyclic bundles (SCCs).
3. Resolves fully-qualified class paths for each node.
4. Builds ``WorkflowStep`` objects with correct ``next`` / ``end`` edges.
5. Wraps everything in an ``AgentWorkflowManifest`` ready for YAML output.
"""

from __future__ import annotations

import inspect
import textwrap
from typing import Any

from flokoa.workflow._manifest import (
    AgentWorkflowManifest,
    AgentWorkflowSpec,
    WorkflowBundle,
    WorkflowStep,
)
from flokoa.workflow._topology import (
    GraphTopology,
    classify_for_compilation,
    extract_topology,
)


def compile_graph(
    graph: Any,
    *,
    name: str,
    image: str,
    namespace: str = "default",
    entrypoint: str | None = None,
    labels: dict[str, str] | None = None,
    inline: bool = False,
) -> AgentWorkflowManifest:
    """Compile a ``pydantic_graph.Graph`` into an ``AgentWorkflowManifest``.

    Args:
        graph: A ``pydantic_graph.Graph`` instance.
        name: Kubernetes resource name for the AgentWorkflow.
        image: Container image containing the user code, flokoa SDK,
            and pydantic-graph.  For ``inline=True`` a generic
            ``python:3.13-slim`` image is sufficient.
        namespace: Kubernetes namespace.
        entrypoint: Node name to start from.  Defaults to the first
            node in the graph.
        labels: Optional Kubernetes labels for the manifest metadata.
        inline: If ``True``, extract node class source code via
            ``inspect.getsource()`` and embed it in each step's
            ``source`` field.  The container image then only needs
            ``pydantic-graph`` installed — no user code required.

    Returns:
        An ``AgentWorkflowManifest`` that can be serialized to YAML/JSON
        and applied to the cluster.
    """
    topology = extract_topology(graph)
    dag_nodes, bundled_sccs = classify_for_compilation(topology)
    node_classes = _resolve_node_classes(graph)
    all_sources = _extract_all_node_sources(graph) if inline else None

    # Build a lookup: node_id -> bundle index (if cyclic)
    node_to_bundle: dict[str, int] = {}
    for i, scc in enumerate(bundled_sccs):
        for node_id in scc:
            node_to_bundle[node_id] = i

    steps: list[WorkflowStep] = []

    # Single-node steps
    for node_id in dag_nodes:
        step = _build_single_step(
            node_id=node_id,
            topology=topology,
            node_classes=node_classes,
            node_to_bundle=node_to_bundle,
            bundled_sccs=bundled_sccs,
            source=all_sources,
        )
        steps.append(step)

    # Bundled SCC steps
    for scc in bundled_sccs:
        step = _build_bundle_step(
            scc=scc,
            topology=topology,
            node_classes=node_classes,
            node_to_bundle=node_to_bundle,
            bundled_sccs=bundled_sccs,
            source=all_sources,
        )
        steps.append(step)

    # Determine entrypoint
    if entrypoint is None:
        entrypoint = topology.node_ids[0]
    # Map entrypoint to step name (might be inside a bundle)
    if entrypoint in node_to_bundle:
        idx = node_to_bundle[entrypoint]
        entrypoint_step = _bundle_name(bundled_sccs[idx])
    else:
        entrypoint_step = entrypoint

    metadata: dict[str, Any] = {"name": name, "namespace": namespace}
    if labels:
        metadata["labels"] = labels

    return AgentWorkflowManifest(
        metadata=metadata,
        spec=AgentWorkflowSpec(
            entrypoint=entrypoint_step,
            image=image,
            steps=steps,
        ),
    )


# ------------------------------------------------------------------
# Internal helpers
# ------------------------------------------------------------------


def _resolve_node_classes(graph: Any) -> dict[str, str]:
    """Map node IDs to fully-qualified ``module.ClassName`` paths.

    Tries multiple approaches to extract the class references from
    the graph, falling back to bare names if the graph API doesn't
    expose them.
    """
    classes: dict[str, str] = {}

    # Approach 1: NodeDef stores the node class directly
    for node_id, node_def in graph.node_defs.items():
        cls = getattr(node_def, "node", None) or getattr(node_def, "node_cls", None)
        if cls is not None:
            classes[node_id] = f"{cls.__module__}.{cls.__qualname__}"

    if classes:
        return classes

    # Approach 2: Graph exposes an iterable of node types
    node_types = getattr(graph, "node_types", None) or getattr(graph, "_node_types", None)
    if node_types:
        if isinstance(node_types, dict):
            return {k: f"{v.__module__}.{v.__qualname__}" for k, v in node_types.items()}
        for cls in node_types:
            classes[cls.__name__] = f"{cls.__module__}.{cls.__qualname__}"
        return classes

    # Fallback: just the class name (user will need to ensure the
    # module is importable in the container)
    return {node_id: node_id for node_id in graph.node_defs}


def _bundle_name(scc: list[str]) -> str:
    """Deterministic name for a bundled SCC step."""
    return "-".join(sorted(scc))


def _outgoing_next(
    node_id: str,
    topology: GraphTopology,
    node_to_bundle: dict[str, int],
    bundled_sccs: list[list[str]],
) -> tuple[list[str], bool]:
    """Compute ``next`` step names and ``end`` flag for a node.

    Edges to nodes inside the *same* bundle are skipped (they're
    handled in-process).  Edges to nodes in *other* bundles map to the
    bundle step name.
    """
    next_steps: list[str] = []
    can_end = False

    own_bundle = node_to_bundle.get(node_id)

    for neighbor in topology.adjacency.get(node_id, []):
        if neighbor == "__end__":
            can_end = True
            continue

        neighbor_bundle = node_to_bundle.get(neighbor)

        # Skip intra-bundle edges
        if own_bundle is not None and neighbor_bundle == own_bundle:
            continue

        step_name = _bundle_name(bundled_sccs[neighbor_bundle]) if neighbor_bundle is not None else neighbor

        if step_name not in next_steps:
            next_steps.append(step_name)

    return next_steps, can_end


def _build_single_step(
    node_id: str,
    topology: GraphTopology,
    node_classes: dict[str, str],
    node_to_bundle: dict[str, int],
    bundled_sccs: list[list[str]],
    source: str | None = None,
) -> WorkflowStep:
    """Build a WorkflowStep for a single acyclic node."""
    next_steps, can_end = _outgoing_next(
        node_id, topology, node_to_bundle, bundled_sccs
    )
    return WorkflowStep(
        name=node_id,
        node_class=node_classes.get(node_id, node_id),
        source=source,
        next=next_steps,
        end=can_end,
    )


def _build_bundle_step(
    scc: list[str],
    topology: GraphTopology,
    node_classes: dict[str, str],
    node_to_bundle: dict[str, int],
    bundled_sccs: list[list[str]],
    source: str | None = None,
) -> WorkflowStep:
    """Build a WorkflowStep for a bundled SCC."""
    step_name = _bundle_name(scc)

    # Collect all outgoing edges from the SCC to external steps
    all_next: list[str] = []
    any_end = False
    for node_id in scc:
        next_steps, can_end = _outgoing_next(
            node_id, topology, node_to_bundle, bundled_sccs
        )
        for s in next_steps:
            if s not in all_next:
                all_next.append(s)
        any_end = any_end or can_end

    return WorkflowStep(
        name=step_name,
        bundle=WorkflowBundle(
            node_classes=[node_classes.get(n, n) for n in scc],
            entrypoint=scc[0],
        ),
        source=source,
        next=all_next,
        end=any_end,
    )


def _get_node_class_objects(graph: Any) -> dict[str, type]:
    """Extract actual class objects from the graph."""
    classes: dict[str, type] = {}

    for node_id, node_def in graph.node_defs.items():
        cls = getattr(node_def, "node", None) or getattr(node_def, "node_cls", None)
        if cls is not None:
            classes[node_id] = cls

    if classes:
        return classes

    node_types = getattr(graph, "node_types", None) or getattr(graph, "_node_types", None)
    if node_types:
        if isinstance(node_types, dict):
            return dict(node_types)
        return {cls.__name__: cls for cls in node_types}

    return {}


def _extract_all_node_sources(graph: Any) -> str:
    """Extract Python source for every node class in the graph.

    Every step gets the *full* set of class sources because return-type
    annotations reference sibling node classes
    (e.g. ``FetchData.run() -> ProcessData``).

    Raises ``TypeError`` if source cannot be retrieved for a node
    (e.g. defined interactively or in a C extension).
    """
    classes = _get_node_class_objects(graph)
    if not classes:
        msg = (
            "Cannot extract inline source: unable to resolve node "
            "class objects from the graph.  Make sure you are using "
            "a pydantic-graph version that exposes node classes on NodeDef."
        )
        raise TypeError(msg)

    sources: list[str] = []
    seen: set[type] = set()

    for node_id in graph.node_defs:
        cls = classes.get(node_id)
        if cls is None or cls in seen:
            continue
        seen.add(cls)
        try:
            src = textwrap.dedent(inspect.getsource(cls))
        except OSError as exc:
            msg = (
                f"Cannot retrieve source for {node_id!r} "
                f"({cls.__module__}.{cls.__qualname__}).  "
                f"Inline mode requires node classes defined in .py files."
            )
            raise TypeError(msg) from exc
        sources.append(src)

    return "\n\n".join(sources)
