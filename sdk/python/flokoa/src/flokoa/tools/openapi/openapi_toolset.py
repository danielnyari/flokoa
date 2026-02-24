# Copyright 2026 Flokoa Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from __future__ import annotations

import json
import logging
import ssl
from typing import TYPE_CHECKING, Any, Literal

import yaml
from flokoa_common.auth.auth_credential import AuthCredential
from flokoa_common.auth.auth_schemes import AuthScheme
from flokoa_common.utils.openapi.openapi_spec_parser import OpenApiSpecParser
from pydantic_ai import FunctionToolset, Tool

from .rest_api_tool import RestApiToolConfig, create_rest_api_tool

if TYPE_CHECKING:
    from ...types import ToolDefinition

logger = logging.getLogger("flokoa." + __name__)


class OpenAPIToolset:
    """Parses an OpenAPI spec into Pydantic AI Tools.

    Usage::

      # Initialize from a spec dict.
      toolset = OpenAPIToolset(spec_dict=openapi_spec_dict)

      # Or from a spec string.
      toolset = OpenAPIToolset(spec_str=openapi_spec_str, spec_str_type="json")

      # Use as a FunctionToolset with an agent.
      agent = Agent('openai:gpt-4o', deps_type=OpenAPIDeps)
      result = await agent.run(
          "List all users",
          deps=OpenAPIDeps(client=httpx.AsyncClient()),
          toolsets=[toolset.to_function_toolset()],
      )

      # Or get individual tools.
      tool = toolset.get_tool('list_users')
    """

    def __init__(
        self,
        *,
        spec_dict: dict[str, Any] | None = None,
        spec_str: str | None = None,
        spec_str_type: Literal["json", "yaml"] = "json",
        auth_scheme: AuthScheme | None = None,
        auth_credential: AuthCredential | None = None,
        tool_filter: list[str] | None = None,
        tool_name_prefix: str | None = None,
        ssl_verify: bool | str | ssl.SSLContext | None = None,
    ):
        """Initialize the OpenAPIToolset.

        Args:
            spec_dict: The OpenAPI spec as a dictionary. Takes precedence over
                spec_str if both are provided.
            spec_str: The OpenAPI spec as a JSON or YAML string.
            spec_str_type: Format of spec_str ("json" or "yaml").
            auth_scheme: Auth scheme applied to all tools.
            auth_credential: Auth credential applied to all tools.
            tool_filter: Optional list of tool names to include. If None, all
                tools from the spec are included.
            tool_name_prefix: Prefix to prepend to tool names. Applied via
                FunctionToolset.prefixed() in to_function_toolset().
            ssl_verify: SSL certificate verification option for all tools.
        """
        self._tool_name_prefix = tool_name_prefix
        self._tool_filter = tool_filter

        if not spec_dict:
            spec_dict = self._load_spec(spec_str, spec_str_type)

        self._configs: list[RestApiToolConfig] = self._parse(spec_dict, auth_scheme, auth_credential, ssl_verify)

    @classmethod
    def from_tool_definition(cls, tool_definition: ToolDefinition) -> OpenAPIToolset:
        """Create an OpenAPIToolset from a Flokoa ToolDefinition.

        Extracts the inline OpenAPI spec from the tool definition's
        ``openApi.openApiSchema.value`` field. If the tool definition
        specifies a base URL, it overrides the spec's ``servers`` entry.

        Args:
            tool_definition: A ToolDefinition with type "openapi".

        Returns:
            An OpenAPIToolset parsed from the spec.

        Raises:
            ValueError: If the tool definition lacks required openApi config.
        """
        open_api = tool_definition.spec.open_api
        if open_api is None:
            raise ValueError(f"Tool '{tool_definition.name}' has type openapi but no openApi configuration")

        spec_dict = open_api.open_api_schema.value
        if spec_dict is None:
            raise ValueError(f"Tool '{tool_definition.name}' has no inline OpenAPI spec (openApiSchema.value)")

        # Override servers if CRD specifies a base URL
        if open_api.url:
            spec_dict = {**spec_dict, "servers": [{"url": open_api.url}]}

        toolset = cls(spec_dict=spec_dict)

        # Apply default headers from CRD
        if open_api.headers:
            for config in toolset._configs:
                config.default_headers.update(open_api.headers)

        return toolset

    def get_tools(self) -> list[Tool]:
        """Get all Pydantic AI Tool objects, respecting the tool filter."""
        configs = self._filtered_configs()
        return [create_rest_api_tool(c) for c in configs]

    def get_tool(self, tool_name: str) -> Tool | None:
        """Get a single tool by name.

        Args:
            tool_name: The tool name to look up.

        Returns:
            The Tool if found, else None.
        """
        for config in self._configs:
            if config.name == tool_name:
                return create_rest_api_tool(config)
        return None

    def to_function_toolset(self) -> FunctionToolset:
        """Create a Pydantic AI FunctionToolset from all tools.

        If a tool_name_prefix was provided at construction time, the toolset
        is wrapped with prefixed().

        Returns:
            A FunctionToolset containing all (filtered) tools.
        """
        toolset = FunctionToolset()
        for tool in self.get_tools():
            toolset.add_tool(tool)

        if self._tool_name_prefix:
            return toolset.prefixed(self._tool_name_prefix)
        return toolset

    def _filtered_configs(self) -> list[RestApiToolConfig]:
        if self._tool_filter is None:
            return self._configs
        return [c for c in self._configs if c.name in self._tool_filter]

    @staticmethod
    def _load_spec(spec_str: str, spec_type: Literal["json", "yaml"]) -> dict[str, Any]:
        if spec_type == "json":
            return json.loads(spec_str)
        elif spec_type == "yaml":
            return yaml.safe_load(spec_str)
        else:
            raise ValueError(f"Unsupported spec type: {spec_type}")

    @staticmethod
    def _parse(
        openapi_spec_dict: dict[str, Any],
        auth_scheme: AuthScheme | None,
        auth_credential: AuthCredential | None,
        ssl_verify: bool | str | ssl.SSLContext | None,
    ) -> list[RestApiToolConfig]:
        operations = OpenApiSpecParser().parse(openapi_spec_dict)

        configs = []
        for o in operations:
            config = RestApiToolConfig.from_parsed_operation(o, ssl_verify=ssl_verify)

            # Apply global auth if the parsed operation didn't have its own
            if auth_scheme and not config.auth_scheme:
                config.auth_scheme = auth_scheme
            if auth_credential and not config.auth_credential:
                config.auth_credential = auth_credential

            logger.info("Parsed tool: %s", config.name)
            configs.append(config)
        return configs
