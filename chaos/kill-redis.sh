#!/usr/bin/env bash
# SIGKILL one Redis pod (ALL=1: every Redis pod). Redis is a cache: expect a
# latency bump and more links-svc lookups, never errors or lost clicks.
source "$(dirname "$0")/lib.sh"
if [[ "${ALL:-0}" == 1 ]]; then
  pods=$(k get pods -l app.kubernetes.io/name=redis -o name)
else
  pods=$(random_pod redis)
fi
for p in $pods; do say "SIGKILL $p"; k delete "$p" --grace-period=0 --force --wait=false >/dev/null 2>&1; done
