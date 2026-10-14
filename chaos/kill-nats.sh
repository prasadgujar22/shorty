#!/usr/bin/env bash
# SIGKILL the NATS pod that leads the CLICKS stream (Raft leader), forcing a
# JetStream leader election while redirect-svc is publishing.
source "$(dirname "$0")/lib.sh"
leader=$(k exec nats-0 -c nats -- wget -qO- 'http://127.0.0.1:8222/jsz?streams=true' 2>/dev/null \
  | tr ',' '\n' | grep -o '"leader":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
target="${leader:-nats-0}"
say "CLICKS stream leader: ${leader:-unknown (single node?)}; SIGKILL $target"
k delete pod "$target" --grace-period=0 --force --wait=false >/dev/null 2>&1
