"""The flokoa generic runner entrypoint.

Bootstrap pipeline (runtime contract §2):

    load_manifest → load_compiled_spec → resolve_secrets →
    install_capabilities → build_agent → serve

Each stage fails fast with a single-line JSON error naming the stage.

Usage: python -m flokoa_runner
"""

from __future__ import annotations

import logging
import os

from flokoa_runner.agent import build_agent
from flokoa_runner.capabilities import install_capabilities
from flokoa_runner.errors import BootstrapError, fail
from flokoa_runner.manifest import load_manifest
from flokoa_runner.secrets import resolve_secrets
from flokoa_runner.serve import serve
from flokoa_runner.specfile import load_compiled_spec

logger = logging.getLogger(__name__)


def main() -> None:
    log_level = os.environ.get("LOG_LEVEL", "INFO").upper()
    logging.basicConfig(
        level=getattr(logging, log_level, logging.INFO),
        format="%(asctime)s %(name)s %(levelname)s %(message)s",
    )

    # Telemetry first so even bootstrap problems can surface as spans when an
    # exporter is configured; per-request tracing comes from the FastAPI
    # instrumentation and the platform telemetry capability.
    from flokoa.telemetry import init_telemetry

    init_telemetry(os.environ.get("OTEL_SERVICE_NAME", "flokoa-runner"), restore_context_from_env=False)

    try:
        manifest = load_manifest()
        logger.info(
            "flokoa-runner %s (contract v%d, pydantic-ai %s)",
            manifest.runner_version,
            manifest.contract_version,
            manifest.pydantic_ai,
        )

        doc = load_compiled_spec()
        doc = resolve_secrets(doc)
        capability_types = install_capabilities(manifest)
        agent = build_agent(doc, capability_types)
    except BootstrapError as err:
        fail(err)

    serve(agent)


if __name__ == "__main__":
    main()
