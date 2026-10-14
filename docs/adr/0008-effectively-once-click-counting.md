# 0008: At-least-once delivery + idempotent aggregation

- Status: Accepted
- Date: 2026-10-14

## Context

Click counts should be exact through pod kills, node drains, NATS failover
and Postgres failover. True exactly-once delivery across a broker and a
database isn't available. We need at-least-once delivery and idempotent
processing.

## Decision

**Producer (redirect-svc)**
- The redirect returns before the event is published. The event goes into a
  bounded in-memory buffer (100k by default).
- Publisher workers retry each event with backoff until JetStream acks it
  (persisted on a quorum for R3 streams).
- Each event has a random 128-bit ID sent as `Nats-Msg-Id`, so a retry after
  a lost ack is dropped by JetStream's duplicate window (2 min).
- On SIGTERM the HTTP server drains first, then the buffer is flushed before
  exit (`terminationGracePeriodSeconds` covers it).

**Consumer (analytics-svc)**
- A durable pull consumer with explicit acks.
- Each batch is applied in one Postgres transaction that first
  `INSERT ... ON CONFLICT DO NOTHING RETURNING` the event IDs into
  `processed_events`, then only aggregates IDs it newly claimed. Upserts are
  key-sorted so concurrent replicas can't deadlock.
- Acks are sent only after COMMIT. On failure the batch is NAKed with a delay.
- A crash between COMMIT and ack leads to redelivery, and the IDs are already
  claimed, so the redelivery is a no-op.
- Poison messages are `Term()`ed and counted, never retried forever.
- `processed_events` is pruned after 8 days, longer than the stream's 7-day
  retention, so no redelivery can outlive its dedup record.

## Known loss windows (measured in docs/RESULTS.md)

- **SIGKILL / OOM of a redirect-svc pod** loses whatever is in its buffer at
  that instant. Normally that's ~0 events (the buffer drains in
  milliseconds), but up to the backlog if NATS was also down. `kubectl
  delete pod` sends SIGTERM and is lossless.
- **Buffer overflow** during a long NATS outage drops events. Drops are
  counted in `shorty_clicks_dropped_total`.

## Consequences

- One extra table write per click (the idempotency ledger). That's cheap with
  batching: 500 events per transaction.
- Counts are exact unless a loss window above is hit, and each window is
  observable.
