#!/usr/bin/env bash
# local-up.sh — boot the whole Flokoa stack on a local minikube with one command.
#
# It leans on the existing Makefile targets (docker-build, deploy-full,
# deploy-e2e-testdata) and only adds the local-cluster glue around them:
#   1. start/refresh minikube + enable the yakd dashboard addon
#   2. build every image straight into minikube's docker daemon (no registry,
#      no flaky `image load`, no :latest pull surprises)
#   3. install CRDs + deploy operator, server, Argo Workflows, executor plugins
#   4. create the OpenAI secret and (optionally) the e2e sample agent
#   5. background port-forwards for the flokoa UI, Argo UI and yakd dashboard
#
# Re-running is safe: every step is idempotent.
set -euo pipefail

# --- locations -------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPERATOR_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${OPERATOR_DIR}/.." && pwd)"
PID_FILE="${OPERATOR_DIR}/bin/.local-portforward.pids"
cd "${OPERATOR_DIR}"

# --- tunables (override via env) ------------------------------------------
MINIKUBE_PROFILE="${MINIKUBE_PROFILE:-minikube}"
NAMESPACE="${E2E_NAMESPACE:-flokoa-system}"
RUNNER_VERSION="${RUNNER_VERSION:-0.2.0}"   # must match spec.DefaultRunnerVersion
RUNNER_IMAGE="ghcr.io/danielnyari/flokoa-runner:${RUNNER_VERSION}"
WITH_TESTDATA="${WITH_TESTDATA:-true}"      # deploy the petstore sample agent + workflow
CONTAINER_TOOL="${CONTAINER_TOOL:-docker}"
# local listen ports for the three UIs
FLOKOA_UI_PORT="${FLOKOA_UI_PORT:-8080}"
ARGO_UI_PORT="${ARGO_UI_PORT:-2746}"
YAKD_UI_PORT="${YAKD_UI_PORT:-8081}"

