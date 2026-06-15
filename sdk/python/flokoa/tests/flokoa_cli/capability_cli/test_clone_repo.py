"""_inrunner/clone_repo.py: the in-container clone step (subprocess mocked).

No real git/network. Proves: the token never enters the recorded URL or argv
(it rides GIT_ASKPASS), the clone-report records the clean URL + resolved
commit, ssh uses strict host-key checking, and captured stderr is redacted.
"""

from __future__ import annotations

import argparse
from pathlib import Path
from typing import Any
from unittest import mock

import pytest

from flokoa.capability_cli._inrunner import clone_repo


def _completed(returncode: int = 0, stdout: str = "", stderr: str = "") -> mock.Mock:
    return mock.Mock(returncode=returncode, stdout=stdout, stderr=stderr)


def _args(work: Path, **overrides: Any) -> argparse.Namespace:
    base: dict[str, Any] = {
        "work": work,
        "url": "https://github.com/org/repo",
        "scheme": "https",
        "ref": None,
        "subdirectory": None,
    }
    base.update(overrides)
    return argparse.Namespace(**base)


class TestCloneHttps:
    def test_clone_writes_clean_report(self, tmp_path: Path) -> None:
        commit = "a" * 40
        clone_calls: list[list[str]] = []

        def fake_run(argv, *, env=None):
            if argv[:2] == ["git", "clone"]:
                clone_calls.append(argv)
                (tmp_path / clone_repo._CHECKOUT_DIRNAME).mkdir(parents=True, exist_ok=True)
                return _completed(0)
            if "rev-parse" in argv:
                return _completed(0, stdout=f"{commit}\n")
            return _completed(0)

        with (
            mock.patch.object(clone_repo, "_run", side_effect=fake_run),
            mock.patch.object(clone_repo.argparse.ArgumentParser, "parse_args", return_value=_args(tmp_path)),
        ):
            rc = clone_repo.main()
        assert rc == 0
        import json

        report = json.loads((tmp_path / "clone-report.json").read_text())
        assert report["url"] == "https://github.com/org/repo"
        assert report["commit"] == commit
        assert report["ref"] is None
        # No ref → shallow clone of the default-branch tip (--depth 1).
        assert clone_calls[0][:5] == ["git", "clone", "--quiet", "--depth", "1"]

    def test_https_uses_git_askpass_not_url_token(self, tmp_path: Path) -> None:
        seen_envs: list[dict] = []

        def fake_run(argv, *, env=None):
            seen_envs.append(env or {})
            if argv[:2] == ["git", "clone"]:
                (tmp_path / clone_repo._CHECKOUT_DIRNAME).mkdir(parents=True, exist_ok=True)
                # The clone URL handed to git is the clean one — no creds. (The
                # URL is the positional arg before the checkout dir; --depth 1
                # may precede it on the no-ref shallow path.)
                clone_url = next(arg for arg in argv if "//" in arg)
                assert "@" not in clone_url.split("//", 1)[1].split("/", 1)[0]
                return _completed(0)
            if "rev-parse" in argv:
                return _completed(0, stdout="a" * 40 + "\n")
            return _completed(0)

        with (
            mock.patch.object(clone_repo, "_run", side_effect=fake_run),
            mock.patch.object(clone_repo.argparse.ArgumentParser, "parse_args", return_value=_args(tmp_path)),
        ):
            rc = clone_repo.main()
        assert rc == 0
        # The clone env points GIT_ASKPASS at the helper script (token via env).
        clone_env = seen_envs[0]
        assert clone_env.get("GIT_ASKPASS", "").endswith("git-askpass.sh")
        assert clone_env.get("GIT_TERMINAL_PROMPT") == "0"

    def test_clone_failure_redacts_credentialed_url(self, tmp_path: Path, capsys: pytest.CaptureFixture[str]) -> None:
        def fake_run(argv, *, env=None):
            return _completed(1, stderr="fatal: clone of https://x:ghp_LEAK@github.com/o/r failed")

        with (
            mock.patch.object(clone_repo, "_run", side_effect=fake_run),
            mock.patch.object(clone_repo.argparse.ArgumentParser, "parse_args", return_value=_args(tmp_path)),
        ):
            rc = clone_repo.main()
        assert rc == 1
        err = capsys.readouterr().err
        assert "ghp_LEAK" not in err
        assert "<redacted>@github.com" in err


