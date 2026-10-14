# Chaos and load results

Measured on a MacBook Air M2 (8 cores, 24 GB), Docker Desktop with 12 GiB,
k3d with 1 server + 3 agents (zones a/b/c). The system under test is the
**ha** environment (`shorty-ha`, synced by Argo CD from release tags):

- redirect-svc: 3–8 replicas (HPA on CPU + RPS)
- links-svc, analytics-svc, web, Redis: 2 replicas each
- NATS: a 3-node cluster with an R3 file-backed `CLICKS` stream
- Postgres: CloudNativePG, primary plus a quorum-synchronous standby

Every replica is spread across zones and covered by a PDB.

## Method

`chaos/run.sh <scenario>`:
1. k6 (`load/redirect.js`) creates 200 fresh links, then drives a constant
   **800 rps** of `GET /{slug}` for 150 s (redirects not followed; 20% of
   traffic goes to a hot 5% of links).
2. 45 s into the load, the scenario script injects the fault using only
   `kubectl`: `chaos/kill-pods.sh`, `drain-node.sh`, `kill-redis.sh`,
   `kill-nats.sh` or `kill-postgres-primary.sh`.
3. After the load, it waits for the click pipeline to drain and
   **reconciles** the number of 302s k6 received with
   `SUM(analytics.link_totals.clicks)` for that run's slugs. `lost = seen −
   counted`. A negative value means clicks were served and counted after the
   client had already timed out (5 s), so nothing was lost.
4. `chaos/analyze.py` computes per-second p99 and error windows from k6's
   CSV, and `chaos/serverside.py` adds the **server-side** view from
   Prometheus: redirect-svc's own p99 in the 60 s after the fault, and 5xx
   responses served by redirect-svc and by Traefik.

### Why there are two latency views

k6 runs on the macOS host, so its numbers include Docker Desktop's port
proxy and the k3d load balancer. Same load and moment, two vantage points:

| Load generator | Path | p50 | p95 | p99 |
|---|---|---|---|---|
| k6 in-cluster (`load/k8s/k6-job.yaml`, control-plane node) | pod → Traefik → redirect-svc | 0.33 ms | 0.77 ms | **1.72 ms** |
| k6 on the macOS host | host → Docker port proxy → k3d LB → Traefik → redirect-svc | 0.63 ms | 1.58 ms | **3.85 ms** |

The host path roughly doubles tail latency, and it occasionally stalls for
seconds in ways redirect-svc never sees. Several v0.2.0 runs show 2–5 s
worst seconds client-side while redirect-svc's own p99 stayed ≤ 0.5 ms. Read
the **svc p99** and **svc / gw 5xx** columns for service behaviour, and the
k6 columns for end-to-end experience on this laptop.

Columns:
- **p50/p95/p99**: k6, whole run.
- **fault p99**: k6, 60 s after the fault.
- **worst 1s p99**: k6, the worst single second.
- **err %**: share of requests that didn't return 302.
- **err window**: first-to-last second with errors.
- **recovery**: seconds from the fault until 10 consecutive seconds of p99 ≤
  max(2× pre-fault, 25 ms).
- **svc p99 (fault)**: redirect-svc histogram, worst 30 s window.
- **svc / gw 5xx**: 5xx served by redirect-svc / by Traefik for shorty-ha.

## Results

<!-- results:begin -->
| Date | Scenario | Env | rps | p50 ms | p95 ms | p99 ms | fault p99 ms | worst 1s p99 ms | err % | err window | recovery | 302s seen | clicks counted | lost | svc p99 (fault) ms | svc / gw 5xx | Notes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 2026-10-14 11:57 | `baseline` | shorty-ha | 800 | 0.71 | 2.52 | 6.80 | 6.48 | 66.88 | 0.000 | 0s | 0s | 120001 | 120001 | 0 | 1.15 | 0 / 0 | v0.1.0 |
| 2026-10-14 12:00 | `kill-pods` | shorty-ha | 800 | 0.73 | 3.36 | 10.81 | 15.53 | 100.08 | 0.000 | 0s | 6s | 120001 | 120001 | 0 | 4.55 | 0 / 0 | v0.1.0 |
| 2026-10-14 12:04 | `kill-pods-force` | shorty-ha | 800 | 0.74 | 3.45 | 11.24 | 22.93 | 125.55 | 0.000 | 0s | 0s | 119988 | 119988 | 0 | 8.54 | 0 / 0 | v0.1.0 |
| 2026-10-14 12:07 | `drain-node` | shorty-ha | 800 | 1.04 | 16.07 | 86.46 | 136.01 | 448.98 | 0.000 | 0s | 93s | 119915 | 119915 | 0 | 112.02 | 0 / 0 | v0.1.0 |
| 2026-10-14 12:10 | `kill-redis` | shorty-ha | 800 | 0.72 | 3.03 | 10.14 | 11.4 | 101.64 | 0.000 | 0s | 11s | 120001 | 120001 | 0 | 2.38 | 0 / 0 | v0.1.0 |
| 2026-10-14 12:14 | `kill-nats` (follower) | shorty-ha | 800 | 0.74 | 3.92 | 24.16 | 32.19 | 209.69 | 0.000 | 0s | 0s | 119939 | 119939 | 0 | 6.53 | 0 / 0 | v0.1.0; SIGKILL nats-0 (a follower: leader lookup bug, fixed below) |
| 2026-10-14 12:17 | `kill-postgres-primary` | shorty-ha | 800 | 0.76 | 3.26 | 10.01 | 10.86 | 238.32 | 0.000 | 0s | 2s | 119920 | 119920 | 0 | 2.86 | 0 / 0 | v0.1.0 |
| 2026-10-14 12:21 | `kill-nats` | shorty-ha | 768 | 1.90 | 1022.16 | 2490.80 | 3194.81 | 5000.31 | 1.689 | 23s | 81s | 113252 | 113265 | -13 | 2350.55 | 1322 / 1077 | v0.1.0; SIGKILL CLICKS stream leader (Raft election) |
| 2026-10-14 12:25 | `kill-redis` | shorty-ha | 764 | 1.67 | 197.22 | 3184.03 | 92.15 | 5061.27 | 0.681 | 14s | 13s | 113733 | 114073 | -340 | 810.84 | 27 / 24 | v0.1.0; ALL=1: both Redis replicas SIGKILLed (cold cache) |
| 2026-10-14 12:51 | `baseline` | shorty-ha | 794 | 0.64 | 2.53 | 107.70 | 4.8 | 2081.26 | 0.000 | 0s | 0s | 119076 | 119076 | 0 | 0.5 | 0 / 0 | v0.2.0 |
| 2026-10-14 12:54 | `kill-pods` | shorty-ha | 792 | 0.64 | 4.04 | 255.59 | 1407.94 | 2743.68 | 0.000 | 0s | 5s | 118821 | 118821 | 0 | 4.74 | 0 / 0 | v0.2.0 |
| 2026-10-14 12:58 | `kill-pods-force` | shorty-ha | 780 | 0.90 | 657.22 | 1968.78 | 3354.89 | 5000.21 | 0.163 | 16s | 2s | 116714 | 116781 | -67 | 228.86 | 0 / 10 | v0.2.0 |
| 2026-10-14 13:04 | `drain-node` | shorty-ha | 800 | 1.24 | 9.36 | 27.58 | 34.37 | 138.75 | 0.000 | 0s | 37s | 119994 | 119994 | 0 | 0.5 | 0 / 0 | v0.2.0 |
| 2026-10-14 13:07 | `kill-redis` | shorty-ha | 800 | 0.75 | 2.57 | 8.83 | 6.91 | 127.4 | 0.000 | 0s | 0s | 119993 | 119993 | 0 | 0.53 | 0 / 0 | v0.2.0 |
| 2026-10-14 13:11 | `kill-nats` | shorty-ha | 800 | 0.71 | 2.33 | 6.74 | 6.87 | 157.38 | 0.000 | 0s | 1s | 119971 | 119971 | 0 | 0.5 | 0 / 0 | v0.2.0 |
| 2026-10-14 13:14 | `kill-postgres-primary` | shorty-ha | 800 | 0.90 | 4.21 | 11.69 | 9.34 | 119.07 | 0.000 | 0s | 5s | 119985 | 119985 | 0 | 0.5 | 0 / 0 | v0.2.0 |
| 2026-10-14 13:18 | `kill-redis` | shorty-ha | 800 | 0.66 | 1.82 | 5.16 | 7.6 | 149.18 | 0.000 | 0s | 0s | 119980 | 119980 | 0 | 0.84 | 0 / 0 | v0.2.0; ALL=1: both Redis replicas SIGKILLed (cold cache) |
<!-- results:end -->

## What the numbers say

**No click was lost in any run.** That's 17 runs and about 2 million
redirects, including SIGKILLs of pods, the NATS stream leader and the
Postgres primary. The effectively-once design holds under every fault we
injected ([ADR 0008](adr/0008-effectively-once-click-counting.md)):
- redirect-svc buffers clicks and retries publishing until JetStream acks;
- events carry a unique `Nats-Msg-Id`;
- analytics-svc acks only after COMMIT and dedups through `processed_events`.

### Per scenario (v0.2.0)

- **Pod kills (graceful and SIGKILL, 12 pods in 30 s)**: zero server-side
  5xx from redirect-svc. With SIGKILL, Traefik returned 10 5xx for requests
  in flight on a killed pod. Its retry middleware covers connection failures,
  not requests already sent. Graceful kills are fully clean because of the
  preStop sleep, readiness 503 while draining, and `http.Server.Shutdown`.
- **Node drain**: PDBs keep ≥ 1 replica of everything up, and topology
  spread puts replacements on the remaining zones. But node-local
  (`local-path`) volumes cannot follow their pods. The evicted NATS replica,
  Postgres standby and (in one run) Prometheus stayed **Pending until
  uncordon**, and the cluster ran on replication alone (NATS 2/3, Postgres
  primary only) for the whole drain. With network storage they would
  reschedule. On a laptop this is the honest trade-off. The drain script
  avoids the node hosting the Postgres primary, because CNPG's primary PDB
  (correctly) blocks that eviction.
- **Redis (one or both replicas)**: a cache-only failure. With v0.2.0 it is
  invisible to clients: hot slugs are served from the in-process L1 cache,
  and the rest go through the breaker to links-svc.
- **NATS stream leader SIGKILL**: Raft elects a new leader in about 1–2 s.
  Publishes fail during the election and are retried from the buffer, which
  peaked at about 1.2k events. Zero client-visible impact on v0.2.0.
- **Postgres primary SIGKILL**: CNPG promotes the synchronous standby.
  redirect-svc doesn't touch Postgres on the hot path. links-svc writes and
  analytics batches fail for a few seconds and are retried; analytics NAKs
  the batch and JetStream redelivers it.

### v0.1.0 → v0.2.0: what changed and why

The v0.1.0 NATS-leader and kill-all-Redis runs produced **1.7% and 0.7%
client errors**, with redirect-svc's own p99 at **2.35 s and 0.81 s**. The
errors started 30–70 s *after* the fault, which is the clue that the fault
itself wasn't the direct cause:

1. After a NATS replica restart, JetStream resyncs a 1.15M-message R3
   stream, while Postgres WAL, Prometheus and Loki also write. Every k3d
   "node" shares one Docker VM disk, so disk-busy time hit ~100% and write
   throughput doubled.
2. Redis lookups (100 ms read timeout) stalled for up to 500 ms or more: a
   retry plus pool wait came to ~6× the read timeout. The breaker never
   opened because failures weren't consecutive.
3. Every slow lookup fell back to links-svc, which saturated its Postgres
   pool (8 conns). redirect-svc's 500 ms links timeout cancelled the
   requests, and they surfaced as 503s.

v0.2.0 (commit `9882c83`):
- adds an in-process **L1 LRU** (10k entries, 30 s TTL, 5 s negative TTL)
  in front of Redis;
- makes the Redis client **fail fast** (no retries, pool wait = read
  timeout);
- **caps the CLICKS stream at 256 MiB** so replica resync stays small.

Result: the same faults produced **0 errors**, and redirect-svc's p99
dropped from 2,351 ms to 0.5 ms (NATS leader) and from 811 ms to 0.84 ms
(both Redis replicas killed). Details of the knobs are in
[TUNING.md](TUNING.md).

The v0.2.0 promotion itself also found a bug. Helm rendered
`CLICKS_STREAM_MAX_BYTES: 268435456` as `2.68435456e+08`, redirect-svc's
strict config parsing refused to start, and the rollout halted at the first
pod (`maxUnavailable: 0`) while v0.1.0 kept serving. The library chart now
renders integers correctly, and `scripts/lint-manifests.sh` fails CI on
scientific notation.

### Known limits of this setup

- Every "node" is a container in one Docker VM: CPU, disk and network are
  shared, so a node failure is never independent like a real host or AZ
  failure. The noisy-neighbour effect above is a real lesson, though.
- k6 shares the laptop with the cluster. Client-side tails above ~5 ms
  should be read together with the server-side columns.
- Each scenario was run once per version. Treat differences under ~2× in
  tail latency as noise. The error and lost-click columns are the robust
  signal.

Reproduce: `make chaos` (all scenarios) or `make chaos-kill-nats`, with
`NS=shorty-ha RATE=800`. Raw per-run summaries go to `load/results/`
(gitignored).
