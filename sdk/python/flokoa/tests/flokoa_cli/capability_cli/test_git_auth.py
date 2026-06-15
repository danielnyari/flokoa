"""git_auth.py: --from-git grammar matrix + host-side auth precedence (mocked).

No real network or tokens — git/gh subprocesses are mocked. The token is never
returned in a form that could leak into argv (it is a plain string the build
step routes into exec(secret_env=…); the never-in-argv guarantee is asserted in
test_build_command's git suite).
"""

from __future__ import annotations

from unittest import mock

import pytest

from flokoa.capability_cli import git_auth
from flokoa.capability_cli.errors import CapabilityCliError


class TestParseFromGitAccept:
    @pytest.mark.parametrize(
        ("value", "scheme", "url", "ref", "subdir"),
        [
            ("git+https://github.com/org/repo", "https", "https://github.com/org/repo", None, None),
            ("git+https://github.com/org/repo.git", "https", "https://github.com/org/repo", None, None),
            ("git+https://github.com/org/repo@v1.2.0", "https", "https://github.com/org/repo", "v1.2.0", None),
            ("git+https://github.com/org/repo.git@main", "https", "https://github.com/org/repo", "main", None),
            (
                "git+https://github.com/org/repo@v1#subdirectory=pkg/cap",
                "https",
                "https://github.com/org/repo",
                "v1",
                "pkg/cap",
            ),
            (
                "git+https://github.com/org/repo#subdirectory=pkg/cap",
                "https",
                "https://github.com/org/repo",
                None,
                "pkg/cap",
            ),
            ("git+ssh://git@github.com/org/repo", "ssh", "ssh://git@github.com/org/repo", None, None),
            ("git+ssh://git@github.com/org/repo@v2", "ssh", "ssh://git@github.com/org/repo", "v2", None),
            ("git+https://gitlab.example.com/g/r", "https", "https://gitlab.example.com/g/r", None, None),
            ("git+https://github.com/org/repo@" + "a" * 40, "https", "https://github.com/org/repo", "a" * 40, None),
            # Non-standard port: the _FROM_GIT_PATTERN accepts an optional :port
            # component; the parsed URL preserves it so the clone step targets the
            # correct self-hosted instance.
            (
                "git+https://gitlab.internal.example.com:8443/g/r",
                "https",
                "https://gitlab.internal.example.com:8443/g/r",
                None,
                None,
            ),
        ],
    )
    def test_accepts_and_decomposes(
        self, value: str, scheme: str, url: str, ref: str | None, subdir: str | None
    ) -> None:
        parsed = git_auth.parse_from_git(value)
        assert parsed.scheme == scheme
        assert parsed.url == url
        assert parsed.ref == ref
        assert parsed.subdirectory == subdir

    def test_clean_url_carries_no_credentials(self) -> None:
        # The clean URL never embeds user:token@ — credentials are resolved
        # separately and never travel in the recorded URL.
        parsed = git_auth.parse_from_git("git+ssh://git@github.com/org/repo")
        # ssh keeps the bare `git@` user (no password), https drops userinfo.
        assert "@" in parsed.url  # the ssh `git@`
        assert ":" not in parsed.url.split("//", 1)[1].split("/", 1)[0].rstrip()  # no `user:pass`


class TestParseFromGitReject:
    @pytest.mark.parametrize(
        "value",
        [
            "https://github.com/org/repo",  # bare https, missing git+
            "github.com/org/repo",  # no scheme
            "git+ftp://github.com/org/repo",  # unsupported scheme
            "git+https://github.com/org/repo[extra]",  # extras
            "git+https://github.com/org/repo --extra-index-url https://evil",  # pip option
            'git+https://github.com/org/repo; python_version>"3"',  # env marker
            "git+https://user:tok@github.com/org/repo",  # embedded https credentials
            "git+ssh://git:pass@github.com/org/repo",  # embedded ssh password
            "pkg==1.0",  # plain pypi name
            "-r requirements.txt",  # pip requirements file
            "git+https://github.com/org/repo @v1",  # space smuggling
        ],
    )
    def test_rejects_with_guidance(self, value: str) -> None:
        with pytest.raises(CapabilityCliError, match="--from-git must be"):
            git_auth.parse_from_git(value)


class TestParseFromGitHttpsUserinfo:
    """git+https URLs must not carry a `user@` prefix: the username is dropped
    (credentials are resolved separately), so honoring it would mislead. ssh's
    bare `git@` user stays allowed, and a credential-free git+https is fine."""

    def test_https_user_prefix_is_rejected(self) -> None:
        with pytest.raises(CapabilityCliError, match="must not include a 'user@' prefix"):
            git_auth.parse_from_git("git+https://user@github.com/o/r")

    def test_ssh_user_prefix_still_accepted(self) -> None:
        parsed = git_auth.parse_from_git("git+ssh://git@github.com/o/r")
        assert parsed.scheme == "ssh"
        assert parsed.url == "ssh://git@github.com/o/r"

    def test_https_without_user_prefix_still_accepted(self) -> None:
        parsed = git_auth.parse_from_git("git+https://github.com/o/r")
        assert parsed.scheme == "https"
        assert parsed.url == "https://github.com/o/r"


