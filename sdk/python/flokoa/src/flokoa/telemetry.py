"""OpenTelemetry initialization and W3C trace context propagation for Flokoa runtimes.

This module provides a single ``init_telemetry`` entry-point that:

1. Creates a ``TracerProvider`` with an OTLP gRPC exporter (when
   ``OTEL_EXPORTER_OTLP_ENDPOINT`` is set).
2. Optionally restores a parent trace context from the ``FLOKOA_TRACEPARENT``
   environment variable — used by one-shot container tasks whose traceparent
   is injected by the Flokoa operator via Argo workflow parameters.
3. Optionally instruments a FastAPI application so that incoming HTTP requests
   carrying a ``traceparent`` header are automatically linked to the
   distributed trace — used by long-running A2A agent servers.

If the OpenTelemetry packages are not installed the functions degrade gracefully
and tracing is silently disabled.
"""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from fastapi import FastAPI

logger = logging.getLogger(__name__)

_TRACEPARENT_ENV = "FLOKOA_TRACEPARENT"


def init_telemetry(
    service_name: str,
    *,
    restore_context_from_env: bool = False,
) -> None:
    """Initialize OpenTelemetry tracing for the current process.

    Parameters
    ----------
    service_name:
        The ``service.name`` resource attribute reported to the OTEL collector.
    restore_context_from_env:
        When ``True`` the W3C ``traceparent`` value in the ``FLOKOA_TRACEPARENT``
        environment variable is extracted and attached to the current context so
        that all subsequent spans become children of the controller's span.
        Use this for **one-shot container tasks**.
        Do **not** use this for long-running servers that receive per-request
        trace context via HTTP headers.
    """
    try:
        from opentelemetry import context as otel_context
        from opentelemetry import trace
        from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
        from opentelemetry.sdk.resources import SERVICE_NAME, Resource
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.export import BatchSpanProcessor
        from opentelemetry.trace.propagation import TraceContextTextMapPropagator
    except ImportError:
        logger.debug("OpenTelemetry packages not installed — tracing disabled")
        return

    resource = Resource.create({SERVICE_NAME: service_name})
    provider = TracerProvider(resource=resource)

    # Only add an exporter when a collector endpoint is configured.
    endpoint = os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT")
    if endpoint:
        provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
        logger.info("OpenTelemetry exporter configured: %s", endpoint)

    trace.set_tracer_provider(provider)

    # Restore parent trace context injected by the operator (container tasks).
    if restore_context_from_env:
        traceparent = os.environ.get(_TRACEPARENT_ENV)
        if traceparent:
            propagator = TraceContextTextMapPropagator()
            ctx = propagator.extract({"traceparent": traceparent})
            otel_context.attach(ctx)
            logger.info("Restored trace context from %s", _TRACEPARENT_ENV)


def instrument_pydantic_ai() -> None:
    """Enable OpenTelemetry instrumentation for all PydanticAI agents.

    Calls ``Agent.instrument_all()`` so that every agent run, model request,
    and tool call emits OTEL spans following the GenAI semantic conventions.

    If PydanticAI is not installed this is a no-op.
    """
    try:
        from pydantic_ai import Agent
    except ImportError:
        logger.debug("pydantic-ai not installed — skipping agent instrumentation")
        return

    Agent.instrument_all()
    logger.info("PydanticAI OpenTelemetry instrumentation enabled")


def instrument_fastapi(app: FastAPI) -> None:
    """Instrument a FastAPI application with OpenTelemetry.

    Incoming requests with a ``traceparent`` header will automatically be
    linked to the distributed trace.  If the instrumentation package is not
    installed this is a no-op.
    """
    try:
        from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
    except ImportError:
        logger.debug("opentelemetry-instrumentation-fastapi not installed — skipping")
        return

    FastAPIInstrumentor.instrument_app(app)
    logger.info("FastAPI OpenTelemetry instrumentation enabled")
