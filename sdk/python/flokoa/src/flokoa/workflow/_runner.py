"""Node runner — executes a single pydantic-graph node inside a container.

Invoked by Argo Workflow tasks as::

    python -m flokoa.workflow.runner \\
        --node my_module.FetchData \\
        --state '{"key": "value"}'

Or for bundled cyclic subgraphs::

    python -m flokoa.workflow.runner \\
        --graph my_module.graph \\
        --entry-node my_module.Review \\
        --state '{"key": "value"}'

Outputs are written to files under ``/tmp/flokoa/`` so Argo can
pick them up as output parameters::

    /tmp/flokoa/next    — name of the next step (or ``__end__``)
    /tmp/flokoa/state   — JSON-serialized updated state
    /tmp/flokoa/result  — JSON-serialized final result (on ``End``)
"""

from __future__ import annotations

import asyncio
import importlib
import json
import sys
from pathlib import Path

OUTPUT_DIR = Path("/tmp/flokoa")  # noqa: S108


def _import_object(dotted_path: str) -> object:
    """Import ``module.path.ClassName`` and return the object."""
    module_path, _, attr_name = dotted_path.rpartition(".")
    if not module_path:
        raise ImportError(f"Invalid dotted path (no module): {dotted_path}")
    module = importlib.import_module(module_path)
    return getattr(module, attr_name)


def _write_outputs(*, next_step: str, state: str, result: str | None = None) -> None:
    """Write runner outputs to the Argo output parameter directory."""
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    (OUTPUT_DIR / "next").write_text(next_step)
    (OUTPUT_DIR / "state").write_text(state)
    if result is not None:
        (OUTPUT_DIR / "result").write_text(result)


def run_node(node_class_path: str, state_json: str) -> None:
    """Import a node class, run it, and write outputs."""
    from pydantic_graph import End, GraphRunContext

    node_cls = _import_object(node_class_path)
    state = json.loads(state_json) if state_json else None

    node = node_cls()
    ctx = GraphRunContext(state=state, deps=None)
    result = asyncio.run(node.run(ctx))

    if isinstance(result, End):
        _write_outputs(
            next_step="__end__",
            state=json.dumps(state),
            result=json.dumps(result.data),
        )
    else:
        _write_outputs(
            next_step=type(result).__name__,
            state=json.dumps(state),
        )


def run_bundle(graph_path: str, entry_node_path: str, state_json: str) -> None:
    """Run a bundled cyclic subgraph in-process via ``graph.run()``."""
    graph = _import_object(graph_path)
    entry_cls = _import_object(entry_node_path)
    state = json.loads(state_json) if state_json else None

    entry_node = entry_cls()
    run_result = asyncio.run(graph.run(entry_node, state=state))

    _write_outputs(
        next_step="__end__",
        state=json.dumps(state),
        result=json.dumps(run_result.output),
    )


def main(argv: list[str] | None = None) -> None:
    """CLI entrypoint."""
    args = argv or sys.argv[1:]

    parsed: dict[str, str] = {}
    i = 0
    while i < len(args):
        if args[i].startswith("--") and i + 1 < len(args):
            key = args[i].lstrip("-").replace("-", "_")
            parsed[key] = args[i + 1]
            i += 2
        else:
            i += 1

    state = parsed.get("state", "")

    if "graph" in parsed:
        run_bundle(
            graph_path=parsed["graph"],
            entry_node_path=parsed["entry_node"],
            state_json=state,
        )
    elif "node" in parsed:
        run_node(
            node_class_path=parsed["node"],
            state_json=state,
        )
    else:
        print("Usage: python -m flokoa.workflow.runner --node <path> --state <json>", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
