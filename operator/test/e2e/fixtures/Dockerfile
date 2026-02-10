FROM python:3.13-slim

WORKDIR /app

# Install uv for fast package management
RUN pip install uv

# Copy agent code
COPY template_agent.py /app/

# Install flokoa SDK and pydantic-ai
RUN uv pip install --system flokoa[pydantic-ai]

# Expose port for the agent server
EXPOSE 8080

# Run the agent using flokoa CLI
CMD ["flokoa", "template_agent:test_agent", "--host", "0.0.0.0", "--port", "8080"]
