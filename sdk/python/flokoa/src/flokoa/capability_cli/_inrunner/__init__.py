"""Self-contained scripts executed INSIDE the pinned runner image.

Bind-mounted read-only into the build container; outputs land in the
bind-mounted work directory. They may import only the standard library plus
packages guaranteed by the runner baseline (pydantic, packaging) — never
``flokoa`` itself: the runner venv is the import environment, not the dev
CLI venv. This ``__init__`` exists only so host-side unit tests can import
the modules; the container runs them as plain scripts.
"""
