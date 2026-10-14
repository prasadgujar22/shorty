#!/usr/bin/env bash
# k6 load test of the redirect path. Env: BASE_URL, HOST (Host header,
# e.g. ha.localhost), RATE (rps), DURATION, LINKS, HOT_SHARE, MISS_SHARE.
#   make load                                   # local env, 500 rps for 2m
#   make load HOST=ha.localhost RATE=1000
source "$(dirname "$0")/lib.sh"
need k6
mkdir -p "$ROOT_DIR/load/results"
exec k6 run -e BASE_URL="${BASE_URL:-http://localhost:8088}" -e HOST="${HOST:-}" \
  -e RATE="${RATE:-500}" -e DURATION="${DURATION:-2m}" -e LINKS="${LINKS:-100}" \
  -e HOT_SHARE="${HOT_SHARE:-0.2}" -e MISS_SHARE="${MISS_SHARE:-0}" \
  "$ROOT_DIR/load/redirect.js"
