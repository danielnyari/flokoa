# ============================================================================
# flokoa-capability-base — the capability *build* image (source tier: image)
#
# `flokoa capability build` runs its `_inrunner/` pipeline inside this image
# by default. It is literally the pinned runner image plus the build toolchain
# (pip + wheel + setuptools), so:
#
#   * the wheelhouse is resolved against the EXACT environment a runner pod has
#     (same baseline — the compatibility matrix holds by construction), and
#   * pip is seeded ONCE at image-build time, not per CLI invocation: the old
#     per-build `ensurepip` hack (build_wheelhouse.ensure_pip, runner Dockerfile
#     comment) is no longer on the happy path. It stays in the CLI only as a
#     safety net for someone overriding --runner-image to a bare runner.
#
# Lean by design: the wheels-only boundary means the build image carries no
# compilers or system-dev headers. Only the Python build front-end (pip/wheel/
# setuptools) is added on top of the runner.
#
# Tagged to the runner version it derives from
# (ghcr.io/danielnyari/flokoa-capability-base:<runnerVersion>), matching the
# CLI's DEFAULT_RUNNER_VERSION (capability_cli/container.py) and the operator's
# spec.DefaultRunnerVersion — all three move together via release.yml.
#
# Build (from sdk/python/):
#   docker build -f flokoa-capability-base/Dockerfile \
#     --build-arg RUNNER_VERSION=0.2.0 -t flokoa-capability-base:local .
# (use a runner image tag you can pull or have built locally)
# ============================================================================

ARG RUNNER_VERSION=0.2.0

# Pinned build front-end versions. These are NOT the runtime baseline (that
# lives in runner.lock / runner-manifest.json) — they are the pip/wheel/
# setuptools that resolve the wheelhouse at capability-build time. Pinning them
# makes flokoa-capability-base:<runnerVersion> deterministic per runner release:
# rebuilding the same tag yields the same build toolchain instead of whatever
# happened to be latest on PyPI that day. Override per build with
# `--build-arg PIP_VERSION=… --build-arg WHEEL_VERSION=… --build-arg
# SETUPTOOLS_VERSION=…` if a newer front-end is needed.
ARG PIP_VERSION=26.1.2
ARG WHEEL_VERSION=0.47.0
ARG SETUPTOOLS_VERSION=82.0.1

FROM ghcr.io/danielnyari/flokoa-runner:${RUNNER_VERSION}

# Re-declare after FROM so the LABEL and the RUN below can interpolate them.
ARG RUNNER_VERSION=0.2.0
ARG PIP_VERSION=26.1.2
ARG WHEEL_VERSION=0.47.0
ARG SETUPTOOLS_VERSION=82.0.1

# The runner image runs as the non-root pod user (uid 65532); seeding the venv
# needs write access to /app/.venv, so switch to root for the build layer.
USER 0

# Seed pip + the build front-end into the runner venv ONCE, at image-build
# time. ensurepip bootstraps pip from the CPython bundled wheel (offline) so we
# have a pip to then install the exact pins with; the subsequent install pins
# pip, wheel, and setuptools to the ARG versions so the build front-end is fully
# deterministic per runner release (overridable via the ARGs above). pip
# remains a *build* tool here — it is intentionally absent from runner.lock /
# runner-manifest.json (the locked baseline), exactly as in the runner image.
RUN /app/.venv/bin/python -m ensurepip --upgrade \
 && /app/.venv/bin/python -m pip install --no-cache-dir \
      "pip==${PIP_VERSION}" \
      "wheel==${WHEEL_VERSION}" \
      "setuptools==${SETUPTOOLS_VERSION}" \
 && /app/.venv/bin/python -m pip --version

# `flokoa capability build --from-git` shells out to `git` (clone_repo.py:
# git clone/checkout/rev-parse), and git+ssh clones additionally use
# `ssh -o StrictHostKeyChecking=yes`. The runner base (python:3.13-slim) ships
# neither git nor the ssh client nor any known_hosts, so without them a --from-git
# build fails ("git: command not found") and git+ssh would reject every host as
# unknown. Install git + the ssh client, and seed GitHub's published host keys
# into the system known_hosts so git+ssh against github.com works out of the box
# without disabling host-key verification. github.com is the supported host; the
# keys are baked statically (not ssh-keyscan'd at build time) so the image is
# reproducible and the build needs no network access to github.com:22.
RUN apt-get update \
 && apt-get install -y --no-install-recommends git openssh-client \
 && rm -rf /var/lib/apt/lists/*
COPY flokoa-capability-base/github-known-hosts /etc/ssh/ssh_known_hosts
RUN chmod 0644 /etc/ssh/ssh_known_hosts

LABEL ai.flokoa.runner-version="${RUNNER_VERSION}" \
      ai.flokoa.capability-base="true"

# No ENTRYPOINT override: the CLI starts the container with
# `--entrypoint sleep ... infinity` and drives each step via `<tool> exec`
# (capability_cli/container.py). The image just needs python + pip on PATH,
# both inherited from the runner venv (PATH=/app/.venv/bin:$PATH).
