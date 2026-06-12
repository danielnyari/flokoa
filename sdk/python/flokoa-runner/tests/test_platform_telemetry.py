"""flokoa.platform/telemetry: traces + metrics with zero user configuration."""

import pytest
from flokoa import context as flokoa_context
from flokoa_runner.agent import build_agent
from opentelemetry import metrics, trace
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import InMemoryMetricReader
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

pytestmark = pytest.mark.anyio


@pytest.fixture
def anyio_backend():
    return "asyncio"


@pytest.fixture(scope="module")
def telemetry_capture():
    """Module-scoped OTel providers (the global providers are set-once)."""
    span_exporter = InMemorySpanExporter()
    trace.set_tracer_provider(TracerProvider())  # no-op if another suite already installed one
    # In a combined workspace-root run, flokoa's init_telemetry tests may have
    # installed the global provider first (set-once); attach the capture
    # processor to whichever SDK provider is actually effective.
    tracer_provider = trace.get_tracer_provider()
    assert isinstance(tracer_provider, TracerProvider), f"expected SDK TracerProvider, got {type(tracer_provider)}"
    tracer_provider.add_span_processor(SimpleSpanProcessor(span_exporter))

    metric_reader = InMemoryMetricReader()
    metrics.set_meter_provider(MeterProvider(metric_readers=[metric_reader]))

    # The capability module creates its tracer/meter at import; import after
    # the providers are installed.
    import flokoa_runner.platform_capabilities.telemetry  # noqa: F401

    return span_exporter, metric_reader


def metric_points(metric_reader, name):
    data = metric_reader.get_metrics_data()
    points = []
    for rm in data.resource_metrics:
        for scope in rm.scope_metrics:
            for metric in scope.metrics:
                if metric.name == name:
                    points.extend(metric.data.data_points)
    return points


async def test_invoke_span_and_metrics(telemetry_capture, monkeypatch):
    span_exporter, metric_reader = telemetry_capture
    span_exporter.clear()
    monkeypatch.setenv("OTEL_SERVICE_NAME", "support-agent")
    monkeypatch.setenv("OTEL_RESOURCE_ATTRIBUTES", "k8s.namespace.name=default,flokoa.agent.name=support-agent")
    flokoa_context.bind_request("ctx-123", "task-456")

    agent = build_agent({"model": "test", "capabilities": ["flokoa.platform/telemetry"]})
    result = await agent.run("hello")
    assert result.output

    spans = {s.name: s for s in span_exporter.get_finished_spans()}
    invoke = spans.get("flokoa.agent.invoke")
    assert invoke is not None, f"spans: {list(spans)}"
    assert invoke.attributes["flokoa.agent.name"] == "support-agent"
    assert invoke.attributes["flokoa.context.id"] == "ctx-123"
    assert invoke.attributes["flokoa.task.id"] == "task-456"

    requests = metric_points(metric_reader, "flokoa.agent.requests")
    assert any(p.attributes.get("outcome") == "success" for p in requests)

    durations = metric_points(metric_reader, "flokoa.agent.request.duration")
    assert durations

    tokens = metric_points(metric_reader, "gen_ai.client.token.usage")
    token_types = {p.attributes.get("gen_ai.token.type") for p in tokens}
    assert {"input", "output"} <= token_types, f"token points: {tokens}"


async def test_error_outcome_recorded(telemetry_capture, monkeypatch):
    span_exporter, metric_reader = telemetry_capture
    span_exporter.clear()

    from pydantic_ai import Agent
    from pydantic_ai.models.function import FunctionModel

    def explode(messages, info):
        raise RuntimeError("model exploded")

    from flokoa_runner.platform_capabilities.telemetry import FlokoaTelemetry

    agent = Agent(FunctionModel(explode), capabilities=[FlokoaTelemetry()])
    with pytest.raises(Exception, match="model exploded"):
        await agent.run("hello")

    requests = metric_points(metric_reader, "flokoa.agent.requests")
    errors = [p for p in requests if p.attributes.get("outcome") == "error"]
    assert errors
    assert errors[-1].attributes.get("error.type")
