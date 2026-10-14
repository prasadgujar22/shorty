#!/usr/bin/env bash
# shortyctl: stop, start and inspect the whole local Shorty setup
# (k3d cluster + registry, platform, observability, Argo CD, and the
# shorty / shorty-dev / shorty-ha environments).
#
#   scripts/shortyctl.sh start      resume a stopped cluster (state kept), or
#                                   build everything from scratch if none exists
#   scripts/shortyctl.sh stop       pause the cluster: frees CPU/RAM, keeps all data
#   scripts/shortyctl.sh restart    stop + start
#   scripts/shortyctl.sh status     nodes, Argo CD apps, pods, URLs
#   scripts/shortyctl.sh destroy    delete the cluster and registry (asks first; -y to skip)
#
# Env: SKIP_SMOKE=1 to skip the end-to-end checks after start.
source "$(dirname "$0")/lib.sh"
need docker k3d kubectl

REGISTRY_CONTAINER="shorty-registry"
ENVS=("http://localhost:8088" "http://dev.localhost:8088" "http://ha.localhost:8088")

cluster_running() {
  local out; out="$(k3d cluster list -o json 2>/dev/null || true)"
  [[ "$out" == *"\"name\":\"${CLUSTER_NAME}\""* ]] && [[ "$(docker inspect -f '{{.State.Running}}' "k3d-${CLUSTER_NAME}-server-0" 2>/dev/null)" == true ]]
}

# Node web builds need an LTS Node (see services/web/.nvmrc).
[[ -d /opt/homebrew/opt/node@24/bin ]] && export PATH="/opt/homebrew/opt/node@24/bin:$PATH"

wait_healthy() {
  log "waiting for nodes"
  kc wait --for=condition=Ready nodes --all --timeout=300s >/dev/null

  log "waiting for all pods to be Running and ready (up to 10 min)"
  local bad=""
  for _ in $(seq 1 120); do
    bad=$(kc get pods -A --no-headers 2>/dev/null \
      | awk '{split($3,a,"/"); if ($4!="Completed" && ($4!="Running" || a[1]!=a[2])) print $1"/"$2" "$4}')
    [[ -z "$bad" ]] && break
    sleep 5
  done
  [[ -z "$bad" ]] || { warn "still not ready:"; echo "$bad" | sed 's/^/    /' >&2; }

  if kc get ns argocd >/dev/null 2>&1; then
    # After a resume, app health is cached from the moment pods were down;
    # ask Argo CD to re-evaluate every app instead of waiting for its timer.
    for app in $(kc -n argocd get applications -o name 2>/dev/null); do
      kc -n argocd annotate "$app" argocd.argoproj.io/refresh=normal --overwrite >/dev/null 2>&1 || true
    done
    log "waiting for Argo CD apps to be Synced/Healthy"
    # Up to 10 min: after a resume Prometheus replays its WAL, and until it
    # has fresh samples the redirect-svc HPA's RPS metric is unavailable, so
    # Argo CD reports shorty-ha as Degraded (the HPA keeps scaling on CPU).
    local apps=""
    for _ in $(seq 1 120); do
      apps=$(kc -n argocd get applications --no-headers 2>/dev/null | awk '$2!="Synced" || $3!="Healthy" {print $1" "$2"/"$3}')
      [[ -z "$apps" ]] && break
      sleep 5
    done
    [[ -z "$apps" ]] || { warn "Argo CD apps not settled yet (usually the HPA waiting for Prometheus; clears on its own):"; echo "$apps" | sed 's/^/    /' >&2; }
  fi
}

