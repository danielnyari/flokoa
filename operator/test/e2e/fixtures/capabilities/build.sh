#!/usr/bin/env bash
# build.sh — pre-CLI build path for the reference capability fixtures
# (roadmap 09). Unit 10's `flokoa capability build` supersedes this script;
# CI then diffs the manifests both paths produce (the dogfood test).
#
# Usage: build.sh <fixture-dir> [image-tag]
#
# Pipeline (mirrors runtime contract §4):
#   1. `pip wheel` inside the pinned runner image — the wheel is built against
#      the exact environment it will install into. The runner version is read
#      from spec.DefaultRunnerVersion (operator/internal/spec/spec.go).
#   2. Wheel each pinned non-baseline dependency from the fixture's
#      artifact.json (`--no-deps`: the pre-CLI closure is declared statically
#      per fixture; the CLI computes it for real).
#   3. Refuse anything that is not a wheel (wheels-only boundary).
#   4. Assemble manifest.json (jq): artifact.json + computed wheels[{file,sha256}]
#      (+ schemaDigest when configSchema is present).
#   5. `docker build` the busybox artifact image with the contract labels.
#
# Env knobs:
#   CONTAINER_TOOL              docker (default) | podman
#   CAPABILITY_IMAGE_PLATFORM   default linux/amd64 (matches the petstore fixture)
#   FLOKOA_RUNNER_IMAGE         full runner image override
#   FLOKOA_RUNNER_REPOSITORY    default ghcr.io/danielnyari/flokoa-runner
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPERATOR_DIR="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"

FIXTURE_DIR_ARG="${1:?usage: build.sh <fixture-dir> [image-tag]}"
FIXTURE_DIR="$(cd "${FIXTURE_DIR_ARG}" && pwd)"

CONTAINER_TOOL="${CONTAINER_TOOL:-docker}"
PLATFORM="${CAPABILITY_IMAGE_PLATFORM:-linux/amd64}"

command -v jq >/dev/null 2>&1 || { echo "ERROR: jq is required" >&2; exit 1; }
command -v "${CONTAINER_TOOL}" >/dev/null 2>&1 || { echo "ERROR: ${CONTAINER_TOOL} is required" >&2; exit 1; }

ARTIFACT_JSON="${FIXTURE_DIR}/artifact.json"
[ -f "${ARTIFACT_JSON}" ] || { echo "ERROR: ${ARTIFACT_JSON} not found" >&2; exit 1; }

NAME="$(jq -re '.name' "${ARTIFACT_JSON}")"
VERSION="$(jq -re '.version' "${ARTIFACT_JSON}")"
CONTRACT_VERSION="$(jq -re '.contractVersion' "${ARTIFACT_JSON}")"
IMAGE_TAG="${2:-${NAME}:test}"

# Resolve the pinned runner image from spec.DefaultRunnerVersion — read, never
# modified; the runner tag and this constant move together (release process).
SPEC_GO="${OPERATOR_DIR}/internal/spec/spec.go"
RUNNER_VERSION="$(sed -n 's/^var DefaultRunnerVersion = "\(.*\)"$/\1/p' "${SPEC_GO}")"
[ -n "${RUNNER_VERSION}" ] || { echo "ERROR: could not read DefaultRunnerVersion from ${SPEC_GO}" >&2; exit 1; }
RUNNER_IMAGE="${FLOKOA_RUNNER_IMAGE:-${FLOKOA_RUNNER_REPOSITORY:-ghcr.io/danielnyari/flokoa-runner}:${RUNNER_VERSION}}"

BUILD_DIR="${FIXTURE_DIR}/dist"
WHEELHOUSE="${BUILD_DIR}/wheelhouse"
rm -rf "${BUILD_DIR}"
mkdir -p "${WHEELHOUSE}"

# Dependency pins (the statically declared non-baseline closure).
DEP_PINS=()
while IFS= read -r pin; do
  [ -n "${pin}" ] && DEP_PINS+=("${pin}")
done < <(jq -r '.dependencies // [] | .[]' "${ARTIFACT_JSON}")

