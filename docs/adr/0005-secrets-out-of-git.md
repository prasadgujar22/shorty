# 0005: Secrets are created out of band, never committed

- Status: Accepted
- Date: 2026-10-14

## Context

links-svc and analytics-svc each need a Postgres role password, consumed both
by the service (env from a Secret) and by CloudNativePG managed roles
(`passwordSecret`). With GitOps, everything else comes from Git. Helm
`lookup`-based random generation does not work under Argo CD, which renders
with `helm template`, so passwords would change on every sync.

## Decision

`scripts/secrets.sh <namespace>` creates `shorty-db-links` and
`shorty-db-analytics` (type `kubernetes.io/basic-auth`, 192-bit random
passwords) if they don't exist. `make deploy` and `make gitops` call it for
`shorty`, `shorty-dev` and `shorty-ha`. The charts only reference Secrets by
name. gitleaks runs in CI and fails on committed credentials. The only
credential-like strings in Git are the throwaway `*-local-only` and
`ci-only-password` values used by docker-compose and CI service containers,
which are allowlisted in `.gitleaks.toml`.

## Production path

Replace the script with **External Secrets Operator** (secrets in AWS/GCP
secret manager or Vault, synced into the namespace) or **Sealed Secrets**
(encrypted Secret manifests in Git). Neither changes the charts.

## Consequences

- A fresh cluster needs `make gitops` (or the script) before the first Argo
  CD sync. The `shorty-*` apps retry with backoff, so ordering mistakes heal
  once the Secrets exist.
- Rotating a password means updating the Secret. CNPG reconciles the role
  (`cnpg.io/reload` label), then restart the service Deployment.
