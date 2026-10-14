# 0006: Redis HA as independent cache replicas, no Sentinel

- Status: Accepted
- Date: 2026-10-14

## Context

The spec asks that "Redis and NATS are configured so a pod kill does not lose
clicks". In this design, clicks never touch Redis. Redis only caches the
immutable slug → URL mapping for redirect-svc. The question is how to keep
the *cache* available, not how to keep data durable.

## Decision

Redis is a pure cache: no persistence, `allkeys-lru`, and `maxmemory`
bounded. In `ha` it runs as **2 independent replicas behind one Service**,
with no replication and no Sentinel. A PDB and zone spread keep at least one
up during voluntary disruptions.

## Why it is correct

Links are immutable (no update or delete in v1), so two caches can never
disagree in a way that matters. At worst one has an entry the other lacks,
which is just a miss. When a Redis pod dies, connections pooled to it fail
once and reconnect through the Service to the survivor. redirect-svc's
circuit breaker and the fallback to links-svc absorb the few failed lookups.

## Alternatives

- **Sentinel / replication**: needed for durable data or read-your-writes.
  Neither applies here. It adds failover time and client complexity.
- **Single replica**: acceptable (the fallback covers it), but every Redis
  restart would cost a full cold cache and a links-svc/Postgres load spike.
  The chaos results quantify this.

## Consequences

- Each replica warms independently, so the effective hit ratio drops a little
  right after a scale-up or a restart.
- If links ever become mutable (edit or delete), this must change. Use
  explicit invalidation (pub/sub or a versioned key) or move to a single
  logical cache.