class TestCloneRefAndSubdir:
    def test_ref_checked_out_and_recorded(self, tmp_path: Path) -> None:
        calls: list[list[str]] = []

        def fake_run(argv, *, env=None):
            calls.append(argv)
            if argv[:2] == ["git", "clone"]:
                (tmp_path / clone_repo._CHECKOUT_DIRNAME).mkdir(parents=True, exist_ok=True)
            if "rev-parse" in argv:
                return _completed(0, stdout="b" * 40 + "\n")
            return _completed(0)

        with (
            mock.patch.object(clone_repo, "_run", side_effect=fake_run),
            mock.patch.object(
                clone_repo.argparse.ArgumentParser, "parse_args", return_value=_args(tmp_path, ref="v1.2.0")
            ),
        ):
            rc = clone_repo.main()
        assert rc == 0
        assert any(argv[:3] == ["git", "-C", str(tmp_path / "clone")] and "checkout" in argv for argv in calls)
        # With a ref → full clone (no --depth), so the ref can resolve to any
        # branch/tag/sha, then a separate checkout.
        clone_argv = next(argv for argv in calls if argv[:2] == ["git", "clone"])
        assert "--depth" not in clone_argv
        assert clone_argv[:3] == ["git", "clone", "--quiet"]
        import json

        assert json.loads((tmp_path / "clone-report.json").read_text())["ref"] == "v1.2.0"

    def test_subdirectory_escaping_checkout_fails(self, tmp_path: Path, capsys: pytest.CaptureFixture[str]) -> None:
        # Defense in depth: the host already rejects `..` in the subdirectory, but
        # if a crafted value crosses the boundary anyway, the clone step must
        # refuse a subdirectory that resolves outside the checkout directory.
        def fake_run(argv, *, env=None):
            if argv[:2] == ["git", "clone"]:
                (tmp_path / clone_repo._CHECKOUT_DIRNAME).mkdir(parents=True, exist_ok=True)
            if "rev-parse" in argv:
                return _completed(0, stdout="e" * 40 + "\n")
            return _completed(0)

        with (
            mock.patch.object(clone_repo, "_run", side_effect=fake_run),
            mock.patch.object(
                clone_repo.argparse.ArgumentParser,
                "parse_args",
                return_value=_args(tmp_path, subdirectory="../../etc"),
            ),
        ):
            rc = clone_repo.main()
        assert rc == 1
        assert "escapes the checkout directory" in capsys.readouterr().err

    def test_missing_subdirectory_fails(self, tmp_path: Path) -> None:
        def fake_run(argv, *, env=None):
            if argv[:2] == ["git", "clone"]:
                (tmp_path / clone_repo._CHECKOUT_DIRNAME).mkdir(parents=True, exist_ok=True)
            if "rev-parse" in argv:
                return _completed(0, stdout="c" * 40 + "\n")
            return _completed(0)

        with (
            mock.patch.object(clone_repo, "_run", side_effect=fake_run),
            mock.patch.object(
                clone_repo.argparse.ArgumentParser,
                "parse_args",
                return_value=_args(tmp_path, subdirectory="does/not/exist"),
            ),
        ):
            rc = clone_repo.main()
        assert rc == 1


class TestCloneSsh:
    def test_ssh_strict_host_key(self, tmp_path: Path) -> None:
        seen_envs: list[dict] = []

        def fake_run(argv, *, env=None):
            seen_envs.append(env or {})
            if argv[:2] == ["git", "clone"]:
                (tmp_path / clone_repo._CHECKOUT_DIRNAME).mkdir(parents=True, exist_ok=True)
            if "rev-parse" in argv:
                return _completed(0, stdout="d" * 40 + "\n")
            return _completed(0)

        with (
            mock.patch.object(clone_repo, "_run", side_effect=fake_run),
            mock.patch.object(
                clone_repo.argparse.ArgumentParser,
                "parse_args",
                return_value=_args(tmp_path, url="ssh://git@github.com/org/repo", scheme="ssh"),
            ),
        ):
            rc = clone_repo.main()
        assert rc == 0
        # ssh clone enforces strict host-key checking; no GIT_ASKPASS for ssh.
        clone_env = seen_envs[0]
        assert "StrictHostKeyChecking=yes" in clone_env.get("GIT_SSH_COMMAND", "")
        assert "GIT_ASKPASS" not in clone_env
