#!/usr/bin/env bash
# Install the trimmed observability stack into `monitoring`:
# kube-prometheus-stack, Loki (single binary), Alloy, prometheus-adapter,
# plus the Shorty Grafana dashboards and alert rules.
source "$(dirname "$0")/lib.sh"
need helm kubectl
# shellcheck disable=SC1091
source "$ROOT_DIR/deploy/platform/versions.env"
P="$ROOT_DIR/deploy/platform"
H=(helm --kube-context "$KUBE_CONTEXT" upgrade --install -n monitoring --create-namespace --wait --timeout 10m --hide-notes)

kc get ns monitoring >/dev/null 2>&1 || kc create ns monitoring >/dev/null
if ! kc -n monitoring get secret grafana-admin >/dev/null 2>&1; then
  kc -n monitoring create secret generic grafana-admin \
    --from-literal=admin-user=admin --from-literal=admin-password="$(openssl rand -hex 12)" >/dev/null
  log "created monitoring/grafana-admin"
fi

log "kube-prometheus-stack ${KPS_CHART}"
argo_managed monitoring && log "monitoring: managed by Argo CD, skipping" || "${H[@]}" kps prometheus-community/kube-prometheus-stack --version "$KPS_CHART" -f "$P/monitoring/values.yaml"
log "loki ${LOKI_CHART}"
argo_managed loki && log "loki: managed by Argo CD, skipping" || "${H[@]}" loki grafana/loki --version "$LOKI_CHART" -f "$P/loki/values.yaml"
log "alloy ${ALLOY_CHART}"
argo_managed alloy && log "alloy: managed by Argo CD, skipping" || "${H[@]}" alloy grafana/alloy --version "$ALLOY_CHART" -f "$P/alloy/values.yaml"
log "prometheus-adapter ${ADAPTER_CHART}"
argo_managed prometheus-adapter && log "prometheus-adapter: managed by Argo CD, skipping" || "${H[@]}" prometheus-adapter prometheus-community/prometheus-adapter --version "$ADAPTER_CHART" -f "$P/prometheus-adapter/values.yaml"

log "dashboards + alert rules"
kc apply -k "$ROOT_DIR/deploy/dashboards" >/dev/null
kc -n monitoring get pods
log "Grafana: http://grafana.localhost:8088 (anonymous viewer; admin password: kubectl -n monitoring get secret grafana-admin -o jsonpath='{.data.admin-password}' | base64 -d)"