sync_ha() {
  # shorty-ha is synced manually by design: sync only if it is not deployed
  # or out of sync (a fresh cluster), never just because it is warming up.
  local s; s=$(kc -n argocd get application shorty-ha -o jsonpath='{.status.sync.status}/{.status.health.status}' 2>/dev/null || true)
  [[ -z "$s" || "$s" == Synced/* ]] && return 0
  log "syncing shorty-ha (manual-sync app, currently $s)"
  kc -n argocd patch application shorty-ha --type merge \
    -p '{"operation":{"initiatedBy":{"username":"shortyctl"},"sync":{"revision":"main"}}}' >/dev/null
  for _ in $(seq 1 90); do
    s=$(kc -n argocd get application shorty-ha -o jsonpath='{.status.sync.status}/{.status.health.status}')
    [[ "$s" == "Synced/Healthy" ]] && return 0
    sleep 5
  done
  warn "shorty-ha is $s"
}

smoke_all() {
  [[ "${SKIP_SMOKE:-0}" == 1 ]] && return 0
  local failed=0
  for url in "${ENVS[@]}"; do
    if BASE_URL="$url" "$ROOT_DIR/scripts/smoke.sh" >/dev/null 2>&1; then
      printf '  \033[1mOK\033[0m   %s\n' "$url"
    else
      printf '  \033[1mFAIL\033[0m %s  (run: BASE_URL=%s make smoke)\n' "$url" "$url"; failed=1
    fi
  done
  return $failed
}

print_urls() {
  cat <<EOF

  Shorty (local)   http://localhost:8088
  Shorty (dev)     http://dev.localhost:8088
  Shorty (ha)      http://ha.localhost:8088
  Grafana          http://grafana.localhost:8088
  Prometheus       http://prometheus.localhost:8088
  Argo CD          http://argocd.localhost:8088   (admin / $(kc -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' 2>/dev/null | base64 -d 2>/dev/null || echo '<n/a>'))
EOF
}

cmd_start() {
  if cluster_exists; then
    if cluster_running; then
      log "cluster '${CLUSTER_NAME}' is already running"
    else
      log "starting registry and cluster '${CLUSTER_NAME}' (state is preserved)"
      docker start "$REGISTRY_CONTAINER" >/dev/null 2>&1 || true
      # k3d's post-start step (patching /etc/hosts in every node) can race
      # with agents that restart until the server is up; it is idempotent,
      # so retry instead of failing.
      local ok=false
      for attempt in 1 2 3 4 5; do
        if k3d cluster start "$CLUSTER_NAME" >/dev/null 2>&1; then ok=true; break; fi
        warn "k3d cluster start attempt $attempt hit a startup race; retrying"
        sleep 10
      done
      $ok || cluster_running || die "could not start cluster '${CLUSTER_NAME}' (see: k3d cluster start ${CLUSTER_NAME})"
    fi
    kubectl config use-context "$KUBE_CONTEXT" >/dev/null
    wait_healthy
    sync_ha
  else
    log "no cluster found: building everything from scratch (~20-30 min)"
    ( cd "$ROOT_DIR" && make bootstrap && make up && make gitops )
    wait_healthy
    sync_ha
  fi
  log "smoke tests"
  smoke_all || warn "some smoke tests failed (pods may still be warming up; retry in a minute)"
  print_urls
}

cmd_stop() {
  if ! cluster_exists; then log "cluster '${CLUSTER_NAME}' does not exist; nothing to stop"; return 0; fi
  # Stop any load/chaos still running against the cluster.
  pkill -f 'k6 run' 2>/dev/null || true
  log "stopping cluster '${CLUSTER_NAME}' (all state is kept on Docker volumes)"
  k3d cluster stop "$CLUSTER_NAME" >/dev/null
  docker stop "$REGISTRY_CONTAINER" >/dev/null 2>&1 || true
  log "stopped. Ports 8088/5001 are free; run '$0 start' to resume."
}

cmd_status() {
  if ! cluster_exists; then log "cluster '${CLUSTER_NAME}': not created"; return 0; fi
  if ! cluster_running; then log "cluster '${CLUSTER_NAME}': stopped"; return 0; fi
  log "cluster '${CLUSTER_NAME}': running"
  kc get nodes -L topology.kubernetes.io/zone
  if kc get ns argocd >/dev/null 2>&1; then echo; kc -n argocd get applications; fi
  echo
  local total notready
  total=$(kc get pods -A --no-headers | wc -l | tr -d ' ')
  notready=$(kc get pods -A --no-headers | awk '{split($3,a,"/"); if ($4!="Completed" && ($4!="Running" || a[1]!=a[2])) print}' | wc -l | tr -d ' ')
  echo "pods: $total total, $notready not ready"
  for ns in shorty shorty-dev shorty-ha; do
    kc get ns "$ns" >/dev/null 2>&1 || continue
    printf '  %-11s %s\n' "$ns" "$(kc -n "$ns" get deploy redirect-svc -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null)"
  done
  print_urls
}

cmd_destroy() {
  if [[ "${1:-}" != "-y" ]]; then
    read -r -p "Delete cluster '${CLUSTER_NAME}' and ALL its data (links, clicks, dashboards)? [y/N] " a
    [[ "$a" == [yY]* ]] || { log "aborted"; return 0; }
  fi
  pkill -f 'k6 run' 2>/dev/null || true
  "$ROOT_DIR/scripts/cluster-down.sh"
}

case "${1:-}" in
  start)   cmd_start ;;
  stop)    cmd_stop ;;
  restart) cmd_stop; cmd_start ;;
  status)  cmd_status ;;
  destroy) shift; cmd_destroy "${1:-}" ;;
  *) sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
