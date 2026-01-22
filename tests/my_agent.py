from pydantic_ai import Agent

agent = Agent(  
    'google-vertex:gemini-2.5-pro',
    instructions='Be concise, reply with one sentence.',  
)