echo "Building wheelhouse for ${NAME}==${VERSION} inside ${RUNNER_IMAGE}..."
# The runner venv is uv-managed and ships without pip; ensurepip seeds it
# inside the disposable build container only. Root user: the build container
# is a throwaway compiler, not the pod path. Because root writes the wheels to
# the /out bind mount, they land root-owned on the host; reclaim them for the
# invoking user (numeric ids, no passwd entry needed) so the host-side chmod
# and manifest steps work on Linux CI. macOS Docker Desktop already remaps
# ownership, so the chown is a harmless no-op there (|| true guards the rare
# file-sharing driver that rejects it).
"${CONTAINER_TOOL}" run --rm \
  --user 0 \
  -e HOME=/tmp \
  -e HOST_UID="$(id -u)" \
  -e HOST_GID="$(id -g)" \
  -v "${FIXTURE_DIR}:/src:ro" \
  -v "${WHEELHOUSE}:/out" \
  --entrypoint /bin/sh \
  "${RUNNER_IMAGE}" \
  -c 'set -eu;
      python -m pip --version >/dev/null 2>&1 || python -m ensurepip --upgrade >/dev/null;
      python -m pip wheel --no-deps --wheel-dir /out /src;
      for pin in "$@"; do
        python -m pip wheel --no-deps --wheel-dir /out "${pin}";
      done;
      chown -R "${HOST_UID}:${HOST_GID}" /out 2>/dev/null || true' build-wheelhouse ${DEP_PINS[@]+"${DEP_PINS[@]}"}

# Wheels-only boundary (runtime contract §4): refuse sdists or anything else.
ls "${WHEELHOUSE}"/*.whl >/dev/null 2>&1 || { echo "ERROR: wheelhouse is empty" >&2; exit 1; }
for f in "${WHEELHOUSE}"/*; do
  case "$(basename "${f}")" in
    *.whl) ;;
    *) echo "ERROR: non-wheel file in wheelhouse: $(basename "${f}") — wheels only; system deps belong in custom agent images" >&2
       exit 1 ;;
  esac
done

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# Assemble manifest.json = artifact.json + wheels [+ schemaDigest].
WHEELS_JSON="[]"
for f in "${WHEELHOUSE}"/*.whl; do
  base="$(basename "${f}")"
  sha="$(sha256_of "${f}")"
  WHEELS_JSON="$(jq -c --arg file "${base}" --arg sha "${sha}" '. + [{file: $file, sha256: $sha}]' <<<"${WHEELS_JSON}")"
done

MANIFEST="$(jq -S --argjson wheels "${WHEELS_JSON}" '. + {wheels: $wheels}' "${ARTIFACT_JSON}")"
if jq -e '.configSchema' "${ARTIFACT_JSON}" >/dev/null; then
  # schemaDigest = sha256 of the canonical (sorted-keys, compact) configSchema.
  CANONICAL_SCHEMA="$(jq -cS '.configSchema' "${ARTIFACT_JSON}")"
  SCHEMA_TMP="$(mktemp)"
  printf '%s' "${CANONICAL_SCHEMA}" > "${SCHEMA_TMP}"
  SCHEMA_DIGEST="sha256:$(sha256_of "${SCHEMA_TMP}")"
  rm -f "${SCHEMA_TMP}"
  MANIFEST="$(jq -S --arg digest "${SCHEMA_DIGEST}" '. + {schemaDigest: $digest}' <<<"${MANIFEST}")"
fi
printf '%s\n' "${MANIFEST}" > "${WHEELHOUSE}/manifest.json"
chmod 0644 "${WHEELHOUSE}"/*

echo "Building artifact image ${IMAGE_TAG} (${PLATFORM})..."
BUILD_ARGS=(
  --platform="${PLATFORM}"
  -f "${SCRIPT_DIR}/Dockerfile"
  --build-arg "CAPABILITY_NAME=${NAME}"
  --build-arg "CAPABILITY_VERSION=${VERSION}"
  --build-arg "CONTRACT_VERSION=${CONTRACT_VERSION}"
  -t "${IMAGE_TAG}"
  "${BUILD_DIR}"
)
if [ "${CONTAINER_TOOL}" = "docker" ]; then
  docker buildx build --load "${BUILD_ARGS[@]}"
else
  "${CONTAINER_TOOL}" build "${BUILD_ARGS[@]}"
fi

echo "Built ${IMAGE_TAG}: $(jq -r '.wheels | length' <<<"${MANIFEST}") wheel(s) in the wheelhouse"
