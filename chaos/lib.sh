#!/usr/bin/env bash
# Shared helpers for chaos scripts. Fault injection uses kubectl only.
set -euo pipefail
NS="${NS:-shorty-ha}"
CTX="${KUBE_CONTEXT:-k3d-shorty}"
k() { kubectl --context "$CTX" -n "$NS" "$@"; }
kg() { kubectl --context "$CTX" "$@"; }
say() { printf '\033[1m[chaos %s] %s\033[0m\n' "$(date +%T)" "$*"; }
STATELESS="links-svc redirect-svc analytics-svc web"

random_pod() { # random_pod <app-name>
  k get pods -l "app.kubernetes.io/name=$1" --field-selector=status.phase=Running -o name | awk 'BEGIN{srand()} {a[NR]=$0} END{if (NR) print a[int(rand()*NR)+1]}'
}

pg_primary() { k get pods -l cnpg.io/cluster=shorty-pg,cnpg.io/instanceRole=primary -o jsonpath='{.items[0].metadata.name}'; }
