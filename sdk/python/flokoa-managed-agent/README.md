# flokoa-managed-agent

Managed agent runtime for Flokoa operator-deployed pydantic-ai agents.

This package is built into a container image and deployed by the Flokoa Kubernetes Operator. It is **not** published to PyPI.

## Usage

The operator mounts configuration at `/etc/flokoa/` and runs:

```
python -m flokoa_managed_agent
```
