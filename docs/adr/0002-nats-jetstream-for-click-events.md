# 0002: NATS JetStream for click events

- Status: Accepted
- Date: 2026-10-14

## Context

Every redirect produces a click event that must reach analytics-svc without
slowing the redirect, and without being lost when a pod, node or broker is
killed (the chaos tests do exactly that). Volume at laptop scale is a few
thousand events per second. The broker must run in roughly 100–300 MB per
node and support a 3-replica HA setup on k3d.

## Decision

Use **NATS JetStream**: a file-backed `CLICKS` stream (R1 in dev, R3 across
three zones in ha), producers publishing with a unique `Nats-Msg-Id`, and a
durable pull consumer with explicit acks for analytics-svc.

## Comparison

| | NATS JetStream | Kafka (KRaft) | Redis Streams |
|---|---|---|---|
| Memory per broker (idle) | ~20 MB | 500 MB–1 GB+ (JVM) | ~10 MB, but shared with the cache |
| HA model | Raft per stream (R3), automatic leader election | ISR replication, KRaft quorum | Replica + Sentinel/Cluster; async replication can lose acked writes on failover |
| Producer dedup | Built-in (`Nats-Msg-Id` within a window) | Idempotent producer (per session) | None |
| Consumer model | Durable pull consumers, ack/nak/term, redelivery, max-ack-pending | Consumer groups, offset commits | Consumer groups, XACK, XCLAIM |
| Ops on k8s | One small StatefulSet, official chart | Operator (Strimzi) + more moving parts | Already deployed, but would mix cache and durable log |
| Throughput ceiling | Far above our needs | Highest | High |

## Why

- Durability and HA comparable to Kafka at a fraction of the footprint. R3
  streams survive the loss of any one NATS pod with no acked-message loss
  (Raft), and that is what the `kill-nats` chaos scenario checks.
- Server-side publish dedup plus explicit per-message ack keeps the
  at-least-once to effectively-once story simple (see ADR 0008).
- Keeping the event log out of Redis lets Redis stay a pure, disposable cache
  (ADR 0006). Losing Redis then never risks click data.

## Consequences

- No log compaction or long-term replay at Kafka scale. The stream keeps 7
  days (`CLICKS_STREAM_MAX_AGE`), which is enough to rebuild aggregates after
  an analytics outage, but this is not an archive.
- Clients must handle `RetryOnFailedConnect` and publish retries. That logic
  lives in `pkg/events` and `services/redirect-svc/internal/clicks`.
- Partitioned ordering isn't needed (aggregation is commutative), so we
  don't use subject partitioning.
