# 0003: Pull-based GitOps with Argo CD

- Status: Accepted
- Date: 2026-10-14

## Context

The cluster runs on a laptop behind NAT, with no public IP and no inbound
access. CI runs on GitHub-hosted runners. We want deployments to be
auditable, reversible and identical whether triggered by CI or a human.

## Decision

**Pull-based** CD. CI builds images, pushes them to GHCR, and commits the new
tag into `deploy/envs/dev/images.yaml`. Argo CD inside the cluster polls Git
every 60s and reconciles. `dev` auto-syncs, with prune and self-heal. `ha`
syncs manually after a release tag pins its tags. Argo CD uses app-of-apps
(`deploy/argocd/root.yaml` → `deploy/argocd/apps/`).

## Alternatives

- **Push from CI** (`helm upgrade` in a workflow): needs inbound access to
  the cluster API (a tunnel, a self-hosted runner, or an exposed kube-apiserver)
  and long-lived cluster-admin credentials in GitHub. It also means drift
  from manual `kubectl` changes goes unnoticed, and "what's deployed?"
  becomes a question for CI logs instead of Git.
- **Self-hosted runner on the laptop doing push**: avoids inbound access but
  ties CI to the laptop being on and gives CI cluster credentials.
- **Flux**: equally valid pull-based tool. Argo CD was chosen for its UI
  (useful while running chaos experiments) and the app-of-apps pattern the
  spec asks for.
- **Argo CD Image Updater** instead of the CI commit: it needs registry and
  Git write credentials inside the cluster. Kept optional and off.

## Consequences

- Deploy latency is up to one poll interval (60s) after CI's commit. A manual
  "Refresh" makes it immediate. Webhooks are impossible without inbound access.
- Every deploy and rollback is a Git commit (`chore(gitops): ...`), so `git
  log deploy/envs` is the deployment history.
- CI needs only `contents: write` on its own repo and `packages: write`. It has
  no cluster credentials.
- Concurrent image bumps race on `images.yaml`. They are serialised with a
  `concurrency: gitops-dev` group and a rebase-and-retry loop.
