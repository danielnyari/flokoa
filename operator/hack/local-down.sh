#!/usr/bin/env bash
# local-down.sh — tear down what local-up.sh started.
#
#   make local-down                       # stop forwards + undeploy workloads
#   make local-down ARGS=--forwards-only  # only kill the port-forwards
#   make local-down ARGS=--stop-minikube  # also `minikube stop` when done
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPERATOR_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PID_FILE="${OPERATOR_DIR}/bin/.local-portforward.pids"
MINIKUBE_PROFILE="${MINIKUBE_PROFILE:-minikube}"
cd "${OPERATOR_DIR}"

FORWARDS_ONLY=false
STOP_MINIKUBE=false
for arg in "$@"; do
  case "${arg}" in
    --forwards-only) FORWARDS_ONLY=true ;;
    --stop-minikube) STOP_MINIKUBE=true ;;
  esac
done

say()  { printf '\033[1;36m▸ %s\033[0m\n' "$*"; }
ok()   { printf '\033[1;32m✓ %s\033[0m\n' "$*"; }

# --- kill port-forwards ----------------------------------------------------
say "Stopping UI port-forwards"
if [[ -f "${PID_FILE}" ]]; then
  while read -r pid; do
    [[ -n "${pid}" ]] && kill "${pid}" 2>/dev/null || true
  done < "${PID_FILE}"
  rm -f "${PID_FILE}"
fi
# Belt-and-suspenders: any stray forwards for our services.
pkill -f "port-forward svc/flokoa-server" 2>/dev/null || true
pkill -f "port-forward svc/argo-server"   2>/dev/null || true
pkill -f "port-forward svc/yakd-dashboard" 2>/dev/null || true
ok "Port-forwards stopped"

if [[ "${FORWARDS_ONLY}" == "true" ]]; then
  exit 0
fi

# --- undeploy workloads ----------------------------------------------------
say "Undeploying operator + Argo + executor plugins"
make undeploy-full || true
ok "Workloads undeployed (CRDs left in place; run 'make uninstall' to remove them)"

if [[ "${STOP_MINIKUBE}" == "true" ]]; then
  say "Stopping minikube '${MINIKUBE_PROFILE}'"
  minikube -p "${MINIKUBE_PROFILE}" stop || true
  ok "minikube stopped"
fi
