# E2E test image for flokoa-managed-agent
# Simplified build (no layer caching needed for tests)

FROM ghcr.io/astral-sh/uv:0.8.17 AS uv

FROM python:3.13-slim

COPY --from=uv /uv /bin/uv

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

COPY sdk/python/pyproject.toml sdk/python/uv.lock ./
COPY sdk/python/flokoa/ ./flokoa/
COPY sdk/python/flokoa-managed-agent/ ./flokoa-managed-agent/

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --project flokoa-managed-agent --no-dev --no-editable

ENV PATH="/app/.venv/bin:$PATH"

RUN groupadd --gid 65532 nonroot && \
    useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin nonroot && \
    mkdir -p /etc/flokoa/tools && chown -R nonroot:nonroot /etc/flokoa

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["python", "-m", "flokoa_managed_agent"]
