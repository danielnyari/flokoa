from flokoa.agent_executor import FlokoaAgentExecutor
from flokoa_types import IntegrationType

_EXTRA_NAMES: dict[IntegrationType, str] = {
    IntegrationType.PYDANTIC_AI: "pydantic-ai",
    IntegrationType.GOOGLE_ADK: "google-adk",
}

_loaded: dict[IntegrationType, type[FlokoaAgentExecutor]] = {}


def _try_load(name: IntegrationType, import_path: str, class_name: str) -> None:
    """Attempt to load an integration, silently skip if dependencies missing."""
    try:
        import importlib

        module = importlib.import_module(import_path)
        _loaded[name] = getattr(module, class_name)
    except ImportError:
        pass


def get_executor_cls(name: IntegrationType) -> type[FlokoaAgentExecutor]:
    """
    Get an integration's AgentExecutor class.

    Raises:
        ImportError: If the integration's dependencies are not installed.
    """
    if name in _loaded:
        return _loaded[name]

    extra = _EXTRA_NAMES.get(name, name.value)
    raise ImportError(f"flokoa[{extra}] is not installed. Install it with: pip install flokoa[{extra}]")


# Load available integrations
_try_load(
    IntegrationType.PYDANTIC_AI,
    "flokoa.integrations.pydantic_ai.agent_executor",
    "PydanticAIAgentExecutor",
)
_try_load(
    IntegrationType.GOOGLE_ADK,
    "flokoa.integrations.google_adk.agent_executor",
    "GoogleADKAgentExecutor",
)
