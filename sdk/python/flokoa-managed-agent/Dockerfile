# ============================================================================
# flokoa-managed-agent — production container image
#
# Optimizations:
#   - Multi-stage build: build deps in builder, copy only venv to runtime
#   - UV_COMPILE_BYTECODE=1: pre-compile .pyc for faster cold-start
#   - UV_LINK_MODE=copy: hard-copy instead of symlinks (container-safe)
#   - Layer caching: deps layer is cached unless pyproject.toml/uv.lock change
#   - Cache mount: uv download cache persists across builds
#   - --frozen --no-dev --no-editable: exact lockfile, no dev deps, no source refs
#   - Rootless: runs as uid 65532 (nonroot) with nologin shell
#   - PYTHONDONTWRITEBYTECODE: skip runtime .pyc generation (already compiled)
#   - PYTHONUNBUFFERED: direct stdout/stderr for container log drivers
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
COPY flokoa-managed-agent/pyproject.toml ./flokoa-managed-agent/pyproject.toml

# Stub packages so hatchling can discover them for workspace resolution
RUN mkdir -p flokoa/src/flokoa flokoa-managed-agent/src/flokoa_managed_agent && \
    touch flokoa/src/flokoa/__init__.py flokoa-managed-agent/src/flokoa_managed_agent/__init__.py

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --project flokoa-managed-agent \
            --no-dev --no-install-project --no-install-workspace

# Layer 2 — install workspace source (only re-runs when src/ changes)
COPY flokoa/src ./flokoa/src
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

RUN groupadd --gid 65532 nonroot && \
    useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin nonroot && \
    mkdir -p /etc/flokoa/tools && chown -R nonroot:nonroot /etc/flokoa

COPY --from=builder --chown=65532:65532 /app/.venv /app/.venv

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["python", "-m", "flokoa_managed_agent"]
