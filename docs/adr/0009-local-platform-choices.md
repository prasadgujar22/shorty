# 0009: Local platform choices

- Status: Accepted
- Date: 2026-10-14

Smaller decisions where the spec offered options or was silent. In each case
we picked the simplest thing that keeps the project production-shaped.

| Topic | Choice | Why |
|---|---|---|
| Postgres | **CloudNativePG** operator (not Bitnami) | Real failover and sync replication to test, managed roles, PodMonitor. Bitnami images moved behind a paywall in 2025. |
| Schemas | One DB `shorty`, schema + role per service, `REVOKE ALL ... FROM PUBLIC` | "No cross-schema access" is enforced by Postgres, not convention. Verified: `permission denied for schema analytics` as role `links`. |
| Redis | Plain Deployment from our chart, official `redis:alpine` image | Cache only (ADR 0006). An operator would add nothing. |
| NATS | Official `nats` Helm chart as a subchart | Maintained by the NATS team. JetStream clustering works out of the box. |
| Gateway | Traefik (own Helm release; k3s' bundled one disabled) | Lets us run 2 replicas, a PDB and graceful entrypoint shutdown, versioned with the rest. |
| Log shipping | **Grafana Alloy** (not Promtail) | Promtail reached end of life in 2026. Alloy is its supported successor. |
| Packaging | Library chart `shorty-lib` + thin per-service charts + umbrella | Per-service charts as the spec asks, with one implementation of probes, PDB, HPA and hardening. |
| Env overlays | Umbrella chart values files (`deploy/envs/<env>/values.yaml` + `images.yaml`) | One mechanism (Helm) instead of Helm + Kustomize. `images.yaml` keeps CI's commits tiny and conflict-free. |
| Local images | `make build` builds host-arch only into the k3d registry (`127.0.0.1:5001`) | Multi-arch builds are CI's job (buildx). `127.0.0.1` avoids IPv6 `localhost` resolution problems with Docker Desktop. |
| Go toolchain | Go 1.27 | Container-aware GOMAXPROCS, `wg.Go`, `testing` improvements. |
| Web toolchain | SvelteKit 3 (config in `vite.config.ts`), Svelte 5, Vite 8, **Node 24 LTS** | Node 25 (non-LTS) breaks the SvelteKit build (`util.styleText('grey')` was removed). |
| Web base image | `nginx-unprivileged:1.30-alpine-slim` | 0 fixable HIGH/CRITICAL CVEs vs 9 in the full alpine variant (curl, c-ares), 82 MB to 31 MB. |
| Argo CD | Inside the k3d cluster (a docker-desktop Argo CD exists on the dev machine but is left untouched) | Self-contained and reproducible with `make bootstrap gitops`. Confirmed with the project owner. |
