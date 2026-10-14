# Chaos and load results

Each row is one `chaos/run.sh <scenario>` run: k6 drives a constant arrival
rate of `GET /{slug}` against the **ha** environment (`shorty-ha`, via
Traefik at `ha.localhost:8088`). The fault is injected 45 s after load
starts. After the run, the redirects k6 observed are reconciled against what
analytics-svc counted in Postgres, which is the "were clicks lost?" column.

Columns:
- **p50/p95/p99**: whole run.
- **fault p99**: the 60 s after the fault.
- **worst 1s p99**: the worst single second.
- **err %**: share of requests that didn't return 302.
- **err window**: first-to-last second with errors.
- **recovery**: seconds until the per-second p99 is back under 2× the pre-fault p99.

<!-- results:begin -->
| Date | Scenario | Env | rps | p50 ms | p95 ms | p99 ms | fault p99 ms | worst 1s p99 ms | err % | err window | recovery | 302s seen | clicks counted | lost | Notes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 2026-10-14 11:57 | `baseline` | shorty-ha | 800 | 0.71 | 2.52 | 6.80 | 6.48 | 66.88 | 0.000 | 0s | 0s | 120001 | 120001 | 0 |  |
| 2026-10-14 12:00 | `kill-pods` | shorty-ha | 800 | 0.73 | 3.36 | 10.81 | 15.53 | 100.08 | 0.000 | 0s | 6s | 120001 | 120001 | 0 |  |
| 2026-10-14 12:04 | `kill-pods-force` | shorty-ha | 800 | 0.74 | 3.45 | 11.24 | 22.93 | 125.55 | 0.000 | 0s | 0s | 119988 | 119988 | 0 |  |
| 2026-10-14 12:07 | `drain-node` | shorty-ha | 800 | 1.04 | 16.07 | 86.46 | 136.01 | 448.98 | 0.000 | 0s | 93s | 119915 | 119915 | 0 |  |
| 2026-10-14 12:10 | `kill-redis` | shorty-ha | 800 | 0.72 | 3.03 | 10.14 | 11.4 | 101.64 | 0.000 | 0s | 11s | 120001 | 120001 | 0 |  |
| 2026-10-14 12:14 | `kill-nats` (follower) | shorty-ha | 800 | 0.74 | 3.92 | 24.16 | 32.19 | 209.69 | 0.000 | 0s | 0s | 119939 | 119939 | 0 | SIGKILL nats-0 (a follower: leader lookup bug, fixed below) |
| 2026-10-14 12:17 | `kill-postgres-primary` | shorty-ha | 800 | 0.76 | 3.26 | 10.01 | 10.86 | 238.32 | 0.000 | 0s | 2s | 119920 | 119920 | 0 |  |
| 2026-10-14 12:21 | `kill-nats` | shorty-ha | 768 | 1.90 | 1022.16 | 2490.80 | 3194.81 | 5000.31 | 1.689 | 23s | 81s | 113252 | 113265 | -13 | SIGKILL CLICKS stream leader (Raft election) |
| 2026-10-14 12:25 | `kill-redis` | shorty-ha | 764 | 1.67 | 197.22 | 3184.03 | 92.15 | 5061.27 | 0.681 | 14s | 13s | 113733 | 114073 | -340 | ALL=1: both Redis replicas SIGKILLed (cold cache) |
<!-- results:end -->
