"""Structured bootstrap failures (runtime contract §2).

Every bootstrap failure exits non-zero with a single-line JSON error on
stderr naming the stage, so ``kubectl logs`` is actionable. Bootstrap
failures are environment problems (a schema-invalid spec never reaches the
runner — the operator validates first) and must read as such. Secret values
are never included.
"""

from __future__ import annotations

import json
import sys
from typing import Any, NoReturn


class BootstrapError(Exception):
    """A bootstrap-stage failure with structured details."""

    def __init__(self, stage: str, error: str, **details: Any) -> None:
        super().__init__(f"[{stage}] {error}")
        self.stage = stage
        self.error = error
        self.details = details

    def to_json(self) -> str:
        payload: dict[str, Any] = {"stage": self.stage, "error": self.error}
        payload.update(self.details)
        return json.dumps(payload, sort_keys=True)


def fail(err: BootstrapError) -> NoReturn:
    print(err.to_json(), file=sys.stderr)
    raise SystemExit(1)
