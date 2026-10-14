#!/usr/bin/env bash
# Shared helpers for Shorty scripts. Source, don't execute.
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-shorty}"
KUBE_CONTEXT="${KUBE_CONTEXT:-k3d-${CLUSTER_NAME}}"
REGISTRY_HOST="${REGISTRY_HOST:-127.0.0.1:5001}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log()  { printf '\033[1m==> %s\033[0m\n' "$*"; }
warn() { printf '\033[1m!!  %s\033[0m\n' "$*" >&2; }
die()  { warn "$*"; exit 1; }

need() {
  local missing=""
  for bin in "$@"; do command -v "$bin" >/dev/null 2>&1 || missing="$missing $bin"; done
  [[ -z "$missing" ]] || die "missing tools:$missing (run 'make bootstrap')"
}

kc() { kubectl --context "$KUBE_CONTEXT" "$@"; }

# Capture first: `k3d ... | grep -q` + pipefail reports false when grep exits early (SIGPIPE).
cluster_exists() { local out; out="$(k3d cluster list -o json 2>/dev/null || true)"; [[ "$out" == *"\"name\":\"${CLUSTER_NAME}\""* ]]; }
