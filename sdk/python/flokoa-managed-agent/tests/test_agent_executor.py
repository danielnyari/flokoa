"""Unit tests for flokoa_managed_agent.agent_executor."""

import pytest
from a2a.server.agent_execution import RequestContext
from a2a.server.events import EventQueue
from a2a.types import (
    Message,
    MessageSendParams,
    Part,
    Task,
    TaskArtifactUpdateEvent,
    TaskState,
    TaskStatusUpdateEvent,
    TextPart,
)
from flokoa.exceptions import ProviderNotConfiguredError
from pydantic_ai import Agent

pytestmark = pytest.mark.anyio


def _make_context(text: str, task: Task | None = None) -> RequestContext:
    """Build a RequestContext from plain text."""
    params = MessageSendParams(
        message=Message(
            message_id="test-msg",
            role="user",
            parts=[Part(root=TextPart(text=text))],
        ),
    )
    return RequestContext(request=params, task=task)


# ---------------------------------------------------------------------------
# Construction
# ---------------------------------------------------------------------------


def test_construction_creates_agent(executor):
    """Executor wraps a pydantic-ai Agent built from config."""
    assert isinstance(executor.agent, Agent)


def test_config_property(executor, template_config):
    """Executor.config returns the TemplateConfig passed at construction."""
    assert executor.config is template_config
    assert executor.config.output_schema.name == "TestOutput"


# ---------------------------------------------------------------------------
# Instruction property
# ---------------------------------------------------------------------------


def test_instruction_returns_text(executor):
    """instruction property returns text from the mounted file."""
    assert executor.instruction == "You are a helpful test agent. Answer questions concisely."


def test_instruction_raises_when_missing(executor_no_instruction):
    """instruction property raises ProviderNotConfiguredError when file is absent."""
    with pytest.raises(ProviderNotConfiguredError, match="Instruction is required"):
        _ = executor_no_instruction.instruction


# ---------------------------------------------------------------------------
# Model config
# ---------------------------------------------------------------------------


def test_model_config_loaded(executor):
    """Executor loads model config from the mounted file."""
    assert executor.model_config is not None
    assert executor.model_config.model == "gpt-4o-mini"


def test_model_config_provider_type(executor):
    """model_provider returns the correct ProviderType."""
    assert executor.model_provider is not None
    assert executor.model_provider.value == "openai"


# ---------------------------------------------------------------------------
# Execute
# ---------------------------------------------------------------------------


async def test_execute_creates_new_task(executor):
    """execute() creates a new task when context has no current task."""
    context = _make_context("Hello")
    event_queue = EventQueue()

    await executor.execute(context, event_queue)

    # First event should be the new Task
    event = await event_queue.dequeue_event(no_wait=True)
    assert isinstance(event, Task)


async def test_execute_sends_artifact_event(executor):
    """execute() enqueues a TaskArtifactUpdateEvent with output schema metadata."""
    context = _make_context("What is 2+2?")
    event_queue = EventQueue()

    await executor.execute(context, event_queue)

    events = []
    while not event_queue.queue.empty():
        events.append(await event_queue.dequeue_event(no_wait=True))

    artifact_events = [e for e in events if isinstance(e, TaskArtifactUpdateEvent)]
    assert len(artifact_events) == 1

    artifact_event = artifact_events[0]
    assert artifact_event.last_chunk is True
    assert artifact_event.append is False
    assert artifact_event.artifact.name == "TestOutput"
    assert artifact_event.artifact.description == "A test output schema"


async def test_execute_sends_completed_status(executor):
    """execute() enqueues a TaskStatusUpdateEvent with completed state."""
    context = _make_context("Hello")
    event_queue = EventQueue()

    await executor.execute(context, event_queue)

    events = []
    while not event_queue.queue.empty():
        events.append(await event_queue.dequeue_event(no_wait=True))

    status_events = [e for e in events if isinstance(e, TaskStatusUpdateEvent)]
    assert len(status_events) == 1
    assert status_events[0].status.state == TaskState.completed
    assert status_events[0].final is True


async def test_execute_raises_without_model_config(executor_no_model):
    """execute() raises ProviderNotConfiguredError when model config is absent."""
    context = _make_context("Hello")
    event_queue = EventQueue()

    with pytest.raises(ProviderNotConfiguredError, match="Model configuration is required"):
        await executor_no_model.execute(context, event_queue)


async def test_execute_returns_structured_output(executor):
    """execute() produces a data artifact containing structured output parts."""
    context = _make_context("Tell me something")
    event_queue = EventQueue()

    await executor.execute(context, event_queue)

    events = []
    while not event_queue.queue.empty():
        events.append(await event_queue.dequeue_event(no_wait=True))

    artifact_events = [e for e in events if isinstance(e, TaskArtifactUpdateEvent)]
    assert len(artifact_events) == 1
    artifact = artifact_events[0].artifact
    # The artifact should have data parts from TestModel's structured output
    assert artifact.parts is not None
    assert len(artifact.parts) > 0


async def test_execute_event_ordering(executor):
    """Events are enqueued in order: Task, ArtifactUpdate, StatusUpdate."""
    context = _make_context("Order test")
    event_queue = EventQueue()

    await executor.execute(context, event_queue)

    events = []
    while not event_queue.queue.empty():
        events.append(await event_queue.dequeue_event(no_wait=True))

    assert isinstance(events[0], Task)
    assert isinstance(events[1], TaskArtifactUpdateEvent)
    assert isinstance(events[2], TaskStatusUpdateEvent)


async def test_execute_task_ids_consistent(executor):
    """All events reference the same task_id and context_id."""
    context = _make_context("Consistency test")
    event_queue = EventQueue()

    await executor.execute(context, event_queue)

    events = []
    while not event_queue.queue.empty():
        events.append(await event_queue.dequeue_event(no_wait=True))

    task = events[0]
    assert isinstance(task, Task)
    task_id = task.id
    context_id = task.context_id

    for event in events[1:]:
        assert event.task_id == task_id
        assert event.context_id == context_id
