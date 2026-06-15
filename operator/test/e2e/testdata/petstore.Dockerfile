# Minimal MCP server used as the e2e agent's tool (service name: tool-service).
# Replaces the old Swagger petstore REST image, which did not speak MCP and
# therefore hung the agent the moment a spec actually invoked it. Serves a
# streamable-HTTP MCP endpoint at :8080/mcp exposing a single list_pets tool.
FROM python:3.13-slim

# Install the MCP SDK as root, then drop to the non-root UID the deployment's
# securityContext runs as (1000). Site-packages stay world-readable.
RUN pip install --no-cache-dir "mcp>=1.12,<2"

WORKDIR /app
COPY petstore_mcp.py /app/petstore_mcp.py

USER 1000

EXPOSE 8080
CMD ["python", "/app/petstore_mcp.py"]
