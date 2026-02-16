import logging
from typing import TYPE_CHECKING, Any, override

from a2a.server.agent_execution import RequestContext
from a2a.server.events import EventQueue
from a2a.utils import new_agent_text_message

from flokoa.agent_executor import FlokoaAgentExecutor
from flokoa.cache import ConfigCache
from flokoa.exceptions import CancelNotSupportedError
from flokoa.tools import ToolsetFactory, default_factory
from flokoa_types import IntegrationType, ToolType
from flokoa_types import ToolDefinition as FlokoaToolDefinition

from .toolset import FlokoaToolset

if TYPE_CHECKING:
    from google.adk.agents import LlmAgent

logger = logging.getLogger(__name__)


def _extract_final_response(event: Any) -> str | None:
    """Extract final text response from an ADK event.

    Inspects the event's content parts and returns the last text part found,
    or None if the event has no text content.

    Args:
        event: An ADK event object with optional ``content.parts`` attribute.

    Returns:
        The text from the last text part, or None.
    """
    if not (hasattr(event, "content") and event.content):
        return None
    parts = getattr(event.content, "parts", None)
    if not parts:
        return None
    text = None
    for part in parts:
        if hasattr(part, "text") and part.text:
            text = part.text
    return text


def _openapi_adk_builder(tool_definition: FlokoaToolDefinition) -> list[Any]:
    from google.adk.tools.openapi_tool.openapi_spec_parser.openapi_toolset import (
        OpenAPIToolset,
    )

    open_api = tool_definition.spec.open_api
    if open_api is None:
        raise ValueError(f"Tool '{tool_definition.name}' has type openapi but no openApi configuration")

    spec_dict = open_api.open_api_schema.value
    if spec_dict is None:
        raise ValueError(f"Tool '{tool_definition.name}' has no inline OpenAPI spec (openApiSchema.value)")

    # Override servers if CRD specifies a base URL
    if open_api.url:
        spec_dict = {**spec_dict, "servers": [{"url": open_api.url}]}

    toolset = OpenAPIToolset(spec_dict=spec_dict)
    # ADK OpenAPIToolset.get_tools() is sync and returns list[RestApiTool]
    return toolset.get_tools()


default_factory.register(ToolType.OPENAPI, IntegrationType.GOOGLE_ADK, _openapi_adk_builder)


class GoogleADKAgentExecutor(FlokoaAgentExecutor):
    """A2A AgentExecutor that wraps a Google ADK agent.

    This executor provides:
    - Integration with Google ADK's Runner for agent execution
    - Automatic session management with in-memory services
    - Cached loading of tool definitions with TTL (inherited)
    - Cached loading of model configuration with TTL (inherited)

    Environment Variables:
        FLOKOA_CACHE_TTL_SECONDS: TTL for cached configs in seconds (default: 60)
        FLOKOA_CACHE_ENABLED: Enable/disable caching (default: true)
    """

    _agent: "LlmAgent"

    def __init__(
        self,
        agent: "LlmAgent",
        cache: ConfigCache | None = None,
        toolset_factory: ToolsetFactory | None = None,
    ):
        """Initialize the executor.

        Args:
            agent: The Google ADK agent to wrap.
            cache: Optional cache instance. Uses global cache if not provided.
            toolset_factory: Optional factory for building toolsets. Uses the
                default factory if not provided.
        """
        super().__init__(agent, cache)  # type: ignore[arg-type]
        self._toolset_factory = toolset_factory or default_factory

    @property
    def agent(self) -> "LlmAgent":  # type: ignore[override]
        return self._agent

    def _get_toolset(self) -> FlokoaToolset:
        """Get the Flokoa toolset for injection into ADK agent.

        Returns:
            FlokoaToolset with all configured tools.
        """
        tools = self._toolset_factory.build(self.tool_definitions, IntegrationType.GOOGLE_ADK)
        return FlokoaToolset(tools)

    def _inject_tools(self) -> None:
        """Inject Flokoa tools into the agent's tools list."""
        if not self.tool_definitions:
            return

        toolset = self._get_toolset()

        # Append toolset to agent's tools list
        if self.agent.tools is None:
            self.agent.tools = []

        # Check if toolset is already injected (avoid duplicates)
        for existing_tool in self.agent.tools:
            if isinstance(existing_tool, FlokoaToolset):
                return

        self.agent.tools.append(toolset)  # type: ignore[arg-type]
        logger.info(f"Injected {len(self.tool_definitions)} Flokoa tools into ADK agent.")

    @override
    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        from google.adk.artifacts import InMemoryArtifactService
        from google.adk.memory import InMemoryMemoryService
        from google.adk.runners import Runner
        from google.adk.sessions import InMemorySessionService
        from google.genai import types

        request = context.get_user_input()

        # Inject Flokoa tools into the agent
        self._inject_tools()

        runner = Runner(
            app_name=self.agent.name or "flokoa_adk_agent",
            agent=self.agent,
            artifact_service=InMemoryArtifactService(),
            session_service=InMemorySessionService(),
            memory_service=InMemoryMemoryService(),
        )

        session = await runner.session_service.create_session(
            app_name=runner.app_name,
            user_id="flokoa_user",
        )

        user_content = types.Content(
            role="user",
            parts=[types.Part.from_text(text=request)],
        )

        # Run the agent and capture the final response
        final_response = None
        async for event in runner.run_async(
            user_id=session.user_id,
            session_id=session.id,
            new_message=user_content,
        ):
            text = _extract_final_response(event)
            if text is not None:
                final_response = text

        if final_response:
            await event_queue.enqueue_event(new_agent_text_message(final_response))

    @override
    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        raise CancelNotSupportedError("cancel not supported")
