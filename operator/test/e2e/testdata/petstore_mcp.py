"""Minimal MCP server standing in for a real petstore tool in the e2e suite.

The agent's ``petstore-api`` AgentTool points at this service over streamable
HTTP (``:8080/mcp``). The "direct A2A workflow" spec is the only spec that
actually *invokes* the agent, so this server is what turns that spec into a
genuine end-to-end exercise: the agent connects, lists tools, calls
``list_pets``, and answers the prompt.

Stateless + JSON responses keep it maximally compatible with the pydantic-ai
streamable-HTTP client: every request is independent (no session bookkeeping)
and replies are plain JSON rather than SSE.
"""

from __future__ import annotations

from mcp.server.fastmcp import FastMCP

mcp = FastMCP(
    "petstore",
    host="0.0.0.0",  # noqa: S104 - test container; bind all interfaces in-cluster
    port=8080,
    stateless_http=True,
    json_response=True,
)


@mcp.tool()
def list_pets() -> list[dict[str, object]]:
    """List the pets available in the store, with their numeric IDs and names."""
    return [
        {"id": 1, "name": "Rex", "species": "dog"},
        {"id": 2, "name": "Whiskers", "species": "cat"},
        {"id": 3, "name": "Bubbles", "species": "fish"},
    ]


if __name__ == "__main__":
    mcp.run(transport="streamable-http")
