"""flokoa run -f: the local mirror of the cluster runner path."""

from pathlib import Path

import pytest
from click.testing import CliRunner

from flokoa.__main__ import _load_agent_from_spec, cli


def test_spec_file_hydrates(tmp_path: Path):
    spec = tmp_path / "agent.yaml"
    spec.write_text("model: test\ninstructions: Be terse.\n")
    agent = _load_agent_from_spec(spec)
    assert agent is not None


def test_run_requires_exactly_one_source(tmp_path: Path):
    runner = CliRunner()
    result = runner.invoke(cli, ["run"])
    assert result.exit_code != 0
    assert "exactly one" in result.output

    spec = tmp_path / "agent.yaml"
    spec.write_text("model: test\n")
    result = runner.invoke(cli, ["run", "-m", "x:y", "-f", str(spec)])
    assert result.exit_code != 0


@pytest.mark.parametrize("module", ["no-colon", ":attr", "mod:"])
def test_run_module_form_validated(module):
    runner = CliRunner()
    result = runner.invoke(cli, ["run", "-m", module])
    assert result.exit_code != 0
