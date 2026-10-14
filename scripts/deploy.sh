#!/usr/bin/env bash
# Deploy the umbrella chart with locally built images (make build) into
# $NAMESPACE using deploy/envs/$ENV/values.yaml. This is the inner dev loop;
# GitOps (Argo CD) uses the same chart and values with GHCR images.
source "$(dirname "$0")/lib.sh"
need helm kubectl
env="${ENV:-dev}"
ns="${NAMESPACE:-shorty}"
tag="${TAG:-dev}"
values="$ROOT_DIR/deploy/envs/$env/values.yaml"
[[ -f "$values" ]] || die "no such env: $env"

"$ROOT_DIR/scripts/secrets.sh" "$ns"

log "helm dependency build"
helm dependency build "$ROOT_DIR/deploy/charts/shorty" >/dev/null

log "deploying env=$env tag=$tag to namespace $ns"
helm --kube-context "$KUBE_CONTEXT" upgrade --install shorty "$ROOT_DIR/deploy/charts/shorty" \
  -n "$ns" --create-namespace \
  -f "$values" \
  --set global.imageRegistry="${REGISTRY:-$REGISTRY_HOST}" \
  --set links-svc.image.tag="$tag" \
  --set redirect-svc.image.tag="$tag" \
  --set analytics-svc.image.tag="$tag" \
  --set web.image.tag="$tag" \
  --set gateway.host="${HOST:-}" \
  --set links-svc.env.PUBLIC_BASE_URL="${BASE_URL:-http://localhost:8088}" \
  --wait --timeout 10m --hide-notes

kc -n "$ns" get pods -o wide
