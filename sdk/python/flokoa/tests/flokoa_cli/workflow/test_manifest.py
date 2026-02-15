"""Tests for AgentWorkflow manifest serialization."""

from __future__ import annotations

import json
import tempfile
from pathlib import Path

import yaml

from flokoa.workflow._manifest import (
    AgentWorkflowManifest,
    AgentWorkflowSpec,
    WorkflowBundle,
    WorkflowStep,
)


def _simple_manifest() -> AgentWorkflowManifest:
    return AgentWorkflowManifest(
        metadata={"name": "test-wf", "namespace": "default"},
        spec=AgentWorkflowSpec(
            entrypoint="FetchData",
            image="test:1.0",
            steps=[
                WorkflowStep(
                    name="FetchData",
                    node_class="my_mod.FetchData",
                    next=["ProcessData"],
                ),
                WorkflowStep(
                    name="ProcessData",
                    node_class="my_mod.ProcessData",
                    end=True,
                ),
            ],
        ),
    )


def _bundle_manifest() -> AgentWorkflowManifest:
    return AgentWorkflowManifest(
        metadata={"name": "cyclic-wf", "namespace": "default"},
        spec=AgentWorkflowSpec(
            entrypoint="Research-Review",
            image="test:1.0",
            steps=[
                WorkflowStep(
                    name="Research-Review",
                    bundle=WorkflowBundle(
                        node_classes=["mod.Research", "mod.Review"],
                        entrypoint="Research",
                    ),
                    end=True,
                ),
            ],
        ),
    )


class TestToDict:
    def test_uses_camel_case_keys(self):
        d = _simple_manifest().to_dict()
        assert d["apiVersion"] == "agent.flokoa.ai/v1alpha1"
        assert "nodeClass" in json.dumps(d)

    def test_excludes_none(self):
        d = _simple_manifest().to_dict()
        step = d["spec"]["steps"][0]
        assert "bundle" not in step

    def test_bundle_step_serializes(self):
        d = _bundle_manifest().to_dict()
        step = d["spec"]["steps"][0]
        assert "nodeClasses" in step["bundle"]
        assert step["bundle"]["entrypoint"] == "Research"


class TestToYaml:
    def test_produces_valid_yaml(self):
        text = _simple_manifest().to_yaml()
        parsed = yaml.safe_load(text)
        assert parsed["kind"] == "AgentWorkflow"
        assert parsed["spec"]["entrypoint"] == "FetchData"

    def test_round_trip(self):
        m = _simple_manifest()
        parsed = yaml.safe_load(m.to_yaml())
        assert parsed == m.to_dict()


class TestToJson:
    def test_produces_valid_json(self):
        text = _simple_manifest().to_json()
        parsed = json.loads(text)
        assert parsed["kind"] == "AgentWorkflow"


class TestToFile:
    def test_writes_yaml_file(self):
        m = _simple_manifest()
        with tempfile.TemporaryDirectory() as tmp:
            path = m.to_file(Path(tmp) / "out.yaml")
            assert path.exists()
            parsed = yaml.safe_load(path.read_text())
            assert parsed["metadata"]["name"] == "test-wf"
