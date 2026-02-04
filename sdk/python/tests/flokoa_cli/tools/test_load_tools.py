
import json

import flokoa.utils as utils_module
from flokoa.utils import load_tools


def test_load_tools_tools_config(tools_config, tmp_path, monkeypatch):
    tools_dir = tmp_path / "flokoa" / "tools"
    tools_dir.mkdir(parents=True)

    # Patch the path in the module so load_tools reads from tmp_path
    monkeypatch.setattr(utils_module, "TOOLS_PATH", str(tools_dir) + "/")

    for t in tools_config:
        with open(tools_dir / f"{t['name']}.json", "w") as f:
            json.dump(t, f)
    loaded_tools = load_tools()
    assert len(loaded_tools) == len(tools_config)
    for t in loaded_tools:
        matching_tool = next((tc for tc in tools_config if tc["name"] == t.name), None)
        assert matching_tool is not None
        assert t.spec.type.value == matching_tool["spec"]["type"]
        assert t.spec.description == matching_tool["spec"]["description"]
        assert t.spec.input_schema == matching_tool["spec"]["inputSchema"]
        assert t.spec.output_schema == matching_tool["spec"]["outputSchema"]
        assert t.spec.http_api is not None
        assert t.spec.http_api.method.value == matching_tool["spec"]["httpApi"]["method"]
        assert t.spec.http_api.url == matching_tool["spec"]["httpApi"]["url"]
