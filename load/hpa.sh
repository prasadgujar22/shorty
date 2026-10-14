#!/usr/bin/env bash
# HPA experiment on the ha env: ramp 300 -> 2400 rps (in-cluster k6) and
# sample the redirect-svc HPA every 5s.
#   load/hpa.sh <label>          # uses the HPA as currently configured
# Prints a timeline and a JSON summary line (appended to load/results/hpa.jsonl).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NS=shorty-ha; CTX=k3d-shorty; label="${1:-current}"
RAMP="${RAMP:-300:30s,2400:60s,2400:120s,300:30s}"
k() { kubectl --context $CTX -n $NS "$@"; }

echo "== hpa run '$label': metrics = $(k get hpa redirect-svc -o jsonpath='{range .spec.metrics[*]}{.type}:{.resource.name}{.pods.metric.name} {end}')"
until [ "$(k get hpa redirect-svc -o jsonpath='{.status.currentReplicas}')" = 3 ]; do sleep 10; done

k create configmap k6-script --from-file="$ROOT/load/redirect.js" --dry-run=client -o yaml | kubectl --context $CTX apply -f - >/dev/null
k delete job k6 --ignore-not-found --wait=true >/dev/null
run_id="hpa-$(echo "$label" | tr -cd 'a-z0-9-' | cut -c1-12)-$(date +%M%S)"
sed -e 's|"-q", "/scripts/redirect.js"|"-q", "--no-thresholds", "/scripts/redirect.js"|' "$ROOT/load/k8s/k6-job.yaml" \
  | yq "(.spec.template.spec.containers[0].env) = [{\"name\":\"RAMP\",\"value\":\"$RAMP\"},{\"name\":\"RUN_ID\",\"value\":\"$run_id\"},{\"name\":\"BASE_URL\",\"value\":\"http://traefik.traefik.svc.cluster.local\"},{\"name\":\"HOST\",\"value\":\"ha.localhost\"},{\"name\":\"LINKS\",\"value\":\"200\"},{\"name\":\"SUMMARY_FILE\",\"value\":\"/tmp/s.json\"}]" \
  | k apply -f - >/dev/null

# Wait for the load to start (setup creates links first).
until k logs job/k6 2>/dev/null | grep -q . || [ "$(k get pods -l job-name=k6 -o jsonpath='{.items[0].status.phase}')" = Running ]; do sleep 1; done
t0=$(date +%s); first_up=""; max_rep=3; timeline=""
while :; do
  st=$(k get job k6 -o jsonpath='{.status.succeeded}{.status.failed}')
  t=$(( $(date +%s) - t0 ))
  cur=$(k get hpa redirect-svc -o jsonpath='{.status.currentReplicas}')
  des=$(k get hpa redirect-svc -o jsonpath='{.status.desiredReplicas}')
  m=$(k get hpa redirect-svc -o jsonpath='{range .status.currentMetrics[*]}{.resource.current.averageUtilization}{.pods.current.averageValue} {end}')
  ready=$(k get deploy redirect-svc -o jsonpath='{.status.readyReplicas}')
  timeline="$timeline\n  t=${t}s desired=$des current=$cur ready=$ready metrics(cpu%,rps/pod)=[$m]"
  [[ -z "$first_up" && "$des" -gt 3 ]] && first_up=$t
  (( ready > max_rep )) && max_rep=$ready
  [[ -n "$st" ]] && break
  sleep 5
done
printf "%b\n" "$timeline"
summary=$(k logs job/k6 | sed -n 's/^SUMMARY //p' | tail -1)
line=$(jq -c -n --arg label "$label" --argjson k6 "$summary" --arg first_up "${first_up:-none}" --arg max_ready "$max_rep" \
  '{label: $label, first_scale_up_s: $first_up, max_ready: ($max_ready|tonumber), p50_ms: $k6.p50_ms, p95_ms: $k6.p95_ms, p99_ms: $k6.p99_ms, max_ms: $k6.max_ms, dropped: $k6.dropped_iterations, requests: $k6.requests}')
echo "$line" | tee -a "$ROOT/load/results/hpa.jsonl"