class TestParseFromGitPort:
    """A non-standard `:port` is captured on ParsedGitSource (the credential
    lookup needs it: git keys creds per host[:port])."""

    def test_port_recorded_on_parsed_source(self) -> None:
        parsed = git_auth.parse_from_git("git+https://gitlab.internal.example.com:8443/g/r")
        assert parsed.host == "gitlab.internal.example.com"
        assert parsed.port == "8443"
        assert parsed.url == "https://gitlab.internal.example.com:8443/g/r"

    def test_no_port_is_none(self) -> None:
        parsed = git_auth.parse_from_git("git+https://github.com/org/repo")
        assert parsed.port is None


class TestParseFromGitRejectPathTraversal:
    """`..` components in the #subdirectory= or @ref are rejected on the HOST,
    before the value reaches the build container (the clone step bound-checks
    the subdirectory defensively too)."""

    @pytest.mark.parametrize(
        "value",
        [
            "git+https://github.com/org/repo#subdirectory=../../etc",
            "git+https://github.com/org/repo#subdirectory=pkg/../../etc",
            "git+https://github.com/org/repo@v1#subdirectory=..",
            "git+https://github.com/org/repo.git#subdirectory=a/../../b",
        ],
    )
    def test_rejects_subdirectory_with_dotdot(self, value: str) -> None:
        with pytest.raises(CapabilityCliError, match="must not contain a '..' path component"):
            git_auth.parse_from_git(value)

    @pytest.mark.parametrize(
        "value",
        [
            "git+https://github.com/org/repo@../etc",
            "git+https://github.com/org/repo@feature/../../etc",
            "git+https://github.com/org/repo@..",
        ],
    )
    def test_rejects_ref_with_dotdot(self, value: str) -> None:
        with pytest.raises(CapabilityCliError, match="@ref must not contain a '..' path component"):
            git_auth.parse_from_git(value)

    @pytest.mark.parametrize(
        ("value", "ref", "subdir"),
        [
            # A normal ref/tag/sha and a normal subdir are still accepted — the
            # `..` guard must not reject legitimate `.`-bearing values.
            ("git+https://github.com/org/repo@v1.2.0", "v1.2.0", None),
            ("git+https://github.com/org/repo@release/1.2.x", "release/1.2.x", None),
            ("git+https://github.com/org/repo@" + "a" * 40, "a" * 40, None),
            ("git+https://github.com/org/repo#subdirectory=pkg.sub/cap", None, "pkg.sub/cap"),
        ],
    )
    def test_accepts_normal_ref_and_subdir(self, value: str, ref: str | None, subdir: str | None) -> None:
        parsed = git_auth.parse_from_git(value)
        assert parsed.ref == ref
        assert parsed.subdirectory == subdir


