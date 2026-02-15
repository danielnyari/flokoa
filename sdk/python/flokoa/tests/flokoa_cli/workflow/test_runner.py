"""Tests for the workflow node runner."""

from __future__ import annotations

import json
import tempfile
from pathlib import Path
from unittest.mock import patch

from flokoa.workflow._runner import _import_object, _write_outputs, main


class TestImportObject:
    def test_imports_builtin(self):
        obj = _import_object("json.loads")
        assert obj is json.loads

    def test_imports_nested(self):
        obj = _import_object("os.path.join")
        import os.path

        assert obj is os.path.join


class TestWriteOutputs:
    def test_writes_next_and_state(self):
        with tempfile.TemporaryDirectory() as tmp:
            out_dir = Path(tmp)
            with patch("flokoa.workflow._runner.OUTPUT_DIR", out_dir):
                _write_outputs(next_step="ProcessData", state='{"x": 1}')
                assert (out_dir / "next").read_text() == "ProcessData"
                assert json.loads((out_dir / "state").read_text()) == {"x": 1}
                assert not (out_dir / "result").exists()

    def test_writes_result_when_provided(self):
        with tempfile.TemporaryDirectory() as tmp:
            out_dir = Path(tmp)
            with patch("flokoa.workflow._runner.OUTPUT_DIR", out_dir):
                _write_outputs(next_step="__end__", state="{}", result='"done"')
                assert (out_dir / "result").read_text() == '"done"'


class TestMainCLI:
    def test_node_mode_invokes_run_node(self):
        with patch("flokoa.workflow._runner.run_node") as mock:
            main(["--node", "my_mod.MyNode", "--state", '{"a": 1}'])
            mock.assert_called_once_with(
                node_class_path="my_mod.MyNode",
                state_json='{"a": 1}',
            )

    def test_bundle_mode_invokes_run_bundle(self):
        with patch("flokoa.workflow._runner.run_bundle") as mock:
            main([
                "--graph", "my_mod.graph",
                "--entry-node", "my_mod.Review",
                "--state", "{}",
            ])
            mock.assert_called_once_with(
                graph_path="my_mod.graph",
                entry_node_path="my_mod.Review",
                state_json="{}",
            )
