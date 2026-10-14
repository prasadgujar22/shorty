#!/usr/bin/env python3
"""Summarise a k6 CSV run around a fault: per-second p99 and error windows.

usage: analyze.py <k6.csv> <fault_epoch|0>
Prints one JSON object (consumed by chaos/run.sh)."""
import csv
import json
import math
import sys
from collections import defaultdict


def pct(values, p):
    if not values:
        return 0.0
    v = sorted(values)
    k = max(0, min(len(v) - 1, math.ceil(p / 100 * len(v)) - 1))
    return v[k]


def main(path, fault):
    per_sec = defaultdict(list)
    errs = defaultdict(int)
    for row in csv.DictReader(open(path, newline="")):
        if row["metric_name"] != "http_req_duration" or row.get("name") != "redirect":
            continue
        t = int(float(row["timestamp"]))
        per_sec[t].append(float(row["metric_value"]))
        if row.get("status") not in ("302", "404"):
            errs[t] += 1
    if not per_sec:
        print(json.dumps({"error": "no samples"}))
        return
    start, end = min(per_sec), max(per_sec)
    out = {"seconds": end - start + 1, "samples": sum(len(v) for v in per_sec.values())}
    pre = [x for t, v in per_sec.items() if fault and t < fault for x in v]
    win = [x for t, v in per_sec.items() if fault and fault <= t < fault + 60 for x in v]
    out["pre_fault_p99_ms"] = round(pct(pre, 99), 2)
    out["fault_window_p99_ms"] = round(pct(win, 99), 2)
    out["worst_second_p99_ms"] = round(max(pct(v, 99) for v in per_sec.values()), 2)
    err_secs = sorted(t for t, n in errs.items() if n)
    out["errors"] = sum(errs.values())
    out["error_seconds"] = len(err_secs)
    out["error_window_s"] = (err_secs[-1] - err_secs[0] + 1) if err_secs else 0
    out["first_error_after_fault_s"] = (err_secs[0] - fault) if (err_secs and fault) else None
    # Recovery: seconds from the fault until the first run of 10 consecutive
    # seconds whose p99 is under max(2x pre-fault p99, 25 ms). Isolated
    # one-second blips (GC, host jitter) don't count as "not recovered".
    if fault and pre:
        limit = max(2 * out["pre_fault_p99_ms"], 25)
        streak, rec = 0, None
        for t in range(fault, end + 1):
            ok = (not per_sec.get(t)) or pct(per_sec[t], 99) <= limit
            streak = streak + 1 if ok else 0
            if streak >= 10:
                rec = t - 9 - fault
                break
        out["latency_recovery_s"] = rec
    print(json.dumps(out))


if __name__ == "__main__":
    main(sys.argv[1], int(float(sys.argv[2])) if len(sys.argv) > 2 else 0)
