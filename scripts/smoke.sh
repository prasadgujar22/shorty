#!/usr/bin/env bash
# End-to-end smoke test through the gateway: UI, create, redirect, stats.
source "$(dirname "$0")/lib.sh"
need curl jq
base="${BASE_URL:-http://localhost:8088}"
pass() { printf '  \033[1mPASS\033[0m %s\n' "$*"; }
fail() { printf '  \033[1mFAIL\033[0m %s\n' "$*"; exit 1; }

log "smoke test against $base"
code=$(curl -s -o /dev/null -w '%{http_code}' "$base/")
[[ "$code" == 200 ]] && pass "GET / -> 200 (web)" || fail "GET / -> $code"

slug="smoke-$(date +%s)"
created=""
for _ in 1 2 3 4 5; do   # tolerate a gateway/endpoint warm-up right after a deploy
  created=$(curl -sf -XPOST "$base/api/links" -H 'Content-Type: application/json' \
    -d "{\"url\":\"https://example.com/smoke?ts=$(date +%s)\",\"slug\":\"$slug\"}") && break
  sleep 2
done
[[ -n "$created" ]] || fail "POST /api/links"
pass "POST /api/links -> $(jq -r .short_url <<<"$created")"

loc=$(curl -s -o /dev/null -w '%{http_code} %{redirect_url}' -H 'Referer: https://smoke.test/' "$base/$slug")
[[ "$loc" == "302 https://example.com/smoke"* ]] && pass "GET /$slug -> $loc" || fail "redirect: $loc"
for _ in 1 2; do curl -s -o /dev/null "$base/$slug"; done

code=$(curl -s -o /dev/null -w '%{http_code}' "$base/does-not-exist-$$")
[[ "$code" == 404 ]] && pass "GET /<unknown> -> 404" || fail "unknown slug -> $code"

code=$(curl -s -o /dev/null -w '%{http_code}' "$base/s/$slug")
[[ "$code" == 200 ]] && pass "GET /s/$slug -> 200 (SPA route)" || fail "SPA route -> $code"

for i in $(seq 1 30); do
  total=$(curl -sf "$base/api/stats/$slug" 2>/dev/null | jq -r .total 2>/dev/null || true)
  [[ "$total" == 3 ]] && break
  sleep 1
done
[[ "$total" == 3 ]] && pass "GET /api/stats/$slug -> total=3 (click pipeline, ${i}s)" || fail "stats total=$total, want 3"
curl -sf "$base/api/stats/$slug" | jq -c '{total, referrers}'
log "smoke OK"
