#!/usr/bin/env bash
# Kill one random pod of each stateless service, ROUNDS times, INTERVAL apart.
# FORCE=1 sends SIGKILL (--grace-period=0 --force) to simulate a crash/OOM;
# otherwise a normal delete (SIGTERM, preStop, graceful drain).
source "$(dirname "$0")/lib.sh"
ROUNDS="${ROUNDS:-3}"; INTERVAL="${INTERVAL:-15}"
flags=(--wait=false)
[[ "${FORCE:-0}" == 1 ]] && flags=(--grace-period=0 --force --wait=false)
for r in $(seq 1 "$ROUNDS"); do
  for app in $STATELESS; do
    p=$(random_pod "$app"); [[ -n "$p" ]] || continue
    say "round $r: delete $p ${FORCE:+(SIGKILL)}"
    k delete "$p" "${flags[@]}" >/dev/null 2>&1
  done
  [[ $r -lt $ROUNDS ]] && sleep "$INTERVAL"
done
