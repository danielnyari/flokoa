"""Topology extraction from pydantic-graph Graph instances.

Reads ``graph.node_defs`` to build an adjacency list, then classifies
nodes into DAG-safe singles and cyclic bundles (strongly connected
components) so the compiler knows what can become an independent
workflow step vs. what must run in-process.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass
class GraphTopology:
    """Extracted topology from a pydantic-graph Graph."""

    node_ids: list[str]
    adjacency: dict[str, list[str]]
    end_nodes: set[str]
    edge_labels: dict[tuple[str, str], str | None] = field(default_factory=dict)
    returns_base_node: dict[str, bool] = field(default_factory=dict)


def extract_topology(graph: Any) -> GraphTopology:
    """Extract adjacency structure from a ``pydantic_graph.Graph``.

    Reads ``graph.node_defs`` — a ``dict[str, NodeDef]`` where each
    ``NodeDef`` exposes ``next_node_edges``, ``end_edge``, and
    ``returns_base_node``.
    """
    node_ids: list[str] = []
    adjacency: dict[str, list[str]] = {}
    end_nodes: set[str] = set()
    edge_labels: dict[tuple[str, str], str | None] = {}
    returns_base_node_map: dict[str, bool] = {}

    for node_id, node_def in graph.node_defs.items():
        node_ids.append(node_id)
        neighbors: list[str] = []

        if node_def.returns_base_node:
            returns_base_node_map[node_id] = True
            for other_id in graph.node_defs:
                neighbors.append(other_id)
                edge_labels[(node_id, other_id)] = None
        else:
            returns_base_node_map[node_id] = False
            for next_id, edge in node_def.next_node_edges.items():
                neighbors.append(next_id)
                edge_labels[(node_id, next_id)] = edge.label

        if node_def.end_edge is not None:
            end_nodes.add(node_id)
            neighbors.append("__end__")
            edge_labels[(node_id, "__end__")] = node_def.end_edge.label

        adjacency[node_id] = neighbors

    return GraphTopology(
        node_ids=node_ids,
        adjacency=adjacency,
        end_nodes=end_nodes,
        edge_labels=edge_labels,
        returns_base_node=returns_base_node_map,
    )


def _find_sccs(topology: GraphTopology) -> list[list[str]]:  # noqa: C901
    """Tarjan's algorithm for strongly connected components."""
    index_counter = [0]
    stack: list[str] = []
    lowlinks: dict[str, int] = {}
    index: dict[str, int] = {}
    on_stack: set[str] = set()
    sccs: list[list[str]] = []

    def strongconnect(v: str) -> None:
        index[v] = index_counter[0]
        lowlinks[v] = index_counter[0]
        index_counter[0] += 1
        stack.append(v)
        on_stack.add(v)

        for w in topology.adjacency.get(v, []):
            if w == "__end__":
                continue
            if w not in index:
                strongconnect(w)
                lowlinks[v] = min(lowlinks[v], lowlinks[w])
            elif w in on_stack:
                lowlinks[v] = min(lowlinks[v], index[w])

        if lowlinks[v] == index[v]:
            scc: list[str] = []
            while True:
                w = stack.pop()
                on_stack.discard(w)
                scc.append(w)
                if w == v:
                    break
            sccs.append(scc)

    for v in topology.node_ids:
        if v not in index:
            strongconnect(v)

    return sccs


def classify_for_compilation(
    topology: GraphTopology,
) -> tuple[list[str], list[list[str]]]:
    """Split nodes into DAG-safe singles and cyclic bundles.

    Returns:
        dag_nodes: Node IDs safe for individual workflow steps.
        bundled_sccs: SCCs with >1 node (or self-loops) that must
            be bundled into a single in-process step.
    """
    sccs = _find_sccs(topology)
    dag_nodes: list[str] = []
    bundled_sccs: list[list[str]] = []

    for scc in sccs:
        if len(scc) == 1:
            node = scc[0]
            if node in topology.adjacency.get(node, []):
                bundled_sccs.append(scc)
            else:
                dag_nodes.append(node)
        else:
            bundled_sccs.append(scc)

    return dag_nodes, bundled_sccs
