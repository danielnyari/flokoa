# ============================================================================
# flokoa-managed-agent — production container image
#
# Base: python:3.13-slim (Debian — pre-built manylinux wheels available)
#
# Build:
#   - 3-stage: uv binary → slim builder → slim runtime
#   - UV_COMPILE_BYTECODE=1: pre-compile .pyc for faster cold-start
#   - UV_LINK_MODE=copy: hard-copy files (no dangling symlinks across stages)
#   - Layer caching: deps layer cached separately from source layer
#   - Cache mount on /root/.cache/uv persists across builds
#   - --frozen --no-dev --no-editable: exact lockfile, prod only, no src refs
#
# Runtime:
#   - Slim base (pre-built wheels work out of the box)
#   - Only .venv copied (no uv, no build tools, no source)
#   - Bytecode pre-compiled; PYTHONDONTWRITEBYTECODE prevents re-generation
#   - PYTHONUNBUFFERED for container log drivers
#   - Rootless: uid 65532, nologin shell
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
COPY flokoa-common/pyproject.toml ./flokoa-common/pyproject.toml
COPY flokoa-managed-agent/pyproject.toml ./flokoa-managed-agent/pyproject.toml

# Stub packages so hatchling can discover them for workspace resolution
RUN mkdir -p flokoa/src/flokoa flokoa-types/src/flokoa_types \
             flokoa-common/src/flokoa_common \
             flokoa-managed-agent/src/flokoa_managed_agent && \
    touch flokoa/src/flokoa/__init__.py flokoa-types/src/flokoa_types/__init__.py \
          flokoa-common/src/flokoa_common/__init__.py \
          flokoa-managed-agent/src/flokoa_managed_agent/__init__.py

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --project flokoa-managed-agent \
            --no-dev --no-install-project --no-install-workspace

# Layer 2 — install workspace source (only re-runs when src/ changes)
COPY flokoa/README.md ./flokoa/README.md
COPY flokoa/src ./flokoa/src
COPY flokoa-types/README.md ./flokoa-types/README.md
COPY flokoa-types/src ./flokoa-types/src
COPY flokoa-common/README.md ./flokoa-common/README.md
COPY flokoa-common/src ./flokoa-common/src
COPY flokoa-managed-agent/README.md ./flokoa-managed-agent/README.md
COPY flokoa-managed-agent/src ./flokoa-managed-agent/src

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --project flokoa-managed-agent \
            --no-dev --no-editable

# --- Stage 2: runtime ---
FROM python:3.13-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH="/app/.venv/bin:$PATH" \
    FLOKOA_CONFIG_PATH=/etc/flokoa/config.yaml

RUN groupadd -g 65532 nonroot && \
    useradd -u 65532 -g nonroot -M -s /usr/sbin/nologin nonroot && \
    mkdir -p /etc/flokoa/tools && chown -R nonroot:nonroot /etc/flokoa

COPY --from=builder --chown=65532:65532 /app/.venv /app/.venv

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["python", "-m", "flokoa_managed_agent"]
