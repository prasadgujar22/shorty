# 0004: Readiness fails only on critical dependencies

- Status: Accepted
- Date: 2026-10-14

## Context

The spec asks for `/readyz` to check dependencies. Applied naively, a shared
dependency blip (Redis restart, NATS leader election) makes every replica of
every service unready at once. Kubernetes then removes all endpoints, and a
partial degradation becomes a full outage. That's a classic cascading failure.

## Decision

`pkg/server` has two kinds of checks:

- `AddCheck` (critical): failure returns 503 and the pod leaves the load
  balancer. Used only when the service cannot do *anything* useful without
  the dependency: Postgres for links-svc and analytics-svc.
- `AddSoftCheck` (degradable): status is reported in the `/readyz` body as
  `"degraded: <err>"` but the response stays 200.
  - redirect-svc: Redis (fall back to links-svc), links-svc (cached slugs
    still redirect), NATS (clicks buffer in memory and retry).
  - analytics-svc: NATS (stats keep being served, ingestion lags).

`/healthz` (liveness) never checks dependencies. Restarting a healthy process
because the database is down only adds load during recovery.

Readiness also returns 503 while the process is starting (before
dependencies are initialised) and while draining after SIGTERM.

## Consequences

- During a dependency outage, metrics and logs show the degradation:
  `shorty_redis_breaker_open`, `shorty_clicks_buffer_depth` and the
  `/readyz` body. Readiness does not.
- A pod with a genuinely broken soft dependency, such as a bad Redis address,
  stays in rotation and serves slower. That is the intended trade-off;
  alerting must cover it.
