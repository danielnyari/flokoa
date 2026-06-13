"""``flokoa capability`` — authoring tooling for capability artifacts.

Roadmap unit 10: ``build`` produces the artifact + CR, ``push`` publishes and
digest-pins, ``import`` turns a PyPI package into both, ``search``/``list``
discover what is published and what is in the cluster.
"""

from __future__ import annotations

import click

# Aliased import: rebinding the bare name `build` would shadow the
# flokoa.capability_cli.build submodule on the package.
from flokoa.capability_cli.build import build as build_command
from flokoa.capability_cli.import_cmd import import_command
from flokoa.capability_cli.push import push as push_command
from flokoa.capability_cli.search import list_command
from flokoa.capability_cli.search import search as search_command


@click.group()
def capability() -> None:
    """Build, publish, and discover capability artifacts."""


capability.add_command(build_command)
capability.add_command(push_command)
capability.add_command(import_command)
capability.add_command(search_command)
capability.add_command(list_command)

__all__ = ["capability"]
