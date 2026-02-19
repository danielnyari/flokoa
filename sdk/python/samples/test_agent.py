"""Quick smoke test: send a message to a running managed-agent via A2A."""

import asyncio

from a2a.client import ClientConfig, ClientFactory, create_text_message_object


async def main() -> None:
    client = await ClientFactory.connect(
        agent="http://localhost:8080",
        client_config=ClientConfig(streaming=False),
    )

    message = create_text_message_object(content="What is 2+2? Answer in one word.")

    async for event in client.send_message(message):
        if isinstance(event, tuple):
            task, _update = event
            print(f"State: {task.status.state}")
            if task.artifacts:
                for artifact in task.artifacts:
                    print(f"Artifact: {artifact.model_dump_json(indent=2)}")
        else:
            print(f"Event: {event}")


if __name__ == "__main__":
    asyncio.run(main())
