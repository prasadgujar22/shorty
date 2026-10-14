## What

<!-- One or two sentences. Title must follow Conventional Commits, e.g. `feat(redirect-svc): add negative cache`. -->

## Why

## How was it verified

- [ ] `make test` passes
- [ ] `make lint` passes
- [ ] Deployed with `make up` and smoke-tested (`make smoke`)
- [ ] Load/chaos numbers updated in `docs/RESULTS.md` (if it touches the hot path)

## Checklist

- [ ] No secrets, tokens or real passwords committed
- [ ] Helm values for `dev` and `ha` updated if config changed
- [ ] ADR added in `docs/adr/` for hard-to-reverse decisions
