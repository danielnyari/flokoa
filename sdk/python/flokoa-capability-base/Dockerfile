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

FROM ghcr.io/danielnyari/flokoa-runner:${RUNNER_VERSION}

# Re-declare after FROM so the LABEL below can interpolate it.
ARG RUNNER_VERSION=0.2.0

# The runner image runs as the non-root pod user (uid 65532); seeding the venv
# needs write access to /app/.venv, so switch to root for the build layer.
USER 0

# Seed pip + the build front-end into the runner venv ONCE, at image-build
# time. ensurepip uses the CPython bundled wheel (offline, deterministic per
# base-image patch version); wheel + setuptools come from PyPI so the build
# front-end is complete and current. pip remains a *build* tool here — it is
# intentionally absent from runner.lock / runner-manifest.json (the locked
# baseline), exactly as in the runner image.
RUN /app/.venv/bin/python -m ensurepip --upgrade \
 && /app/.venv/bin/python -m pip install --no-cache-dir --upgrade pip wheel setuptools \
 && /app/.venv/bin/python -m pip --version

# git+ssh capability builds (`flokoa capability build --from-git git+ssh://…`)
# clone with `ssh -o StrictHostKeyChecking=yes` (clone_repo.py). The runner base
# (python:3.13-slim) ships neither the ssh client nor any known_hosts, so strict
# host-key checking would reject every host as unknown and the clone would fail.
# Install the ssh client and seed GitHub's published host keys into the system
# known_hosts so git+ssh against github.com works out of the box without
# disabling host-key verification. github.com is the supported host; the keys are
# baked statically (not ssh-keyscan'd at build time) so the image is reproducible
# and the build needs no network access to github.com:22.
RUN apt-get update \
 && apt-get install -y --no-install-recommends openssh-client \
 && rm -rf /var/lib/apt/lists/*
COPY flokoa-capability-base/github-known-hosts /etc/ssh/ssh_known_hosts
RUN chmod 0644 /etc/ssh/ssh_known_hosts

LABEL ai.flokoa.runner-version="${RUNNER_VERSION}" \
      ai.flokoa.capability-base="true"

# No ENTRYPOINT override: the CLI starts the container with
# `--entrypoint sleep ... infinity` and drives each step via `<tool> exec`
# (capability_cli/container.py). The image just needs python + pip on PATH,
# both inherited from the runner venv (PATH=/app/.venv/bin:$PATH).
