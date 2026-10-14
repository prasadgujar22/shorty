#!/usr/bin/env bash
# Tuning harness: run one experiment against the local `shorty` namespace
# (Helm-managed, not Argo) with Helm overrides, then record k6 numbers plus
# Prometheus measurements for the load window as one JSON line.
#
#   load/tune.sh <name> <k6-script> "<k6 -e args>" [--set k=v ...]
#   e.g. load/tune.sh gomaxprocs-1 redirect.js "-e RATE=1500" --set redirect-svc.env.GOMAXPROCS=1
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
name="$1"; script="$2"; k6args="$3"; shift 3
NS=shorty; CTX=k3d-shorty
TAG="${TAG:-$(kubectl --context $CTX -n $NS get deploy redirect-svc -o jsonpath='{.spec.template.spec.containers[0].image}' | sed 's/.*://')}"
out="$ROOT/load/results/tuning.jsonl"
prom() { curl -s -H 'Host: prometheus.localhost' 'http://localhost:8088/api/v1/query' --data-urlencode "query=$1" | jq -r '.data.result[0].value[1] // "0"'; }

echo "== $name: overrides: $*"
helm --kube-context $CTX upgrade shorty "$ROOT/deploy/charts/shorty" -n $NS \
  -f "$ROOT/deploy/envs/dev/values.yaml" --set global.imageRegistry=127.0.0.1:5001 \
  --set links-svc.image.tag="$TAG" --set redirect-svc.image.tag="$TAG" --set analytics-svc.image.tag="$TAG" --set web.image.tag="$TAG" \
  --set gateway.host= --set links-svc.env.PUBLIC_BASE_URL=http://localhost:8088 \
  "$@" --wait --timeout 5m --hide-notes >/dev/null
kubectl --context $CTX -n $NS wait --for=condition=Ready pods --all --timeout=180s >/dev/null
sleep "${SETTLE:-20}"   # let terminating pods and the HPA/metrics settle

run_id="tune-$(echo "$name" | tr -cd 'a-z0-9-' | cut -c1-16)-$(date +%M%S)"
summary="$ROOT/load/results/$run_id.json"
start=$(date +%s)
# shellcheck disable=SC2086
k6 run -q -e RUN_ID="$run_id" -e SUMMARY_FILE="$summary" $k6args "$ROOT/load/$script" >/dev/null 2>&1 || true
w="$(( $(date +%s) - start ))s"

m=$(jq -n \
  --arg throttle "$(prom "sum(increase(container_cpu_cfs_throttled_periods_total{namespace=\"$NS\",container=\"redirect-svc\"}[$w])) / clamp_min(sum(increase(container_cpu_cfs_periods_total{namespace=\"$NS\",container=\"redirect-svc\"}[$w])),1)")" \
  --arg redirect_cpu "$(prom "max(rate(container_cpu_usage_seconds_total{namespace=\"$NS\",container=\"redirect-svc\"}[$w]))")" \
  --arg redirect_mem_mib "$(prom "max(max_over_time(container_memory_working_set_bytes{namespace=\"$NS\",container=\"redirect-svc\"}[$w])) / 1048576")" \
  --arg gc_per_s "$(prom "sum(rate(go_gc_duration_seconds_count{namespace=\"$NS\",job=\"redirect-svc\"}[$w]))")" \
  --arg gomaxprocs "$(prom "max(go_sched_gomaxprocs_threads{namespace=\"$NS\",job=\"redirect-svc\"})")" \
  --arg cache_hit "$(prom "sum(increase(shorty_redirect_cache_requests_total{namespace=\"$NS\",result=~\"hit|negative_hit\"}[$w])) / clamp_min(sum(increase(shorty_redirect_cache_requests_total{namespace=\"$NS\"}[$w])),1)")" \
  --arg links_rps "$(prom "sum(rate(shorty_http_requests_total{namespace=\"$NS\",job=\"links-svc\"}[$w]))")" \
  --arg pool_empty_per_s "$(prom "sum(rate(shorty_db_pool_empty_acquires_total{namespace=\"$NS\",job=\"links-svc\"}[$w]))")" \
  --arg pool_wait_ms "$(prom "1000 * sum(rate(shorty_db_pool_acquire_seconds_total{namespace=\"$NS\",job=\"links-svc\"}[$w])) / clamp_min(sum(rate(shorty_db_pool_acquires_total{namespace=\"$NS\",job=\"links-svc\"}[$w])),1e-9)")" \
  --arg buffer_max "$(prom "max(max_over_time(shorty_clicks_buffer_depth{namespace=\"$NS\"}[$w]))")" \
  --arg publish_p99_ms "$(prom "1000 * histogram_quantile(0.99, sum by (le) (rate(shorty_clicks_publish_seconds_bucket{namespace=\"$NS\"}[$w])))")" \
  --arg consumer_lag_max "$(prom "max(max_over_time(shorty_analytics_consumer_pending{namespace=\"$NS\"}[$w]))")" \
  --arg freshness_max_s "$(prom "max(max_over_time(shorty_analytics_last_event_age_seconds{namespace=\"$NS\"}[$w]))")" \
  --arg analytics_eps "$(prom "sum(rate(shorty_analytics_events_total{namespace=\"$NS\",result=\"processed\"}[$w]))")" \
  --arg restarts "$(prom "sum(increase(kube_pod_container_status_restarts_total{namespace=\"$NS\"}[$w]))")" \
  --arg oomkilled "$(prom "sum(kube_pod_container_status_last_terminated_reason{namespace=\"$NS\",reason=\"OOMKilled\"})")" \
  '$ARGS.named | map_values(tonumber? // .)')
line=$(jq -c -n --arg name "$name" --arg overrides "$*" --argjson k6 "$(cat "$summary")" --argjson prom "$m" '{name: $name, overrides: $overrides, k6: $k6, prom: $prom}')
echo "$line" | tee -a "$out" | jq -c '{name, rps: .k6.rps, p50: .k6.p50_ms, p99: .k6.p99_ms, err: (.k6.unexpected_rate // .k6.failed_rate)} + .prom' 
