from typing import TYPE_CHECKING

from fastapi import FastAPI

if TYPE_CHECKING:
    from pydantic_ai import Agent as PydanticAgent

def wrap_agent(agent: "PydanticAgent") -> FastAPI:
    app = FastAPI()

    @app.get("/info")
    async def get_info():
        return {"name": agent.name, "description": agent.description}

    return app