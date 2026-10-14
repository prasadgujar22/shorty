#!/usr/bin/env bash
# The tuning experiments behind docs/TUNING.md. Each line is one run of
# load/tune.sh against the local `shorty` namespace (single replica of each
# service), with k6 in-cluster. Results append to load/results/tuning.jsonl.
#   TAG=<image tag in 127.0.0.1:5001> load/experiments.sh [group...]
set -euo pipefail
T="$(dirname "$0")/tune.sh"
groups="${*:-cpu memory pool cache publish consumer}"

for g in $groups; do case $g in
cpu) # GOMAXPROCS vs CPU quota. Go >= 1.25 sets GOMAXPROCS from the cgroup limit
     # but never below 2. A redirect costs ~0.1 ms of CPU, so limits must sit
     # near the real demand (~0.17 cores at 1500 rps) to matter.
  $T cpu-150m-auto   redirect.js "RATE=1500 DURATION=60s LINKS=500" --set redirect-svc.resources.limits.cpu=150m
  $T cpu-150m-procs1 redirect.js "RATE=1500 DURATION=60s LINKS=500" --set redirect-svc.resources.limits.cpu=150m --set redirect-svc.env.GOMAXPROCS=1
  $T cpu-250m-auto   redirect.js "RATE=1500 DURATION=60s LINKS=500" --set redirect-svc.resources.limits.cpu=250m
  $T cpu-250m-procs1 redirect.js "RATE=1500 DURATION=60s LINKS=500" --set redirect-svc.resources.limits.cpu=250m --set redirect-svc.env.GOMAXPROCS=1
  $T cpu-1-auto      redirect.js "RATE=1500 DURATION=60s LINKS=500" --set redirect-svc.resources.limits.cpu=1
  ;;
memory) # GOMEMLIMIT vs OOM: NATS unreachable -> click buffer grows in memory
  $T mem-32Mi-nolimit redirect.js "RATE=1500 DURATION=60s LINKS=500" --set redirect-svc.resources.limits.memory=32Mi --set redirect-svc.resources.requests.memory=24Mi \
     --set redirect-svc.env.NATS_URL=nats://unreachable:4222 --set redirect-svc.env.CLICK_BUFFER_SIZE=150000 --set redirect-svc.env.GOMEMLIMIT=off
  $T mem-32Mi-gomemlimit redirect.js "RATE=1500 DURATION=60s LINKS=500" --set redirect-svc.resources.limits.memory=32Mi --set redirect-svc.resources.requests.memory=24Mi \
     --set redirect-svc.env.NATS_URL=nats://unreachable:4222 --set redirect-svc.env.CLICK_BUFFER_SIZE=150000
  ;;
pool) # links-svc Postgres pool size under a read-heavy API load
  for n in 2 10 40; do
    $T "pool-$n" links.js "RATE=1200 DURATION=60s WRITE_SHARE=0.1" --set links-svc.env.DB_MAX_CONNS=$n --set links-svc.env.DB_MIN_CONNS=$((n < 2 ? n : 2)) --set links-svc.resources.limits.cpu=1
  done
  ;;
cache) # cache layers: none (TTL 1s, no L1) vs Redis only vs L1 + Redis
  $T cache-none   redirect.js "RATE=1500 DURATION=60s LINKS=2000" --set redirect-svc.env.L1_CACHE_SIZE=0 --set redirect-svc.env.CACHE_TTL=1s
  $T cache-redis  redirect.js "RATE=1500 DURATION=60s LINKS=2000" --set redirect-svc.env.L1_CACHE_SIZE=0
  $T cache-l1     redirect.js "RATE=1500 DURATION=60s LINKS=2000"
  ;;
publish) # JetStream publisher concurrency (sync publish = 1 RTT per event per worker)
  for n in 1 4 16; do
    $T "publish-$n" redirect.js "RATE=2000 DURATION=60s LINKS=500" --set redirect-svc.env.CLICK_PUBLISH_WORKERS=$n
  done
  ;;
consumer) # analytics pull batch size (1 transaction per batch)
  for n in 1 50 500; do
    $T "batch-$n" redirect.js "RATE=1500 DURATION=60s LINKS=500" --set analytics-svc.env.CONSUMER_BATCH_SIZE=$n
  done
  ;;
esac; done
