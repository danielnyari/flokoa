"""Topology extractor for pydantic-graph Graph instances.

Extracts the edges/adjacency list from a pydantic-graph Graph
by reading its `node_defs` dict — the same data structure used
by pydantic_graph.mermaid to generate Mermaid diagrams.

Supports both the stable API (pydantic_graph.Graph) and provides
helpers for detecting cycles and strongly connected components.

Usage:
    from pydantic_graph import BaseNode, End, Graph, GraphRunContext
    from dataclasses import dataclass

    @dataclass
    class Research(BaseNode[None]):
        async def run(self, ctx: GraphRunContext) -> "Review":
            return Review()

    @dataclass
    class Review(BaseNode[None, None, str]):
        async def run(self, ctx: GraphRunContext) -> Research | End[str]:
            return End("done")

    graph = Graph(nodes=(Research, Review))
    topo = extract_topology(graph)
    print(topo.adjacency)
    # {'Research': ['Review'], 'Review': ['Research', '__end__']}
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass
class GraphTopology:
    """Extracted topology from a pydantic-graph Graph instance."""

    node_ids: list[str]
    """All node IDs in the graph."""

    adjacency: dict[str, list[str]]
    """Adjacency list: node_id -> list of next node_ids.
    '__end__' is used as a sentinel for nodes that can terminate the graph."""

    end_nodes: set[str]
    """Node IDs that can produce an End (terminate the graph)."""

    edge_labels: dict[tuple[str, str], str | None]
    """Optional labels on edges: (from_id, to_id) -> label."""

    returns_base_node: dict[str, bool]
    """Node IDs where run() returns `BaseNode` (any node in the graph)."""


def extract_topology(graph: Any) -> GraphTopology:
    """Extract topology from a pydantic-graph Graph instance.

    Works with the stable pydantic_graph.Graph class by reading
    its `node_defs` attribute — a dict[str, NodeDef] where each
    NodeDef contains:
      - next_node_edges: dict[str, Edge] — outgoing edges to other nodes
      - end_edge: Edge | None — if the node can produce End
      - returns_base_node: bool — if run() returns BaseNode (wildcard)

    Args:
        graph: A pydantic_graph.Graph instance.

    Returns:
        A GraphTopology with the full adjacency structure.
    """
    node_ids: list[str] = []
    adjacency: dict[str, list[str]] = {}
    end_nodes: set[str] = set()
    edge_labels: dict[tuple[str, str], str | None] = {}
    returns_base_node_map: dict[str, bool] = {}

    for node_id, node_def in graph.node_defs.items():
        node_ids.append(node_id)
        neighbors: list[str] = []

        # If this node returns `BaseNode` directly, it can go to any node
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

        # Check if this node can end the graph
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


def find_cycles(topology: GraphTopology) -> list[list[str]]:
    """Find all cycles in the graph using DFS.

    Returns a list of cycles, where each cycle is a list of node IDs
    forming a path back to the starting node.
    """
    cycles: list[list[str]] = []
    visited: set[str] = set()
    rec_stack: set[str] = set()
    path: list[str] = []

    def dfs(node: str) -> None:
        visited.add(node)
        rec_stack.add(node)
        path.append(node)

        for neighbor in topology.adjacency.get(node, []):
            if neighbor == "__end__":
                continue
            if neighbor not in visited:
                dfs(neighbor)
            elif neighbor in rec_stack:
                # Found a cycle — extract it
                cycle_start = path.index(neighbor)
                cycles.append(path[cycle_start:] + [neighbor])

        path.pop()
        rec_stack.discard(node)

    for node in topology.node_ids:
        if node not in visited:
            dfs(node)

    return cycles


def find_strongly_connected_components(topology: GraphTopology) -> list[list[str]]:
    """Find strongly connected components using Tarjan's algorithm.

    SCCs with more than one node represent cycles that must be
    bundled as a single task when compiling to a DAG backend (Argo).
    """
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
    """Classify nodes for DAG compilation.

    Returns:
        - dag_nodes: Node IDs that are not in any cycle (safe for
          individual DAG tasks)
        - bundled_sccs: Strongly connected components with >1 node
          that must be bundled as a single task (run in-process)
    """
    sccs = find_strongly_connected_components(topology)
    dag_nodes: list[str] = []
    bundled_sccs: list[list[str]] = []

    for scc in sccs:
        if len(scc) == 1:
            # Single node — check for self-loop
            node = scc[0]
            if node in topology.adjacency.get(node, []):
                bundled_sccs.append(scc)
            else:
                dag_nodes.append(node)
        else:
            bundled_sccs.append(scc)

    return dag_nodes, bundled_sccs


def extract_beta_topology(graph: Any) -> dict[str, list[str]]:
    """Extract topology from a pydantic-graph beta Graph instance.

    The beta Graph stores topology differently:
      - graph.nodes: dict[NodeID, AnyNode]
      - graph.edges_by_source: dict[NodeID, list[Path]]

    Each Path contains DestinationMarker items pointing to the next node.

    Args:
        graph: A pydantic_graph.beta.graph.Graph instance.

    Returns:
        An adjacency dict: node_id -> list of next node_ids.
    """
    from pydantic_graph.beta.paths import DestinationMarker
    from pydantic_graph.beta.decision import Decision

    adjacency: dict[str, list[str]] = {}

    for node_id in graph.nodes:
        neighbors: list[str] = []

        # Collect destinations from regular edges
        for path in graph.edges_by_source.get(node_id, []):
            for item in path.items:
                if isinstance(item, DestinationMarker):
                    neighbors.append(str(item.destination_id))

        # Collect destinations from decision branches
        node = graph.nodes[node_id]
        if isinstance(node, Decision):
            for branch in node.branches:
                for item in branch.path.items:
                    if isinstance(item, DestinationMarker):
                        neighbors.append(str(item.destination_id))

        adjacency[str(node_id)] = neighbors

    return adjacency
