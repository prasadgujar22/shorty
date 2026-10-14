#!/usr/bin/env bash
# Install Argo CD and hand the cluster to Git (app-of-apps).
# Pull-based: the cluster polls GitHub and GHCR over outbound HTTPS; nothing
# needs to reach the laptop. See docs/GITOPS.md.
source "$(dirname "$0")/lib.sh"
need helm kubectl
# shellcheck disable=SC1091
source "$ROOT_DIR/deploy/platform/versions.env"

log "argo-cd ${ARGOCD_CHART}"
helm repo add argo https://argoproj.github.io/argo-helm >/dev/null 2>&1 || true
helm repo update argo >/dev/null
helm --kube-context "$KUBE_CONTEXT" upgrade --install argocd argo/argo-cd --version "$ARGOCD_CHART" \
  -n argocd --create-namespace -f "$ROOT_DIR/deploy/argocd/bootstrap/argocd-values.yaml" \
  --wait --timeout 10m --hide-notes

# Secrets are not in Git (see ADR 0005): create them per GitOps namespace.
for ns in shorty-dev shorty-ha; do "$ROOT_DIR/scripts/secrets.sh" "$ns"; done

log "applying app-of-apps (deploy/argocd/root.yaml)"
kc apply -f "$ROOT_DIR/deploy/argocd/root.yaml"

log "waiting for shorty-dev to become Synced/Healthy (first sync pulls images from GHCR)"
for _ in $(seq 1 90); do
  s=$(kc -n argocd get application shorty-dev -o jsonpath='{.status.sync.status}/{.status.health.status}' 2>/dev/null || true)
  [[ "$s" == "Synced/Healthy" ]] && break
  sleep 5
done
kc -n argocd get applications
pw=$(kc -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' 2>/dev/null | base64 -d || true)
log "Argo CD UI: http://argocd.localhost:8088  (user: admin, password: ${pw:-<rotated>})"
