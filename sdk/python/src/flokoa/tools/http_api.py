from typing import Any
import httpx

async def call_http_api_tool(endpoint: str, method: str, params: dict[str, Any]) -> dict[str, Any]:
    """Call an external API tool."""
    async with httpx.AsyncClient() as client:
        if method.upper() == "GET":
            response = await client.get(endpoint, params=params)
        elif method.upper() == "POST":
            response = await client.post(endpoint, json=params)
        else:
            raise ValueError(f"Unsupported HTTP method: {method}")

    response.raise_for_status()
    return response.json()