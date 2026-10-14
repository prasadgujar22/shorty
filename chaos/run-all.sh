#!/usr/bin/env bash
# Every scenario in sequence (≈ 3.5 min each), with a cool-down between runs
# so each starts from a healthy, settled cluster.
source "$(dirname "$0")/lib.sh"
for s in ${SCENARIOS:-baseline kill-pods kill-pods-force drain-node kill-redis kill-nats kill-postgres-primary}; do
  "$(dirname "$0")/run.sh" "$s"
  say "cool-down: waiting for all pods in $NS to be ready"
  k wait --for=condition=Ready pods --all --timeout=300s >/dev/null 2>&1 || true
  sleep "${COOLDOWN:-30}"
done
