import logging
from typing import TYPE_CHECKING, override

from a2a.server.agent_execution import RequestContext
from a2a.server.events import EventQueue
from a2a.utils import new_agent_text_message

from flokoa.agent_executor import FlokoaAgentExecutor
from flokoa.cache import ConfigCache
from flokoa.exceptions import CancelNotSupportedError

from .toolset import FlokoaToolset

if TYPE_CHECKING:
    from google.adk.agents import LlmAgent

logger = logging.getLogger(__name__)


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

    def __init__(self, agent: "LlmAgent", cache: ConfigCache | None = None):
        """Initialize the executor.

        Args:
            agent: The Google ADK agent to wrap.
            cache: Optional cache instance. Uses global cache if not provided.
        """
        super().__init__(agent, cache)  # type: ignore[arg-type]

    @property
    def agent(self) -> "LlmAgent":  # type: ignore[override]
        return self._agent

    def _get_toolset(self) -> FlokoaToolset:
        """Get the Flokoa toolset for injection into ADK agent.

        Returns:
            FlokoaToolset with all configured tools.
        """
        return FlokoaToolset(
            tool_definitions=self.tool_definitions,
            get_tool_callable=self._get_tool_callable,
        )

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
            # Capture the final text response from content events
            if hasattr(event, "content") and event.content:
                parts = getattr(event.content, "parts", None)
                if parts:
                    for part in parts:
                        if hasattr(part, "text") and part.text:
                            final_response = part.text

        if final_response:
            await event_queue.enqueue_event(new_agent_text_message(final_response))

    @override
    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        raise CancelNotSupportedError("cancel not supported")
