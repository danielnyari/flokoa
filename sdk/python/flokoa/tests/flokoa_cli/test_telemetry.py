"""Tests for flokoa.telemetry: OTel initialization and trace-context restoration.

The test environment installs the ``tracing`` extra, so ``init_telemetry`` must
get past the imports inside its try/except block. A broken import there is
swallowed by the ImportError handler and silently disables tracing — the exact
regression these tests guard against.
"""

import logging

from opentelemetry import context as otel_context
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider

from flokoa.telemetry import init_telemetry

TRACE_ID = "0af7651916cd43dd8448eb211c80319c"
TRACEPARENT = f"00-{TRACE_ID}-b7ad6b7169203331-01"


def test_init_telemetry_enables_tracing_when_otel_installed(caplog, monkeypatch):
    monkeypatch.delenv("OTEL_EXPORTER_OTLP_ENDPOINT", raising=False)

    with caplog.at_level(logging.DEBUG, logger="flokoa.telemetry"):
        init_telemetry("test-service")

    assert "tracing disabled" not in caplog.text
    assert isinstance(trace.get_tracer_provider(), TracerProvider)


def test_init_telemetry_restores_traceparent_from_env(monkeypatch):
    monkeypatch.delenv("OTEL_EXPORTER_OTLP_ENDPOINT", raising=False)
    monkeypatch.setenv("FLOKOA_TRACEPARENT", TRACEPARENT)

    # init_telemetry attaches the restored context without returning a detach
    # token, so bracket it with our own attach/detach to keep the test isolated.
    token = otel_context.attach(otel_context.get_current())
    try:
        init_telemetry("test-service", restore_context_from_env=True)

        span_context = trace.get_current_span().get_span_context()
        assert format(span_context.trace_id, "032x") == TRACE_ID
        assert span_context.is_remote
    finally:
        otel_context.detach(token)
