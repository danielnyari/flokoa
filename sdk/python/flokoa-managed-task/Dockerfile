# ============================================================================
# flokoa-managed-task — production container image
#
# Base: python:3.13-alpine (~17 MB compressed vs ~50 MB for slim)
#
# Build:
#   - 3-stage: uv binary → alpine builder → alpine runtime
#   - UV_COMPILE_BYTECODE=1: pre-compile .pyc for faster cold-start
#   - UV_LINK_MODE=copy: hard-copy files (no dangling symlinks across stages)
#   - Layer caching: deps layer cached separately from source layer
#   - Cache mount on /root/.cache/uv persists across builds
#   - --frozen --no-dev --no-editable: exact lockfile, prod only, no src refs
#
# Runtime:
#   - Alpine base + libstdc++ / libgcc (needed by native wheels)
#   - Only .venv copied (no uv, no build tools, no source)
#   - Bytecode pre-compiled; PYTHONDONTWRITEBYTECODE prevents re-generation
#   - PYTHONUNBUFFERED for container log drivers
#   - Rootless: uid 65532, nologin shell
# ============================================================================

# --- Stage 0: grab uv binary ---
FROM ghcr.io/astral-sh/uv:0.8.17 AS uv

# --- Stage 1: build ---
FROM python:3.13-alpine AS builder

COPY --from=uv /uv /bin/uv

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never

WORKDIR /app

# Layer 1 — dependency install (cached unless config files change)
COPY pyproject.toml uv.lock ./
COPY flokoa-managed-task/pyproject.toml ./flokoa-managed-task/pyproject.toml

# Stub package so hatchling can discover it for resolution
RUN mkdir -p flokoa-managed-task/src/flokoa_managed_task && \
    touch flokoa-managed-task/src/flokoa_managed_task/__init__.py

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --project flokoa-managed-task \
            --no-dev --no-install-project --no-install-workspace

# Layer 2 — install project source (only re-runs when src/ changes)
COPY flokoa-managed-task/src ./flokoa-managed-task/src

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --project flokoa-managed-task \
            --no-dev --no-editable

# --- Stage 2: runtime ---
FROM python:3.13-alpine

# libstdc++ / libgcc: required at runtime by native extension wheels
RUN apk add --no-cache libstdc++ libgcc

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH="/app/.venv/bin:$PATH" \
    FLOKOA_CONFIG_PATH=/etc/flokoa/config.yaml

RUN addgroup -g 65532 -S nonroot && \
    adduser -u 65532 -G nonroot -S -H -s /sbin/nologin nonroot && \
    mkdir -p /etc/flokoa && chown -R nonroot:nonroot /etc/flokoa

COPY --from=builder --chown=65532:65532 /app/.venv /app/.venv

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["python", "-m", "flokoa_managed_task"]
