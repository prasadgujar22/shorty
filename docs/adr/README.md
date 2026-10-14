# Architecture Decision Records

Short, immutable records of decisions that are hard to reverse or that
interpret the spec. Format: context, decision, consequences. Supersede an ADR
with a new one instead of editing it.

| # | Decision | Status |
|---|---|---|
| [0001](0001-go-for-backend-services.md) | Go for all backend services | Accepted |
| [0002](0002-nats-jetstream-for-click-events.md) | NATS JetStream for click events (vs Kafka, Redis Streams) | Accepted |
| [0003](0003-pull-based-gitops.md) | Pull-based GitOps with Argo CD (vs push from CI) | Accepted |
| [0004](0004-degrade-on-soft-dependencies.md) | Readiness fails only on critical dependencies | Accepted |
| [0005](0005-secrets-out-of-git.md) | Secrets are created out of band, never committed | Accepted |
| [0006](0006-redis-as-independent-cache-replicas.md) | Redis HA as independent cache replicas, no Sentinel | Accepted |
| [0007](0007-routing-and-reserved-slugs.md) | Gateway routing, reserved slugs, host port 8088 | Accepted |
| [0008](0008-effectively-once-click-counting.md) | At-least-once delivery + idempotent aggregation | Accepted |
| [0009](0009-local-platform-choices.md) | Local platform choices (CNPG, Alloy, chart layout, toolchain) | Accepted |
