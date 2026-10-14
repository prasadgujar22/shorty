#!/usr/bin/env bash
# Drain the agent running the most Shorty pods (excluding the node that hosts
# the Postgres primary: CNPG's primary PDB blocks eviction by design, see
# docs/RESULTS.md), hold it drained for HOLD seconds, then uncordon.
source "$(dirname "$0")/lib.sh"
HOLD="${HOLD:-60}"
primary_node=$(k get pod "$(pg_primary)" -o jsonpath='{.spec.nodeName}')
node=$(k get pods -o jsonpath='{range .items[*]}{.spec.nodeName}{"\n"}{end}' | grep -v "^${primary_node}$" | grep agent | sort | uniq -c | sort -rn | awk 'NR==1{print $2}')
[[ -n "$node" ]] || { say "no candidate node"; exit 1; }
say "draining $node (pg primary is on $primary_node): $(k get pods -o wide --no-headers | awk -v n="$node" '$7==n{print $1}' | tr '\n' ' ')"
kg drain "$node" --ignore-daemonsets --delete-emptydir-data --timeout=180s >/dev/null 2>&1 || say "drain did not complete within 180s"
say "drained $node; holding ${HOLD}s"
sleep "$HOLD"
kg uncordon "$node" >/dev/null
say "uncordoned $node"
