# E2E test image for flokoa-managed-agent (Alpine-based)

FROM ghcr.io/astral-sh/uv:0.8.17 AS uv

FROM python:3.13-alpine AS builder

COPY --from=uv /uv /bin/uv

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never

WORKDIR /app

COPY sdk/python/pyproject.toml sdk/python/uv.lock ./
COPY sdk/python/flokoa/ ./flokoa/
COPY sdk/python/flokoa-managed-agent/ ./flokoa-managed-agent/

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --project flokoa-managed-agent --no-dev --no-editable

FROM python:3.13-alpine

RUN apk add --no-cache libstdc++ libgcc

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH="/app/.venv/bin:$PATH"

RUN addgroup -g 65532 -S nonroot && \
    adduser -u 65532 -G nonroot -S -H -s /sbin/nologin nonroot && \
    mkdir -p /etc/flokoa/tools && chown -R nonroot:nonroot /etc/flokoa

COPY --from=builder --chown=65532:65532 /app/.venv /app/.venv

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["python", "-m", "flokoa_managed_agent"]
