"""Tests for the pydantic-graph → AgentWorkflow compiler."""

from __future__ import annotations

from dataclasses import dataclass

from pydantic_graph import BaseNode, End, Graph, GraphRunContext

from flokoa.workflow import compile_graph

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
class Research(BaseNode[None]):
    async def run(self, ctx: GraphRunContext) -> Review:
        return Review()


@dataclass
class Review(BaseNode[None, None, str]):
    async def run(self, ctx: GraphRunContext) -> Research | End[str]:
        return End("approved")


cyclic_graph = Graph(nodes=[Research, Review])


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


# -- Compiler tests --------------------------------------------------------


class TestCompileLinearGraph:
    def test_manifest_structure(self):
        m = compile_graph(linear_graph, name="linear-test", image="test:1.0")
        assert m.api_version == "agent.flokoa.ai/v1alpha1"
        assert m.kind == "AgentWorkflow"
        assert m.metadata["name"] == "linear-test"
        assert m.metadata["namespace"] == "default"

    def test_steps_count(self):
        m = compile_graph(linear_graph, name="t", image="img:1")
        assert len(m.spec.steps) == 2

    def test_entrypoint_defaults_to_first_node(self):
        m = compile_graph(linear_graph, name="t", image="img:1")
        assert m.spec.entrypoint == "FetchData"

    def test_explicit_entrypoint(self):
        m = compile_graph(linear_graph, name="t", image="img:1", entrypoint="ProcessData")
        assert m.spec.entrypoint == "ProcessData"

    def test_step_next_edges(self):
        m = compile_graph(linear_graph, name="t", image="img:1")
        steps = {s.name: s for s in m.spec.steps}
        assert "ProcessData" in steps["FetchData"].next
        assert steps["ProcessData"].next == []

    def test_step_end_flag(self):
        m = compile_graph(linear_graph, name="t", image="img:1")
        steps = {s.name: s for s in m.spec.steps}
        assert steps["FetchData"].end is False
        assert steps["ProcessData"].end is True

    def test_node_class_is_set(self):
        m = compile_graph(linear_graph, name="t", image="img:1")
        for step in m.spec.steps:
            assert step.node_class is not None
            assert step.bundle is None

    def test_custom_namespace_and_labels(self):
        m = compile_graph(
            linear_graph,
            name="t",
            image="img:1",
            namespace="prod",
            labels={"app": "test"},
        )
        assert m.metadata["namespace"] == "prod"
        assert m.metadata["labels"] == {"app": "test"}


class TestCompileBranchingGraph:
    def test_branching_edges(self):
        m = compile_graph(branching_graph, name="t", image="img:1")
        steps = {s.name: s for s in m.spec.steps}
        assert set(steps["StepA"].next) == {"StepB", "StepC"}

    def test_branch_targets_are_end_nodes(self):
        m = compile_graph(branching_graph, name="t", image="img:1")
        steps = {s.name: s for s in m.spec.steps}
        assert steps["StepB"].end is True
        assert steps["StepC"].end is True


class TestCompileCyclicGraph:
    def test_creates_bundle_step(self):
        m = compile_graph(cyclic_graph, name="t", image="img:1")
        bundles = [s for s in m.spec.steps if s.bundle is not None]
        assert len(bundles) == 1

    def test_bundle_contains_both_nodes(self):
        m = compile_graph(cyclic_graph, name="t", image="img:1")
        bundle_step = next(s for s in m.spec.steps if s.bundle)
        assert set(bundle_step.bundle.node_classes) >= set()
        assert len(bundle_step.bundle.node_classes) == 2

    def test_bundle_has_entrypoint(self):
        m = compile_graph(cyclic_graph, name="t", image="img:1")
        bundle_step = next(s for s in m.spec.steps if s.bundle)
        assert bundle_step.bundle.entrypoint in {"Research", "Review"}

    def test_bundle_can_end(self):
        m = compile_graph(cyclic_graph, name="t", image="img:1")
        bundle_step = next(s for s in m.spec.steps if s.bundle)
        assert bundle_step.end is True
