FROM python:3.13-slim

WORKDIR /app

# Install uv for fast package management
RUN pip install uv

# Copy SDK workspace source code
COPY sdk/python /sdk

# Install flokoa-managed-agent from local workspace source
RUN uv pip install --system /sdk/flokoa-managed-agent

# Expose port for the agent server
EXPOSE 8080

# Run managed agent - operator mounts instruction, model, tools config
CMD ["python", "-m", "flokoa_managed_agent"]
