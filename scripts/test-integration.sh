#!/usr/bin/env bash
# Run Go integration tests (-tags integration) against throwaway Postgres,
# Redis and NATS JetStream containers. KEEP=1 leaves the containers running.
source "$(dirname "$0")/lib.sh"
need docker go

compose=(docker compose -f "$ROOT_DIR/deploy/compose/test.yml")
if [[ "${KEEP:-0}" != 1 ]]; then
  trap '"${compose[@]}" down -v --remove-orphans >/dev/null 2>&1' EXIT
fi

log "starting test dependencies"
"${compose[@]}" up -d --wait --quiet-pull

export TEST_DATABASE_URL="postgres://shorty:shorty-test-only@127.0.0.1:55432/shorty?sslmode=disable"
export TEST_REDIS_ADDR="127.0.0.1:56379"
export TEST_NATS_URL="nats://127.0.0.1:54222"

log "running integration tests"
cd "$ROOT_DIR"
go test -tags integration -race -count=1 -covermode=atomic -coverprofile=coverage-integration.out "$@" ./...
