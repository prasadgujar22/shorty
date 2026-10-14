#!/usr/bin/env python3
"""Server-side view of chaos runs, from Prometheus.

k6 runs on the macOS host, so its numbers include Docker Desktop's port
proxy and the k3d load balancer. This adds what the *service* saw:

  svc p99 (fault): worst 30s-window p99 of redirect-svc's own request
                   histogram during the 60s after the fault
  svc 5xx:         5xx responses served by redirect-svc during the run
  gw 5xx:          5xx returned by Traefik for shorty-ha backends

usage:
  serverside.py window <start_epoch> <fault_epoch> <end_epoch>   -> JSON
  serverside.py backfill <RESULTS.md> <runner logs...>            -> rewrites table
"""
import json
import re
import subprocess
import sys
import urllib.parse
from datetime import datetime

PROM = "http://localhost:8088/api/v1/"
NS = "shorty-ha"


def prom(path, **params):
    url = PROM + path + "?" + urllib.parse.urlencode(params)
    out = subprocess.run(["curl", "-s", "-H", "Host: prometheus.localhost", url], capture_output=True, text=True, check=True).stdout
    return json.loads(out)["data"]["result"]


def window(start, fault, end):
    p99 = prom("query_range",
               query=f'histogram_quantile(0.99, sum by (le) (rate(shorty_http_request_duration_seconds_bucket{{namespace="{NS}",job="redirect-svc"}}[30s])))',
               start=fault, end=fault + 60, step=5)
    vals = [float(v) for r in p99 for _, v in r["values"] if v not in ("NaN", "+Inf")]
    dur = max(30, end - start)
    svc5 = prom("query", time=end, query=f'sum(increase(shorty_http_requests_total{{namespace="{NS}",job="redirect-svc",code=~"5.."}}[{dur}s])) or vector(0)')
    gw5 = prom("query", time=end, query=f'sum(increase(traefik_service_requests_total{{exported_service=~"{NS}-.*",code=~"5.."}}[{dur}s])) or vector(0)')
    return {
        "svc_p99_fault_ms": round(max(vals) * 1000, 2) if vals else None,
        "svc_5xx": round(float(svc5[0]["value"][1])) if svc5 else 0,
        "gw_5xx": round(float(gw5[0]["value"][1])) if gw5 else 0,
    }


def parse_logs(paths, day):
    """Yield (scenario, start, fault, end) in chronological order."""
    runs = []
    ts = lambda hms: int(datetime.strptime(f"{day} {hms}", "%Y-%m-%d %H:%M:%S").timestamp())
    for path in paths:
        cur = None
        for line in open(path, errors="replace"):
            line = re.sub(r"\x1b\[[0-9;]*m", "", line)
            m = re.search(r"\[chaos (\d\d:\d\d:\d\d)\] run \S+: (\S+) on", line)
            if m:
                cur = {"scenario": m.group(2), "start": ts(m.group(1))}
                runs.append(cur)
                continue
            if cur is None:
                continue
            m = re.match(r"\s+\[chaos (\d\d:\d\d:\d\d)\]", line)
            if m and "fault" not in cur:
                cur["fault"] = ts(m.group(1))
            m = re.search(r"\[chaos (\d\d:\d\d:\d\d)\] load finished", line)
            if m:
                cur["end"] = ts(m.group(1))
    return [r for r in runs if {"fault", "end"} <= r.keys()]


def backfill(results, logs):
    day = datetime.now().strftime("%Y-%m-%d")
    runs = parse_logs(logs, day)
    lines = open(results).read().split("\n")
    rows = [i for i, l in enumerate(lines) if l.startswith("| 20")]
    if len(rows) != len(runs):
        sys.exit(f"{len(rows)} table rows but {len(runs)} runs in logs")
    for i, run in zip(rows, runs):
        cells = lines[i].split("|")
        if run["scenario"] not in cells[2]:
            sys.exit(f"row/log mismatch: {cells[2]} vs {run['scenario']}")
        w = window(run["start"], run["fault"], run["end"])
        extra = f" {w['svc_p99_fault_ms']} | {w['svc_5xx']} / {w['gw_5xx']} "
        if len(cells) == 18:  # not yet backfilled: insert before Notes
            cells = cells[:-2] + [extra.split("|")[0], extra.split("|")[1]] + cells[-2:]
        lines[i] = "|".join(cells)
    for i, l in enumerate(lines):
        if l.startswith("| Date") and "svc p99" not in l:
            lines[i] = l.replace("| lost | Notes |", "| lost | svc p99 (fault) ms | svc / gw 5xx | Notes |")
        if l.startswith("|---") and l.count("|") == 18:
            lines[i] = l + "---|---|"
    open(results, "w").write("\n".join(lines))
    print(f"backfilled {len(runs)} rows")


if __name__ == "__main__":
    if sys.argv[1] == "window":
        print(json.dumps(window(*map(int, sys.argv[2:5]))))
    else:
        backfill(sys.argv[2], sys.argv[3:])
