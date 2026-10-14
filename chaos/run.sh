#!/usr/bin/env bash
# Run one chaos scenario under k6 load and record the result.
#   chaos/run.sh <scenario>        (baseline | kill-pods | kill-pods-force | drain-node |
#                                   kill-redis | kill-nats | kill-postgres-primary)
# Env: NS (shorty-ha), RATE (600 rps), DURATION (150s), FAULT_AT (45s after load
# starts), LINKS (200), RESULTS (docs/RESULTS.md), NOTE (free text for the table).
source "$(dirname "$0")/lib.sh"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
scenario="${1:?scenario}"
script="$ROOT/chaos/$scenario.sh"
[[ -x "$script" ]] || { echo "unknown scenario: $scenario" >&2; exit 2; }
RATE="${RATE:-600}"; DURATION="${DURATION:-150s}"; FAULT_AT="${FAULT_AT:-45}"; LINKS="${LINKS:-200}"
BASE_URL="${BASE_URL:-http://localhost:8088}"
case "$NS" in shorty-ha) HOST="${HOST-ha.localhost}" ;; shorty-dev) HOST="${HOST-dev.localhost}" ;; *) HOST="${HOST-}" ;; esac
RESULTS="${RESULTS:-$ROOT/docs/RESULTS.md}"
run_id="$(echo "$scenario" | tr -cd 'a-z-' | cut -c1-18)-$(date +%H%M%S)"
csv="$ROOT/load/results/$run_id.csv"; summary="$ROOT/load/results/$run_id.json"
mkdir -p "$ROOT/load/results"

say "run $run_id: $scenario on $NS at $RATE rps for $DURATION (fault at +${FAULT_AT}s)"
k6 run -q --out "csv=$csv" -e BASE_URL="$BASE_URL" -e HOST="$HOST" -e RATE="$RATE" -e DURATION="$DURATION" \
  -e LINKS="$LINKS" -e RUN_ID="$run_id" -e SUMMARY_FILE="$summary" "$ROOT/load/redirect.js" >"$ROOT/load/results/$run_id.log" 2>&1 &
k6pid=$!

# Start the fault clock when the first redirect sample appears (after setup()).
until grep -q ',redirect,' "$csv" 2>/dev/null; do kill -0 $k6pid 2>/dev/null || { cat "$ROOT/load/results/$run_id.log"; exit 1; }; sleep 0.5; done
sleep "$FAULT_AT"
fault_at=$(date +%s)
"$script" 2>&1 | sed 's/^/    /' &
faultpid=$!
wait $k6pid || true
wait $faultpid || true
say "load finished: $(tail -1 "$ROOT/load/results/$run_id.log")"

# Wait for the click pipeline to drain, then reconcile k6's 302s with analytics.
counted() { k exec "$(pg_primary)" -c postgres -- psql -d shorty -tAc \
  "SELECT coalesce(sum(clicks),0) FROM analytics.link_totals WHERE slug LIKE '${run_id}-%'" 2>/dev/null | tr -d '[:space:]'; }
want=$(jq -r .redirects_302 "$summary")
prev=-1; stable=0; got=0
for _ in $(seq 1 60); do
  got=$(counted || echo 0); got=${got:-0}
  if [[ "$got" == "$want" ]]; then break; fi
  if [[ "$got" == "$prev" ]]; then stable=$((stable + 1)); else stable=0; fi
  [[ $stable -ge 6 ]] && break     # unchanged for 30s: pipeline drained
  prev=$got; sleep 5
done
lost=$((want - got))
a=$(python3 "$ROOT/chaos/analyze.py" "$csv" "$fault_at")
say "clicks: k6 saw $want redirects, analytics counted $got (lost: $lost)"
say "analysis: $a"

s() { jq -r "$1" "$summary"; }
x() { jq -r "$1" <<<"$a"; }
f2() { printf '%.2f' "$1"; }
if [[ ! -f "$RESULTS" ]] || ! grep -q '^| Date' "$RESULTS"; then
  echo "results table missing in $RESULTS" >&2
else
  row="| $(date '+%Y-%m-%d %H:%M') | \`$scenario\` | $NS | $(s .rps) | $(f2 "$(s .p50_ms)") | $(f2 "$(s .p95_ms)") | $(f2 "$(s .p99_ms)") | $(x .fault_window_p99_ms) | $(x .worst_second_p99_ms) | $(printf '%.3f' "$(echo "$(s .unexpected_rate) * 100" | bc -l)") | $(x .error_window_s)s | $(x '.latency_recovery_s // "-"')s | $want | $got | $lost | ${NOTE:-} |"
  # Insert before the end marker so prose below the table is preserved.
  python3 - "$RESULTS" "$row" <<'PY'
import sys
path, row = sys.argv[1], sys.argv[2]
s = open(path).read()
marker = "<!-- results:end -->"
s = s.replace(marker, row + "\n" + marker) if marker in s else s + row + "\n"
open(path, "w").write(s)
PY
  say "appended to $RESULTS"
fi
rm -f "$csv"   # large; the summary JSON and log are kept (gitignored)