class TestHttpsAuthPrecedence:
    """Ambient (git credential fill, then gh auth token) before the env-var
    fallback (GITHUB_TOKEN then GH_TOKEN). All subprocesses mocked."""

    @pytest.fixture(autouse=True)
    def _clean_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("GITHUB_TOKEN", raising=False)
        monkeypatch.delenv("GH_TOKEN", raising=False)

    def _completed(self, returncode: int, stdout: str = "") -> mock.Mock:
        return mock.Mock(returncode=returncode, stdout=stdout, stderr="")

    def test_git_credential_helper_wins(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GITHUB_TOKEN", "env-token")  # must NOT be used
        with (
            mock.patch.object(git_auth.shutil, "which", side_effect=lambda t: f"/usr/bin/{t}"),
            mock.patch.object(
                git_auth,
                "_run",
                return_value=self._completed(0, "protocol=https\nhost=github.com\npassword=ambient-pat\n"),
            ),
        ):
            auth = git_auth.resolve_https_auth("github.com")
        assert auth.token == "ambient-pat"
        assert "git credential" in auth.source

    def test_gh_auth_token_when_no_credential_helper(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GH_TOKEN", "env-token")  # must NOT be used

        def fake_run(argv, *, input_text=None):
            if argv[:2] == ["git", "credential"]:
                return self._completed(1)  # no helper / no credential
            if argv[:3] == ["gh", "auth", "token"]:
                return self._completed(0, "gh-cli-token\n")
            return self._completed(1)

        with (
            mock.patch.object(git_auth.shutil, "which", side_effect=lambda t: f"/usr/bin/{t}"),
            mock.patch.object(git_auth, "_run", side_effect=fake_run),
        ):
            auth = git_auth.resolve_https_auth("github.com")
        assert auth.token == "gh-cli-token"
        assert "gh auth" in auth.source

    def test_github_token_env_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GITHUB_TOKEN", "ghp_env_value")
        with (
            mock.patch.object(git_auth.shutil, "which", return_value=None),  # no git, no gh
        ):
            auth = git_auth.resolve_https_auth("github.com")
        assert auth.token == "ghp_env_value"
        assert auth.source == "$GITHUB_TOKEN"

    def test_gh_token_after_github_token(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GH_TOKEN", "ghp_gh_token")
        with mock.patch.object(git_auth.shutil, "which", return_value=None):
            auth = git_auth.resolve_https_auth("github.com")
        assert auth.token == "ghp_gh_token"
        assert auth.source == "$GH_TOKEN"

    def test_github_token_beats_gh_token(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GITHUB_TOKEN", "first")
        monkeypatch.setenv("GH_TOKEN", "second")
        with mock.patch.object(git_auth.shutil, "which", return_value=None):
            auth = git_auth.resolve_https_auth("github.com")
        assert auth.token == "first"

    def test_nothing_resolves_is_not_an_error(self) -> None:
        # Public repo: no credential resolves, no exception — the clone proceeds.
        with mock.patch.object(git_auth.shutil, "which", return_value=None):
            auth = git_auth.resolve_https_auth("github.com")
        assert auth.token is None
        assert auth.source == "none"


class TestHttpsAuthPort:
    """A non-standard port must reach `git credential fill` as host=HOST:PORT
    so git's per-host[:port] credential store resolves creds for self-hosted
    instances."""

    @pytest.fixture(autouse=True)
    def _clean_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("GITHUB_TOKEN", raising=False)
        monkeypatch.delenv("GH_TOKEN", raising=False)

    def test_credential_fill_request_includes_port(self, monkeypatch: pytest.MonkeyPatch) -> None:
        seen_inputs: list[str | None] = []

        def fake_run(argv, *, input_text=None):
            seen_inputs.append(input_text)
            if argv[:2] == ["git", "credential"]:
                return mock.Mock(
                    returncode=0,
                    stdout="protocol=https\nhost=gitlab.internal.example.com:8443\npassword=port-pat\n",
                    stderr="",
                )
            return mock.Mock(returncode=1, stdout="", stderr="")

        parsed = git_auth.parse_from_git("git+https://gitlab.internal.example.com:8443/g/r")
        assert parsed.port == "8443"
        with (
            mock.patch.object(git_auth.shutil, "which", side_effect=lambda t: f"/usr/bin/{t}"),
            mock.patch.object(git_auth, "_run", side_effect=fake_run),
        ):
            auth = git_auth.resolve_auth(parsed)
        assert auth.token == "port-pat"
        # The credential request stdin carried the host WITH the port.
        assert any(line and "host=gitlab.internal.example.com:8443" in line for line in seen_inputs)
        assert not any(line and "host=gitlab.internal.example.com\n" in line for line in seen_inputs)

    def test_auth_failure_message_shows_port(self) -> None:
        parsed = git_auth.parse_from_git("git+https://gitlab.internal.example.com:8443/g/r")
        msg = git_auth.auth_failure_message(parsed)
        assert "gitlab.internal.example.com:8443" in msg


class TestSshAuth:
    def test_agent_socket_forwarded(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("SSH_AUTH_SOCK", "/run/agent.sock")
        auth = git_auth.resolve_ssh_auth()
        assert auth.ssh_auth_sock == "/run/agent.sock"
        assert auth.token is None

    def test_no_agent_is_not_fatal(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("SSH_AUTH_SOCK", raising=False)
        auth = git_auth.resolve_ssh_auth()
        assert auth.ssh_auth_sock is None


class TestResolveAuthDispatch:
    def test_ssh_source_uses_ssh_resolver(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("SSH_AUTH_SOCK", "/run/agent.sock")
        parsed = git_auth.parse_from_git("git+ssh://git@github.com/org/repo")
        auth = git_auth.resolve_auth(parsed)
        assert auth.ssh_auth_sock == "/run/agent.sock"

    def test_https_source_uses_https_resolver(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GITHUB_TOKEN", "tok")
        with mock.patch.object(git_auth.shutil, "which", return_value=None):
            parsed = git_auth.parse_from_git("git+https://github.com/org/repo")
            auth = git_auth.resolve_auth(parsed)
        assert auth.token == "tok"


class TestAuthFailureMessage:
    def test_https_names_full_precedence(self) -> None:
        parsed = git_auth.parse_from_git("git+https://github.com/org/repo")
        msg = git_auth.auth_failure_message(parsed)
        assert "git credential" in msg
        assert "gh auth token" in msg
        assert "GITHUB_TOKEN" in msg
        assert "GH_TOKEN" in msg

    def test_ssh_names_agent(self) -> None:
        parsed = git_auth.parse_from_git("git+ssh://git@github.com/org/repo")
        msg = git_auth.auth_failure_message(parsed)
        assert "SSH agent" in msg or "SSH_AUTH_SOCK" in msg
