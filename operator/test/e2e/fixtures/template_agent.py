"""
Simple test agent for e2e testing.
This agent demonstrates template-based instruction handling.
"""
from pydantic_ai import Agent

# Create a simple agent that will use templated instructions
# The actual instructions will be provided via the Instruction CRD
test_agent = Agent(
    "openai:gpt-4o",
    system_prompt="You are a helpful assistant.",
)
