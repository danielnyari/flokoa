"""Unit tests for flokoa_managed_agent.bootstrap."""

from pydantic_ai import Agent

from flokoa_managed_agent.bootstrap import TemplatedAgentBuilder


def test_from_config_returns_agent(template_config):
    """from_config() returns a pydantic-ai Agent."""
    agent = TemplatedAgentBuilder.from_config(config=template_config)
    assert isinstance(agent, Agent)


def test_builder_config_property(template_config):
    """Builder.config returns the stored TemplateConfig."""
    builder = TemplatedAgentBuilder(config=template_config)
    assert builder.config is template_config
    assert builder.config.output_schema.name == "TestOutput"


def test_builder_output_schema_carries_config_name(template_config):
    """output_schema embeds the name from config into the JSON schema title."""
    builder = TemplatedAgentBuilder(config=template_config)
    schema = builder.output_schema
    # StructuredDict stores name as 'title' in the underlying JSON schema closure
    json_schema_fn = schema.__dict__["__get_pydantic_json_schema__"].__func__
    captured_schema = json_schema_fn.__closure__[0].cell_contents
    assert captured_schema["title"] == "TestOutput"


def test_builder_output_schema_is_type(template_config):
    """output_schema returns a type (StructuredDict) usable as Agent output_type."""
    builder = TemplatedAgentBuilder(config=template_config)
    schema = builder.output_schema
    assert isinstance(schema, type)


def test_from_config_agent_has_output_type(template_config):
    """Agent built by from_config has a structured output type configured."""
    agent = TemplatedAgentBuilder.from_config(config=template_config)
    # The agent's output type should be set (not just `str`)
    assert agent._output_type is not None
