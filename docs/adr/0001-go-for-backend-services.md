# 0001: Go for all backend services

- Status: Accepted
- Date: 2026-10-14

## Context

The three backend services are small network services: an API over Postgres,
a latency-critical redirector, and a stream consumer. The project is about
HA and tuning, and it runs on an 8-core / 24 GB laptop that also hosts the
whole platform. Images must be distroless, non-root and multi-arch.

## Decision

Write links-svc, redirect-svc and analytics-svc in Go (1.27), in one module
with shared packages under `pkg/` (config, logging, server lifecycle,
metrics, Postgres, events).

## Why

- **Footprint**: each service idles at about 7 MiB RSS and starts in
  milliseconds. Three services × 2–3 replicas plus the platform still fit in
  a laptop Docker VM. A JVM would need roughly 10× the memory per replica.
- **Container-native tuning knobs**: GOMAXPROCS follows the cgroup CPU limit
  (Go ≥ 1.25), and GOMEMLIMIT can be derived from the memory limit
  (`pkg/memlimit`). Both are first-class topics in docs/TUNING.md.
- **Static binaries**: `CGO_ENABLED=0` cross-compiles to amd64 and arm64 on
  the build host, so multi-arch images need no QEMU. Final images are
  `distroless/static:nonroot` at about 23 MB.
- **Stdlib coverage**: `net/http` routing with method and wildcard patterns,
  `log/slog` JSON logs, and `http.Server.Shutdown` for draining. Third-party
  dependencies are limited to pgx, go-redis, nats.go and the Prometheus client.
- **Mature clients** for every datastore in the stack.

## Alternatives considered

- **Rust**: lower tail latency is possible, but iteration is slower and the
  team-familiarity cost is higher for a reference project. Go is not the
  bottleneck at laptop scale.
- **Node/TypeScript**: one language end to end, but per-replica memory is
  higher and CPU-bound redirect throughput is worse.
- **Kotlin/JVM**: excellent ecosystem, but memory per replica and startup
  time work against HPA and rescheduling experiments.

## Consequences

- A monorepo Go module means a change in `pkg/` rebuilds every Go service.
  The CI path filters include `pkg/**` for that reason.
- No framework: cross-cutting concerns (RED middleware, readiness, graceful
  shutdown) live in `pkg/` and must be kept consistent by review.
