# ============================================================================
# flokoa-runner — the generic runner image (runtime contract, contractVersion 1)
#
# Hydrates the operator-compiled AgentSpec, resolves secret placeholders,
# installs capability wheelhouses, constructs the agent via Agent.from_spec,
# and serves A2A. The runner manifest is baked at
# /etc/flokoa/runner-manifest.json; the image is labeled with the runner and
# contract versions (skew detection: see docs/reference/runtime-contract.md).
#
# Build (from sdk/python/):
#   docker build -f flokoa-runner/Dockerfile -t flokoa-runner:local .
# ============================================================================

# --- Stage 0: grab uv binary ---
FROM ghcr.io/astral-sh/uv:0.8.17 AS uv

# --- Stage 1: build ---
FROM python:3.13-slim AS builder

COPY --from=uv /uv /bin/uv

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never

WORKDIR /app

# Layer 1 — dependency install (cached unless config files change)
COPY pyproject.toml uv.lock ./
COPY flokoa/pyproject.toml ./flokoa/pyproject.toml
COPY flokoa-types/pyproject.toml ./flokoa-types/pyproject.toml
COPY flokoa-runner/pyproject.toml ./flokoa-runner/pyproject.toml

# Stub packages so hatchling can discover them for workspace resolution
RUN mkdir -p flokoa/src/flokoa flokoa-types/src/flokoa_types \
             flokoa-runner/src/flokoa_runner && \
    touch flokoa/src/flokoa/__init__.py flokoa-types/src/flokoa_types/__init__.py \
          flokoa-runner/src/flokoa_runner/__init__.py

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --project flokoa-runner \
            --no-dev --no-install-project --no-install-workspace

# Layer 2 — install workspace source (only re-runs when src/ changes)
COPY flokoa/README.md ./flokoa/README.md
COPY flokoa/src ./flokoa/src
COPY flokoa-types/README.md ./flokoa-types/README.md
COPY flokoa-types/src ./flokoa-types/src
COPY flokoa-runner/README.md ./flokoa-runner/README.md
COPY flokoa-runner/src ./flokoa-runner/src

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --project flokoa-runner \
            --no-dev --no-editable

# --- Stage 2: runtime ---
FROM python:3.13-slim

ARG RUNNER_VERSION=0.2.0
ARG CONTRACT_VERSION=1

LABEL ai.flokoa.runner-version="${RUNNER_VERSION}" \
      ai.flokoa.contract-version="${CONTRACT_VERSION}"

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH="/app/.venv/bin:$PATH"

RUN groupadd -g 65532 nonroot && \
    useradd -u 65532 -g nonroot -M -s /usr/sbin/nologin nonroot && \
    mkdir -p /etc/flokoa /opt/flokoa/capabilities && \
    chown -R nonroot:nonroot /etc/flokoa /opt/flokoa

COPY --from=builder --chown=65532:65532 /app/.venv /app/.venv

# The runner's identity (runtime contract §1).
COPY --chown=65532:65532 flokoa-runner/runner-manifest.json /etc/flokoa/runner-manifest.json

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["python", "-m", "flokoa_runner"]
