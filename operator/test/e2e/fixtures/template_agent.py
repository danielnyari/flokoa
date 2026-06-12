"""
Simple test agent for e2e testing.
This agent demonstrates template-based instruction handling.
"""

from pydantic_ai import Agent

# Create a simple agent that will use templated instructions.
# The system_prompt here is a fallback/default. When deployed via the Operator,
# the Instruction CRD will provide templated instructions that override this.
test_agent = Agent(
    "openai:gpt-4o",
    system_prompt="You are a helpful assistant.",
)
