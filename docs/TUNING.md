# Tuning notes

These are the knobs explored on Shorty, each with before/after numbers.
Unless stated otherwise:

- **Target**: the local `shorty` namespace, with a single replica of each
  service, images `73bc509` (v0.2.0 code).
- **Harness**: `load/tune.sh` and `load/experiments.sh`, with **k6 running
  in-cluster** on the control-plane node so the macOS → Docker port proxy is
  not part of the measurement (see [RESULTS.md](RESULTS.md#why-there-are-two-latency-views)).
- **Data**: each experiment is 60 s of constant arrival rate. The Prometheus
  numbers cover the run window. Raw rows are in
  `load/results/tuning.jsonl`.
- **Caveat**: one run per setting on a shared laptop VM. Differences in
  tail latency below about 2× are noise. The structural effects (throttling,
  OOM, pool waits, backend load) are robust.

| # | Knob | Default | Where |
|---|---|---|---|
| 1 | CPU limit vs `GOMAXPROCS` | limit 1 CPU, GOMAXPROCS automatic | `deploy/charts/*/values.yaml`, env `GOMAXPROCS` |
| 2 | `GOMEMLIMIT` from the memory limit | 0.9 × limit | `pkg/memlimit`, env `GOMEMLIMIT_RATIO` / `GOMEMLIMIT` |
| 3 | Postgres pool size | 10 (links, analytics), 8/6 in ha | env `DB_MAX_CONNS`, `DB_MIN_CONNS` |
| 4 | Cache layers and TTLs | L1 10k × 30 s, Redis 1 h ± 10%, negative 30 s / 5 s | env `L1_CACHE_*`, `CACHE_TTL*` |
| 5 | JetStream publish concurrency | 16 workers, 100k buffer | env `CLICK_PUBLISH_WORKERS`, `CLICK_BUFFER_SIZE` |
| 6 | JetStream consumer batch / ack | batch 500, fetch wait 500 ms, ack wait 30 s | env `CONSUMER_*` |
| 7 | JetStream stream size | 1 GiB (dev), 256 MiB (ha) | env `CLICKS_STREAM_MAX_BYTES` |
| 8 | Redis client timeouts / retries | read 100 ms, no retry, pool wait = read timeout | env `REDIS_*` |
| 9 | HPA metrics and thresholds | CPU 60% + 400 rps/pod, 3–8 replicas | `deploy/envs/ha/values.yaml`, adapter rule |

---

## 1. CPU limits and GOMAXPROCS

Since Go 1.25 the runtime sets `GOMAXPROCS` from the cgroup CPU quota, but
**never below 2**. Fractional limits like `250m` therefore still get 2 Ps,
and two Ps can burn a 25 ms-per-100 ms quota in about 12 ms. The container
then sits throttled for the rest of the period, and every request queued in
that gap waits.

redirect-svc, single pod, 1,500 rps (a redirect costs about 0.1 ms CPU, so
the pod needs about 0.12–0.17 cores):

| CPU limit | GOMAXPROCS | p50 | p99 | throttled CFS periods |
|---|---|---|---|---|
| 150m | 2 (auto) | 2.67 ms | **76.8 ms** | **92%** |
| 150m | **1** | 0.39 ms | 59.9 ms | 59% |
| 250m | 2 (auto) | 0.35 ms | 5.7 ms | 7.9% |
| 250m | **1** | 0.35 ms | **2.3 ms** | 1.2% |
| 1 | 2 (auto) | 0.36 ms | 1.85 ms | 0% |

Takeaways:
- With a limit under 1 CPU, set `GOMAXPROCS=1` explicitly. That cut p99 by
  2.5× at 250m. Or simply don't set fractional limits for latency-sensitive
  Go services.
- Shorty runs redirect-svc with `limits.cpu: 1` (GOMAXPROCS 2, zero
  throttling at this load) and `requests.cpu: 100m`, so the HPA's CPU
  utilisation is measured against a small request and scales early.
- Throttling shows on every service dashboard ("CPU throttling" panel,
  `container_cpu_cfs_throttled_periods_total`).

## 2. GOMEMLIMIT

`pkg/memlimit` reads the container limit from the downward API
(`MEMORY_LIMIT_BYTES`) and sets the soft memory limit to 90% of it, unless
`GOMEMLIMIT` is set explicitly.

Worst case for redirect-svc: NATS unreachable, so every click stays in the
in-memory buffer. Limit 32 Mi, 1,500 rps for 60 s (≈ 90k buffered events):

| Setting | Outcome | Max buffered | GC cycles/s | Memory (max WSS) | p99 | Client errors |
|---|---|---|---|---|---|---|
| `GOMEMLIMIT=off` (GOGC only) | **OOMKilled** after ~34k events; those clicks lost with the pod | 34,414 | 0.3 | 30.6 Mi | 1.6 ms | **5.3%** (only replica restarting) |
| `GOMEMLIMIT` = 0.9 × 32 Mi | **Survived**, all 90k events held | 89,983 | **125** | 31.4 Mi | 12.3 ms | 0 |

Takeaways:
- GOMEMLIMIT turns a hard OOM kill into GC CPU. Without it, GOGC=100 lets
  the heap grow to 2× live before collecting, which overshoots a tight limit.
- 125 GC cycles/s is the beginning of a GC death spiral. That's why the
  buffer is bounded (`CLICK_BUFFER_SIZE`) and drops are counted
  (`shorty_clicks_dropped_total`, alert `ShortyClicksDropped`), and why
  production limits leave headroom: redirect-svc runs with 128 Mi, about 4×
  its steady-state working set.
- Leave about 10% for non-heap memory (goroutine stacks, runtime). 0.9 is a
  good default; drop to 0.8 for services with large cgo or mmap usage.

## 3. Postgres connection pool

links-svc under 1,200 rps of API traffic (90% `GET /api/links/{slug}`, 10%
creates) bypassing the redirect cache, CPU limit 1:

| `DB_MAX_CONNS` | p50 | p99 | empty-pool acquires/s | avg acquire wait |
|---|---|---|---|---|
| 2 | 0.41 ms | **3.32 ms** | **54.6** | 87 µs |
| 10 | 0.41 ms | **1.54 ms** | 0.2 | 2 µs |
| 40 | 0.40 ms | 1.80 ms | 0.2 | 3 µs |

Takeaways:
- Queries are ~0.3 ms, so by Little's law about 0.3 ms × 1,200/s ≈ 0.4
  connections are busy on average. Bursts need a handful, and 10 absorbs
  them completely.
- Going to 40 buys nothing and costs Postgres backends (≈ 5–10 MB each).
  Across replicas, `max_conns × replicas` must stay well under
  `max_connections` (100). In ha that's links 2 × 8 + analytics 2 × 6 = 28.
- Watch `shorty_db_pool_empty_acquires_total` (dashboard "Postgres pool"
  row). A sustained non-zero rate is the signal to grow the pool or add
  replicas.

## 4. Cache layers: L1, Redis TTL, negative caching

redirect-svc at 1,500 rps over 2,000 links (20% of traffic on a hot 5%):

| Setup | p50 | p99 | cache hit ratio | links-svc rps | redirect CPU |
|---|---|---|---|---|---|
| No cache (L1 off, Redis TTL 1 s) | 0.60 ms | 4.15 ms | 47% (singleflight + 1 s TTL) | **515** | 0.22 cores |
| Redis only (TTL 1 h) | 0.41 ms | 1.89 ms | 100% | 54 | 0.18 cores |
| **L1 + Redis** (v0.2.0) | 0.34 ms | 2.24 ms | 99.9% | 54 | **0.12 cores** |

In steady state, Redis alone already removes about 90% of links-svc load,
and the L1 mainly saves CPU (−33%). The L1 earns its place **under
failure**. In the chaos suite (see [RESULTS.md](RESULTS.md)) the same faults
went from 1.7% errors and a 2.35 s service p99 (v0.1.0, Redis-only) to 0
errors and 0.5 ms (v0.2.0), because hot slugs never leave the process while
Redis or links-svc are slow.

TTL choices:
- **Redis TTL 1 h ± 10% jitter**: links are immutable, so a long TTL is
  safe. Jitter spreads expiry so a popular batch of links doesn't expire
  together and stampede links-svc. Singleflight also collapses concurrent
  misses for one slug into one call.
- **L1 TTL 30 s, 10k entries**: bounds memory (~2 MB) and how long a
  (future) deleted link could keep resolving.
- **Negative TTL 30 s (Redis) / 5 s (L1)**: scanners probing random paths
  would otherwise hit links-svc and Postgres on every request. Kept short so
  a slug created right after a 404 starts working quickly.

## 5. JetStream publishing

Clicks are published synchronously (wait for the ack, which is replicated
on R3) by N workers draining a bounded buffer. In-cluster, an ack takes
about 0.5 ms, so one worker tops out around 2,000 events/s.

redirect-svc at 2,000 rps (R1 stream, local env):

| Workers | p99 (redirect) | max buffer depth | publish p99 |
|---|---|---|---|
| 1 | 1.99 ms | 3 | 0.50 ms |
| 4 | 1.89 ms | 1 | 0.89 ms |
| 16 | 1.80 ms | 0 | 0.92 ms |

Takeaways:
- The redirect latency is independent of publishing, because the buffer
  decouples the two.
- Workers determine **drain rate after an outage**. After a 10 s NATS
  leader election at 2,000 rps, 20k events are queued. One worker (at its
  ceiling) never catches up while new traffic keeps arriving. 16 workers
  drain at about 16× the arrival rate. In the NATS-leader chaos run the
  buffer peaked around 1.2k events and drained within seconds.
- An R3 ack in ha costs one quorum round trip. More workers hide that
  latency, and the `Nats-Msg-Id` dedup makes retries safe.
- An alternative is async publishing with a bounded number of in-flight
  acks (`PublishAsync`), which gives higher throughput per connection. It
  wasn't needed at this scale.

## 6. JetStream consumer (analytics-svc)

redirect at 1,500 rps, analytics-svc with a durable pull consumer:

| Batch size | events/s applied | max consumer lag | max freshness (event age when applied) |
|---|---|---|---|
| 1 | 1,072 | 3 | 0.01 s |
| 50 | 1,006 | 5 | 0.50 s |
| 500 | 1,003 | 0 | 0.02 s |

(events/s is averaged over a 90 s window that includes idle time.) With
local Postgres all three keep up. The difference is cost: batch=1 is one
transaction per click, plus a `processed_events` insert and three upserts.
That is about 1,500 commits/s, and in ha each commit waits for the
synchronous standby. Batch=500 is about 3 commits/s for the same work.

Settings:
- `CONSUMER_FETCH_MAX_WAIT=500ms` bounds added latency at low traffic. At
  high traffic batches fill instantly.
- `CONSUMER_ACK_WAIT=30s` and `MaxAckPending=20000`: a batch must commit
  well within AckWait (the apply context uses AckWait/2), or JetStream
  redelivers it to another replica. Redelivery is safe (idempotent) but
  wasteful.
- `MaxDeliver=-1` with explicit `Term()` for undecodable messages: a
  Postgres outage must never dead-letter good clicks.

## 7. Stream size and replica recovery

In v0.1.0 the `CLICKS` stream grew to 1.15M messages (several hundred MB × 3
replicas) during load testing. When a NATS pod restarted, its resync, plus
Postgres WAL and the observability stack, saturated the Docker VM's single
disk. That is how a NATS restart caused **Redis** latency 30–70 s later
(RESULTS.md, "v0.1.0 → v0.2.0"). `CLICKS_STREAM_MAX_BYTES=256MiB` in ha
bounds resync volume. Retention stays at 7 days (`MaxAge`) for normal
traffic, and DiscardOld is safe because the consumer runs about 0.5 s
behind, not days.

## 8. Redis client

| Setting | v0.1.0 | v0.2.0 |
|---|---|---|
| `MaxRetries` | 1 (go-redis also counts the pool wait) | **-1** (no retry) |
| `PoolTimeout` | 2 × ReadTimeout | **= ReadTimeout** |
| Worst-case lookup | ~6 × 100 ms (observed 500 ms+ stalls) | ~100 ms |
| Circuit breaker | 5 consecutive failures → open 2 s | unchanged |

A cache that is slower than the system of record is worse than no cache.
With the L1 in front, a failed Redis call costs one fast error. The breaker
then skips Redis entirely after five in a row.

## 9. HPA (redirect-svc, ha)

`load/hpa.sh` runs in-cluster k6 against the ha env with a ramp of 300 rps
(30 s) → 2,400 rps (over 60 s) → hold 2 min → 300 rps, sampling the HPA
every 5 s. Start state: 3 replicas, max 8. The scale-up policy allows +100%
per 15 s with no stabilisation; scale-down waits 120 s, then removes 1 pod
per minute.

| HPA metrics | first scale-up (s after start) | path | peak replicas | k6 p95 | k6 p99 | dropped iterations |
|---|---|---|---|---|---|---|
| CPU 60% + RPS 400/pod (`rate[1m]`) | 85 | 3→4→6→8 | 8 | 5.1 ms | 19.8 ms | 183 |
| CPU 60% only | 90 | 3→4→6→8 | 8 | 5.3 ms | 23.2 ms | 631 |
| CPU 60% + RPS **250/pod** (`rate[30s]`) | **74** | 3→**5→7**→8 | 8 | 73.5 ms | 229.7 ms | 5,273 |

What happened:
- **The first RPS configuration never led a decision.** The HPA takes the
  maximum of the per-metric recommendations. With a 1-minute rate window the
  RPS signal trailed the ramp, and 400 rps/pod was above the point where CPU
  (requests 100m, so 60% = 60 millicores) already fired. CPU-only behaved the
  same.
- **The tuned signal (30 s window, 250/pod) leads.** It scaled about 15 s
  earlier and in bigger steps, as intended.
- **On this laptop that didn't help clients, and that's a property of the
  testbed.** redirect-svc's own p99 stayed ≤ 7 ms in all three runs. During
  the tuned run the VM hit **~90% CPU**, with k6 at ~2.4 cores and Traefik at
  ~1.3 cores, all on the same 8 cores as the redirect pods. Extra replicas
  add no capacity when every node is a container in one VM. They add startup
  and scheduling work at the worst moment, and the load generator starves.
  On real nodes, new pods bring new CPU, which is exactly when a leading
  indicator pays off.

Kept configuration (ha): **CPU 60% + RPS 250/pod over 30 s**, min 3 (one per
zone), max 8. Notes:
- `requests.cpu: 100m` is deliberately small for a 1-CPU limit. Utilisation
  is measured against the request, so the HPA reacts long before the pod is
  anywhere near its limit.
- `minReplicas: 3` = one per zone. With `maxSkew: 1` zone spread and a PDB of
  `maxUnavailable: 1`, losing a zone or draining a node still leaves 2.
- The adapter's `rate[30s]` needs at least two scrapes in the window (scrape
  interval 15 s). Don't go below 30 s.
- Scale-down is deliberately slow (120 s window, 1 pod/min) so a dip in a
  bursty load doesn't throw away warm L1 caches.

## Summary of defaults chosen

| Knob | Value | Why |
|---|---|---|
| redirect-svc CPU | request 100m, limit 1 (GOMAXPROCS 2) | no throttling at 1.5k rps/pod; early HPA signal |
| GOMEMLIMIT | 0.9 × memory limit | turns OOM kills into GC work (survived a 90k-event backlog in 32 Mi) |
| `DB_MAX_CONNS` | 10 (dev), 8/6 (ha, 2 replicas each) | 2 → 55 empty-pool waits/s; 40 no better than 10 |
| L1 cache | 10k entries, 30 s, negative 5 s | 0 errors under Redis/NATS faults (vs 0.7–1.7%), −33% CPU |
| Redis | TTL 1 h ± 10%, negative 30 s, no retries, 100 ms timeouts | fail fast; immutable links |
| Publish workers / buffer | 16 / 100k | 1 worker saturates at ~2k events/s; 16 drains a backlog quickly |
| Consumer batch | 500, fetch wait 500 ms | ~3 commits/s instead of ~1,500 at the same throughput |
| Stream size (ha) | 256 MiB, 7 d | bounded replica resync after restarts |
| HPA | CPU 60% + 250 rps/pod (30 s), 3–8 | RPS leads CPU by ~15 s |
