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
<!-- results:end -->
