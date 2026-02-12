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
from typing import Any, Dict, List, Literal, Optional, Union

import yaml
from pydantic_ai import FunctionToolset, Tool

from ...auth.auth_credential import AuthCredential
from ...auth.auth_schemes import AuthScheme
from .openapi_spec_parser import OpenApiSpecParser
from .rest_api_tool import RestApiToolConfig, create_rest_api_tool

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
        spec_dict: Optional[Dict[str, Any]] = None,
        spec_str: Optional[str] = None,
        spec_str_type: Literal["json", "yaml"] = "json",
        auth_scheme: Optional[AuthScheme] = None,
        auth_credential: Optional[AuthCredential] = None,
        tool_filter: Optional[List[str]] = None,
        tool_name_prefix: Optional[str] = None,
        ssl_verify: Optional[Union[bool, str, ssl.SSLContext]] = None,
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

        self._configs: List[RestApiToolConfig] = self._parse(
            spec_dict, auth_scheme, auth_credential, ssl_verify
        )

    def get_tools(self) -> List[Tool]:
        """Get all Pydantic AI Tool objects, respecting the tool filter."""
        configs = self._filtered_configs()
        return [create_rest_api_tool(c) for c in configs]

    def get_tool(self, tool_name: str) -> Optional[Tool]:
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

    def _filtered_configs(self) -> List[RestApiToolConfig]:
        if self._tool_filter is None:
            return self._configs
        return [c for c in self._configs if c.name in self._tool_filter]

    @staticmethod
    def _load_spec(spec_str: str, spec_type: Literal["json", "yaml"]) -> Dict[str, Any]:
        if spec_type == "json":
            return json.loads(spec_str)
        elif spec_type == "yaml":
            return yaml.safe_load(spec_str)
        else:
            raise ValueError(f"Unsupported spec type: {spec_type}")

    @staticmethod
    def _parse(
        openapi_spec_dict: Dict[str, Any],
        auth_scheme: Optional[AuthScheme],
        auth_credential: Optional[AuthCredential],
        ssl_verify: Optional[Union[bool, str, ssl.SSLContext]],
    ) -> List[RestApiToolConfig]:
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
