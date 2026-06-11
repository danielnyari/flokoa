"""Tests for ToolsetFactory — builder registration, dispatch, and edge cases."""

from flokoa_types import ToolDefinition, ToolType

from flokoa.tools.toolset_factory import ToolsetFactory


def _make_tool_definition(name="test_tool", url="https://api.test.com"):
    """Create a minimal ToolDefinition for testing."""
    return ToolDefinition.model_validate({
        "name": name,
        "spec": {
            "type": "openapi",
            "description": f"Tool: {name}",
            "openApi": {
                "openApiSchema": {
                    "value": {
                        "openapi": "3.0.0",
                        "info": {"title": name, "version": "1.0"},
                        "paths": {
                            "/test": {
                                "get": {
                                    "operationId": f"{name}Op",
                                    "responses": {"200": {"description": "ok"}},
                                }
                            }
                        },
                    }
                },
                "url": url,
            },
        },
    })


class TestToolsetFactoryRegister:
    def test_register_openapi_builder(self):
        factory = ToolsetFactory()
        builder = lambda td: [f"tool:{td.name}"]
        factory.register(ToolType.OPENAPI, builder)

        tools = factory.build([_make_tool_definition()])
        assert tools == ["tool:test_tool"]

    def test_build_with_no_builder_registered_skips(self):
        """Definitions without a registered builder are logged and skipped."""
        factory = ToolsetFactory()
        tools = factory.build([_make_tool_definition()])
        assert tools == []


class TestToolsetFactoryBuild:
    def test_build_dispatches_to_registered_builder(self):
        factory = ToolsetFactory()
        builder_calls = []

        def tracking_builder(td):
            builder_calls.append(td.name)
            return [f"built:{td.name}"]

        factory.register(ToolType.OPENAPI, tracking_builder)

        td1 = _make_tool_definition("tool_a")
        td2 = _make_tool_definition("tool_b")
        tools = factory.build([td1, td2])

        assert builder_calls == ["tool_a", "tool_b"]
        assert tools == ["built:tool_a", "built:tool_b"]

    def test_build_empty_list(self):
        factory = ToolsetFactory()
        tools = factory.build([])
        assert tools == []

    def test_build_multiple_tools_per_definition(self):
        """A builder can return multiple tool objects per definition."""
        factory = ToolsetFactory()
        factory.register(ToolType.OPENAPI, lambda td: ["a", "b", "c"])

        tools = factory.build([_make_tool_definition()])
        assert tools == ["a", "b", "c"]
