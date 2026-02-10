FROM python:3.13-slim

WORKDIR /app

# Install uv for fast package management
RUN pip install uv

# Copy SDK source code
COPY sdk/python /sdk

# Install flokoa SDK from local source with pydantic-ai extra
RUN uv pip install --system /sdk[pydantic-ai]

# Copy agent code
COPY operator/test/e2e/fixtures/template_agent.py /app/

# Expose port for the agent server
EXPOSE 8080

# Run the agent using flokoa CLI
CMD ["flokoa", "template_agent:test_agent", "--host", "0.0.0.0", "--port", "8080"]
