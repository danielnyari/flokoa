"""Flokoa generic runner.

Hydrates a compiled pydantic-ai AgentSpec (delivered by the operator per the
runtime contract, see docs/reference/runtime-contract.md), resolves secret
placeholders, installs capability wheelhouses, constructs the agent via
``Agent.from_spec``, and serves it over A2A.
"""

__all__ = ["CONTRACT_VERSION", "RUNNER_VERSION"]

CONTRACT_VERSION = 1
RUNNER_VERSION = "0.2.0"
