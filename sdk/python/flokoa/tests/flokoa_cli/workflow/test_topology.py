"""Tests for workflow topology extraction and SCC classification."""

from __future__ import annotations

from dataclasses import dataclass

from pydantic_graph import BaseNode, End, Graph, GraphRunContext

from flokoa.workflow._topology import (
    classify_for_compilation,
    extract_topology,
)

# -- Test graphs -----------------------------------------------------------


@dataclass
class FetchData(BaseNode[None]):
    async def run(self, ctx: GraphRunContext) -> ProcessData:
        return ProcessData()


@dataclass
class ProcessData(BaseNode[None, None, str]):
    async def run(self, ctx: GraphRunContext) -> End[str]:
        return End("done")


linear_graph = Graph(nodes=[FetchData, ProcessData])


@dataclass
class StepA(BaseNode[None]):
    async def run(self, ctx: GraphRunContext) -> StepB | StepC:
        return StepB()


@dataclass
class StepB(BaseNode[None, None, str]):
    async def run(self, ctx: GraphRunContext) -> End[str]:
        return End("b")


@dataclass
class StepC(BaseNode[None, None, str]):
    async def run(self, ctx: GraphRunContext) -> End[str]:
        return End("c")


branching_graph = Graph(nodes=[StepA, StepB, StepC])


@dataclass
class Research(BaseNode[None]):
    async def run(self, ctx: GraphRunContext) -> Review:
        return Review()


@dataclass
class Review(BaseNode[None, None, str]):
    async def run(self, ctx: GraphRunContext) -> Research | End[str]:
        return End("approved")


cyclic_graph = Graph(nodes=[Research, Review])


# -- Topology extraction ---------------------------------------------------


class TestExtractTopology:
    def test_linear_graph_nodes(self):
        topo = extract_topology(linear_graph)
        assert set(topo.node_ids) == {"FetchData", "ProcessData"}

    def test_linear_graph_adjacency(self):
        topo = extract_topology(linear_graph)
        assert topo.adjacency["FetchData"] == ["ProcessData"]
        assert "__end__" in topo.adjacency["ProcessData"]

    def test_linear_graph_end_nodes(self):
        topo = extract_topology(linear_graph)
        assert topo.end_nodes == {"ProcessData"}

    def test_branching_graph_adjacency(self):
        topo = extract_topology(branching_graph)
        assert set(topo.adjacency["StepA"]) == {"StepB", "StepC"}

    def test_cyclic_graph_adjacency(self):
        topo = extract_topology(cyclic_graph)
        assert "Review" in topo.adjacency["Research"]
        assert "Research" in topo.adjacency["Review"]
        assert "__end__" in topo.adjacency["Review"]


# -- Classification --------------------------------------------------------


class TestClassifyForCompilation:
    def test_linear_all_dag_nodes(self):
        topo = extract_topology(linear_graph)
        dag_nodes, bundled = classify_for_compilation(topo)
        assert set(dag_nodes) == {"FetchData", "ProcessData"}
        assert bundled == []

    def test_branching_all_dag_nodes(self):
        topo = extract_topology(branching_graph)
        dag_nodes, bundled = classify_for_compilation(topo)
        assert set(dag_nodes) == {"StepA", "StepB", "StepC"}
        assert bundled == []

    def test_cyclic_creates_bundle(self):
        topo = extract_topology(cyclic_graph)
        dag_nodes, bundled = classify_for_compilation(topo)
        assert dag_nodes == []
        assert len(bundled) == 1
        assert set(bundled[0]) == {"Research", "Review"}
