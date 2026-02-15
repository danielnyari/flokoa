FROM python:3.13-slim AS builder

COPY --from=ghcr.io/astral-sh/uv:0.10.2 /uv /uvx /bin/

WORKDIR /app

# Copy workspace root + both packages needed
COPY pyproject.toml uv.lock ./
COPY flokoa/ ./flokoa/
COPY flokoa-managed-agent/ ./flokoa-managed-agent/

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --project flokoa-managed-agent --no-dev

# --- Runtime stage ---
FROM python:3.13-slim

RUN groupadd --gid 65532 nonroot && \
    useradd --uid 65532 --gid 65532 --no-create-home nonroot

COPY --from=builder /app/.venv /app/.venv

# Create mount points for operator ConfigMaps
RUN mkdir -p /etc/flokoa/tools && chown -R nonroot:nonroot /etc/flokoa

ENV PATH="/app/.venv/bin:$PATH"
ENV FLOKOA_CONFIG_PATH=/etc/flokoa/config.yaml

USER 65532:65532

EXPOSE 8080

CMD ["python", "-m", "flokoa_managed_agent"]
