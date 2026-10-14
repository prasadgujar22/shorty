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

# Fail fast instead of leaving pods in ImagePullBackOff for 10 minutes.
registry="${REGISTRY:-$REGISTRY_HOST}"
for svc in links-svc redirect-svc analytics-svc web; do
  code=$(curl -s -o /dev/null -w '%{http_code}' -H 'Accept: application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json' \
    "http://${registry}/v2/shorty-${svc}/manifests/${tag}")
  [[ "$code" == 200 ]] || die "image ${registry}/shorty-${svc}:${tag} not found (HTTP $code): run 'make build' or 'make up', or pass TAG=<built tag>"
done

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

# Helm's --wait can return before readiness gates settle on a fresh install
# (Postgres initdb takes ~60s); wait explicitly for the data tier and apps.
log "waiting for Postgres and service rollouts"
kc -n "$ns" wait cluster.postgresql.cnpg.io/shorty-pg --for=condition=Ready --timeout=300s >/dev/null
for d in links-svc redirect-svc analytics-svc web redis; do
  kc -n "$ns" rollout status "deploy/$d" --timeout=300s >/dev/null
done
kc -n "$ns" get pods -o wide
