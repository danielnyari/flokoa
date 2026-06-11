"""flokoa.platform/telemetry — the first platform-injected capability.

Wraps pydantic-ai's core ``Instrumentation`` capability (agent/model/tool
spans with GenAI semantic conventions, including token attributes) and adds
flokoa's own signal on top: a ``flokoa.agent.invoke`` span carrying the agent
and session identity, plus RED metrics and token-usage histograms.

The operator appends this entry to every compiled spec (after all user
entries); the implementation ships in the runner baseline and versions with
the runner. Opt-out is cluster policy only — never per-Agent.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

from opentelemetry import metrics, trace
from opentelemetry.trace import StatusCode
from pydantic_ai.capabilities import Instrumentation, WrapperCapability

from flokoa import context as flokoa_context

if TYPE_CHECKING:
    from pydantic_ai._run_context import RunContext
    from pydantic_ai.capabilities.abstract import WrapRunHandler
    from pydantic_ai.run import AgentRunResult

_tracer = trace.get_tracer("flokoa.runner")
_meter = metrics.get_meter("flokoa.runner")

_requests = _meter.create_counter(
    "flokoa.agent.requests",
    description="Agent run outcomes",
)
_duration = _meter.create_histogram(
    "flokoa.agent.request.duration",
    unit="s",
    description="Agent run duration",
)
_tokens = _meter.create_histogram(
    "gen_ai.client.token.usage",
    unit="{token}",
    description="Token usage per agent run",
)


@dataclass(init=False)
class FlokoaTelemetry(WrapperCapability[Any]):
    """Traces + GenAI token metrics for every agent run, zero user config."""

    def __init__(self, wrapped: Any = None, **fields: Any) -> None:
        # `wrapped` and `**fields` exist for dataclasses.replace() (the
        # per-run clone in WrapperCapability.for_run), which passes every
        # inherited dataclass field; spec entries never pass arguments.
        self.wrapped = wrapped if wrapped is not None else Instrumentation()
        for key, value in fields.items():
            setattr(self, key, value)
        self.__post_init__()

    @classmethod
    def from_spec(cls) -> FlokoaTelemetry:
        """Spec entries take no configuration (contract §6: config comes from
        the operator; identity flows via OTEL_* env)."""
        return cls()

    @classmethod
    def get_serialization_name(cls) -> str:
        return "flokoa.platform/telemetry"

    async def wrap_run(
        self,
        ctx: RunContext[Any],
        *,
        handler: WrapRunHandler,
    ) -> AgentRunResult[Any]:
        import time

        attributes = _identity_attributes()
        start = time.perf_counter()
        with _tracer.start_as_current_span("flokoa.agent.invoke", attributes=attributes) as span:
            try:
                # Delegate inward so the core Instrumentation span nests
                # under flokoa.agent.invoke.
                result = await super().wrap_run(ctx, handler=handler)
            except Exception as exc:
                span.set_status(StatusCode.ERROR, str(exc))
                _requests.add(1, {**attributes, "outcome": "error", "error.type": type(exc).__name__})
                _duration.record(time.perf_counter() - start, {**attributes, "outcome": "error"})
                raise

        _requests.add(1, {**attributes, "outcome": "success"})
        _duration.record(time.perf_counter() - start, {**attributes, "outcome": "success"})
        _record_token_usage(result, ctx, attributes)
        return result


def _identity_attributes() -> dict[str, str]:
    attributes: dict[str, str] = {}
    if name := flokoa_context.agent_name():
        attributes["flokoa.agent.name"] = name
    if namespace := flokoa_context.agent_namespace():
        attributes["k8s.namespace.name"] = namespace
    if context_id := flokoa_context.context_id():
        attributes["flokoa.context.id"] = context_id
    if task_id := flokoa_context.task_id():
        attributes["flokoa.task.id"] = task_id
    return attributes


def _record_token_usage(result: AgentRunResult[Any], ctx: RunContext[Any], attributes: dict[str, str]) -> None:
    try:
        usage = result.usage()
    except Exception:
        return
    model_name = getattr(getattr(ctx, "model", None), "model_name", None)
    base = dict(attributes)
    if model_name:
        base["gen_ai.request.model"] = model_name
    if input_tokens := getattr(usage, "input_tokens", None):
        _tokens.record(input_tokens, {**base, "gen_ai.token.type": "input"})
    if output_tokens := getattr(usage, "output_tokens", None):
        _tokens.record(output_tokens, {**base, "gen_ai.token.type": "output"})