say()  { printf '\033[1;36m▸ %s\033[0m\n' "$*"; }
ok()   { printf '\033[1;32m✓ %s\033[0m\n' "$*"; }
warn() { printf '\033[1;33m! %s\033[0m\n' "$*"; }
die()  { printf '\033[1;31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

# --- 0. preflight ----------------------------------------------------------
say "Preflight checks"
for bin in minikube kubectl "${CONTAINER_TOOL}"; do
  command -v "${bin}" >/dev/null 2>&1 || die "${bin} is not installed / not on PATH"
done

# OPENAI_API_KEY: prefer the repo-root .env, fall back to the environment.
if [[ -z "${OPENAI_API_KEY:-}" && -f "${REPO_ROOT}/.env" ]]; then
  say "Loading OPENAI_API_KEY from ${REPO_ROOT}/.env"
  set -a; # shellcheck disable=SC1091
  source "${REPO_ROOT}/.env"; set +a
fi
[[ -n "${OPENAI_API_KEY:-}" ]] || die "OPENAI_API_KEY is not set (put it in ${REPO_ROOT}/.env or export it)"
ok "Tools present, OPENAI_API_KEY found"

# --- 1. minikube -----------------------------------------------------------
say "Ensuring minikube '${MINIKUBE_PROFILE}' is running"
if minikube -p "${MINIKUBE_PROFILE}" status -f '{{.Host}}' 2>/dev/null | grep -q Running \
   && minikube -p "${MINIKUBE_PROFILE}" status -f '{{.Kubelet}}' 2>/dev/null | grep -q Running; then
  ok "minikube already running"
else
  minikube -p "${MINIKUBE_PROFILE}" start --driver=docker
fi
# A stale kubeconfig endpoint is the classic 4-month-old-cluster gotcha.
minikube -p "${MINIKUBE_PROFILE}" update-context >/dev/null 2>&1 || true
kubectl config use-context "${MINIKUBE_PROFILE}" >/dev/null
say "Enabling yakd dashboard addon"
minikube -p "${MINIKUBE_PROFILE}" addons enable yakd >/dev/null 2>&1 || warn "could not enable yakd addon (continuing)"
ok "Cluster reachable: $(kubectl config current-context)"

# --- 2. build images straight into minikube's docker daemon ----------------
# Scoped to a subshell so docker-env never leaks into the deploy/testdata steps
# (deploy-e2e-testdata does its own host-build + `minikube image load`).
say "Building images into minikube's docker daemon (no registry round-trip)"
(
  eval "$(minikube -p "${MINIKUBE_PROFILE}" docker-env)"
  make docker-build CONTAINER_TOOL="${CONTAINER_TOOL}"
  say "Building generic runner image ${RUNNER_IMAGE}"
  ( cd "${REPO_ROOT}/sdk/python" \
    && "${CONTAINER_TOOL}" build -f flokoa-runner/Dockerfile -t "${RUNNER_IMAGE}" . )
)
ok "Images built inside minikube (operator, server, a2a-plugin, runner ${RUNNER_VERSION})"

# --- 3. install CRDs + deploy operator/server/argo/plugins -----------------
say "Installing CRDs"
make install
say "Deploying operator + server + Argo Workflows + executor plugins"
make deploy-full
ok "Control plane deployed"

# --- 4. OpenAI secret + sample agent --------------------------------------
say "Creating openai-api-key secret in ${NAMESPACE}"
kubectl get namespace "${NAMESPACE}" >/dev/null 2>&1 || kubectl create namespace "${NAMESPACE}"
kubectl create secret generic openai-api-key \
  --namespace="${NAMESPACE}" \
  --from-literal=api-key="${OPENAI_API_KEY}" \
  --dry-run=client -o yaml | kubectl apply -f -

if [[ "${WITH_TESTDATA}" == "true" ]]; then
  say "Deploying e2e testdata (petstore tool + sample agent + demo workflow)"
  make deploy-e2e-testdata OPENAI_API_KEY="${OPENAI_API_KEY}" E2E_NAMESPACE="${NAMESPACE}"
else
  warn "WITH_TESTDATA=false — skipping the sample agent/workflow"
fi

# --- 5. wait for the control plane to settle -------------------------------
say "Waiting for deployments to become available"
for dep in flokoa-controller flokoa-server; do
  kubectl -n "${NAMESPACE}" rollout status "deployment/${dep}" --timeout=180s || \
    warn "deployment/${dep} not ready yet — check 'kubectl -n ${NAMESPACE} get pods'"
done

# --- 6. port-forwards ------------------------------------------------------
# Tear down any forwards from a previous run first.
"${SCRIPT_DIR}/local-down.sh" --forwards-only >/dev/null 2>&1 || true
mkdir -p "${OPERATOR_DIR}/bin"
: > "${PID_FILE}"

forward() { # name ns svc localPort remotePort
  local name="$1" ns="$2" svc="$3" lport="$4" rport="$5"
  if ! kubectl -n "${ns}" get "svc/${svc}" >/dev/null 2>&1; then
    warn "${name}: svc/${svc} not found in ns ${ns} — skipping forward"
    return
  fi
  kubectl -n "${ns}" port-forward "svc/${svc}" "${lport}:${rport}" \
    >"${OPERATOR_DIR}/bin/.pf-${name}.log" 2>&1 &
  echo "$!" >> "${PID_FILE}"
}

say "Starting UI port-forwards"
forward flokoa "${NAMESPACE}"     flokoa-server  "${FLOKOA_UI_PORT}" 8080
forward argo   argo               argo-server    "${ARGO_UI_PORT}"   2746
forward yakd   yakd-dashboard     yakd-dashboard "${YAKD_UI_PORT}"   80
sleep 2

# --- 7. summary ------------------------------------------------------------
printf '\n'
ok "Flokoa is up. UIs (port-forwards running in the background):"
printf '   • Flokoa UI    \033[4mhttp://localhost:%s\033[0m\n'  "${FLOKOA_UI_PORT}"
printf '   • Argo UI      \033[4mhttps://localhost:%s\033[0m  (self-signed cert)\n' "${ARGO_UI_PORT}"
printf '   • yakd (k8s)   \033[4mhttp://localhost:%s\033[0m\n'  "${YAKD_UI_PORT}"
printf '\n'
printf '   Pods:   kubectl -n %s get pods\n' "${NAMESPACE}"
printf '   Stop:   make local-down            (tears down forwards + workloads)\n'
printf '   Forwards only:  make local-down ARGS=--forwards-only\n\n'
