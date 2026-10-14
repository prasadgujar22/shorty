#!/usr/bin/env bash
# Create per-service Postgres credentials in a namespace, if missing.
# Secrets never live in Git: in a real cluster this is External Secrets /
# Sealed Secrets territory (see docs/adr/0005-secrets-out-of-git.md).
#   scripts/secrets.sh <namespace>
source "$(dirname "$0")/lib.sh"
ns="${1:-$NAMESPACE}"
kc get ns "$ns" >/dev/null 2>&1 || kc create ns "$ns" >/dev/null

for role in links analytics; do
  name="shorty-db-${role}"
  if kc -n "$ns" get secret "$name" >/dev/null 2>&1; then
    continue
  fi
  pw="$(openssl rand -hex 24)"
  kc -n "$ns" create secret generic "$name" --type=kubernetes.io/basic-auth \
    --from-literal=username="$role" --from-literal=password="$pw" >/dev/null
  kc -n "$ns" label secret "$name" cnpg.io/reload=true app.kubernetes.io/part-of=shorty >/dev/null
  log "created secret $ns/$name"
done
