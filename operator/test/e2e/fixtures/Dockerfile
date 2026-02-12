FROM python:3.13-slim

WORKDIR /app

# Install uv for fast package management
RUN pip install uv

# Copy SDK source code
COPY sdk/python /sdk

# Install flokoa SDK from local source with pydantic-ai extra
RUN uv pip install --system /sdk[pydantic-ai]

# Expose port for the agent server
EXPOSE 8080

# Run as a templated agent - operator mounts instruction, model, tools config
CMD ["flokoa", "run", "--templated", "--host", "0.0.0.0", "--port", "8080"]
