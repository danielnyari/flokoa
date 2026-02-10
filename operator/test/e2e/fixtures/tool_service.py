"""
Simple tool service for e2e testing.
"""
from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI()


class ToolRequest(BaseModel):
    query: str


class ToolResponse(BaseModel):
    result: str


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.post("/execute")
async def execute(request: ToolRequest):
    """Return a deterministic tool response."""
    return {"result": f"Tool executed with query: {request.query}"}


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8080)
