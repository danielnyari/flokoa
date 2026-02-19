"""E2E tests for the managed-task Docker container."""


def test_task_runs_successfully(run_task: str) -> None:
    """The managed-task container exits 0 and logs a completion message."""
    assert "Task completed, writing A2A artifact" in run_task


def test_task_output_is_valid_artifact(run_task: str) -> None:
    """The managed-task container produces valid A2A artifact JSON on stdout.

    The last line of stdout after the log lines is not the artifact (it goes
    to /tmp/output inside the container).  Instead we verify the logs confirm
    the task completed and produced a non-trivial artifact.
    """
    # The log line "Task completed, writing A2A artifact (N chars)" tells us
    # the artifact was serialised.  Parse N to ensure it's > 0.
    for line in run_task.splitlines():
        if "writing A2A artifact" in line:
            # e.g. "... writing A2A artifact (794 chars)"
            chars = int(line.split("(")[1].split(" ")[0])
            assert chars > 0, "Artifact should have non-zero length"
            return

    raise AssertionError("Did not find artifact log line in task output")
