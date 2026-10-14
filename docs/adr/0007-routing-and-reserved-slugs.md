# 0007: Gateway routing, reserved slugs, host port 8088

- Status: Accepted
- Date: 2026-10-14

## Context

The spec routes `/` to web and `/{slug}` to redirect-svc. These overlap: the
UI needs its own routes (assets, the link-detail page), and every single
path segment is also a valid slug.

## Decision

1. **Traefik IngressRoute with explicit priorities** (not rule length):
   `/api/links*` and `/api/stats*` (100) > UI paths `/`, `/_app/*`, `/s/*`,
   `/favicon.svg`, `/robots.txt` (90) > catch-all `/` to redirect-svc (1).
   Host-specific environments add 1000, so `dev.localhost` and `ha.localhost`
   always beat the host-less local route.
2. The link-detail page lives at **`/s/{slug}`**. The UI is an SPA, and
   Nginx serves `index.html` for unknown paths it receives.
3. **Reserved slugs** (`pkg/slug`): `api`, `s`, `_app`, `app`, `stats`,
   `static`, `assets`, `fonts`, `healthz`, `readyz`, `livez`, `metrics`,
   `favicon.ico`, `favicon.svg`, `robots.txt`, `index.html`, `admin`,
   `login`, `logout`, `new`. redirect-svc rejects reserved and malformed
   slugs before touching Redis, which also keeps scanner traffic off
   links-svc.
4. **Ops endpoints on a separate admin port** (`:9090`). The catch-all route
   would otherwise expose `/metrics` and `/healthz` of redirect-svc to the
   internet.
5. **Host port 8088** for the k3d load balancer instead of 8080. On the
   development machine, 8080 was already taken by Jenkins and a kubectl
   port-forward, and those collide in confusing ways (IPv4 vs IPv6
   localhost). `make` targets take `BASE_URL`, so this is a single setting.

## Consequences

- Adding a UI route means adding it to the IngressRoute *and* reserving the
  first path segment in `pkg/slug`.
- Slug rules are 3–32 chars, `[A-Za-z0-9][A-Za-z0-9_-]*`, case-sensitive.
  Generated slugs are 7 chars from a 56-symbol alphabet without ambiguous
  characters (0/O, 1/l/I).
