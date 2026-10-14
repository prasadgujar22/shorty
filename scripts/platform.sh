#!/usr/bin/env bash
# Install the cluster platform with Helm: Traefik (gateway), CloudNativePG
# operator, and (PLATFORM_OBSERVABILITY=1, the default) the trimmed
# observability stack. Idempotent. Argo CD later adopts these releases.
source "$(dirname "$0")/lib.sh"
need helm kubectl
# shellcheck disable=SC1091
source "$ROOT_DIR/deploy/platform/versions.env"
P="$ROOT_DIR/deploy/platform"
H=(helm --kube-context "$KUBE_CONTEXT" upgrade --install --wait --timeout 10m --hide-notes)

log "helm repos"
helm repo add traefik https://traefik.github.io/charts >/dev/null 2>&1 || true
helm repo add cnpg https://cloudnative-pg.github.io/charts >/dev/null 2>&1 || true
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
helm repo add grafana https://grafana.github.io/helm-charts >/dev/null 2>&1 || true
helm repo update traefik cnpg prometheus-community grafana >/dev/null

if [[ "${PLATFORM_OBSERVABILITY:-1}" == 1 && -x "$ROOT_DIR/scripts/observability.sh" ]]; then
  # Installed first so the ServiceMonitor CRDs exist for everything else.
  "$ROOT_DIR/scripts/observability.sh"
fi

has_monitoring=false
kc get crd servicemonitors.monitoring.coreos.com >/dev/null 2>&1 && has_monitoring=true

if argo_managed traefik; then log "traefik: managed by Argo CD (platform-traefik), skipping"; else
log "traefik ${TRAEFIK_CHART}"
"${H[@]}" traefik traefik/traefik --version "$TRAEFIK_CHART" -n traefik --create-namespace \
  -f "$P/traefik/values.yaml" --set metrics.prometheus.serviceMonitor.enabled="$has_monitoring"
fi

if argo_managed cnpg; then log "cnpg: managed by Argo CD (platform-cnpg), skipping"; else
log "cloudnative-pg ${CNPG_CHART}"
"${H[@]}" cnpg cnpg/cloudnative-pg --version "$CNPG_CHART" -n cnpg-system --create-namespace \
  -f "$P/cnpg/values.yaml" --set monitoring.podMonitorEnabled="$has_monitoring"
fi

kc -n traefik get pods -o wide
kc -n cnpg-system get pods
log "platform ready"
