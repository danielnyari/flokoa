"""Errors for the ``flokoa capability`` CLI."""

from __future__ import annotations

import click


class CapabilityCliError(click.ClickException):
    """A user-facing CLI failure (preflight, build, validation).

    Subclassing ``click.ClickException`` keeps command code free of
    try/except plumbing: raising anywhere under a command prints
    ``Error: <message>`` and exits 1.
    """
