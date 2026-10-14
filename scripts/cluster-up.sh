#!/usr/bin/env bash
# Create the k3d cluster (1 server + 3 agents) and its local registry.
# Idempotent: re-running on an existing cluster just starts it.
source "$(dirname "$0")/lib.sh"
need k3d kubectl docker

if cluster_exists; then
  log "cluster '${CLUSTER_NAME}' exists, making sure it is started"
  k3d cluster start "$CLUSTER_NAME" >/dev/null
else
  log "creating k3d cluster '${CLUSTER_NAME}' (1 server + 3 agents) with registry ${REGISTRY_HOST}"
  k3d cluster create --config "$ROOT_DIR/deploy/k3d/cluster.yaml"
fi

log "waiting for nodes"
kc wait --for=condition=Ready nodes --all --timeout=180s
kc get nodes -L topology.kubernetes.io/zone

log "waiting for core addons (coredns, metrics-server, local-path)"
for d in coredns metrics-server local-path-provisioner; do
  kc -n kube-system rollout status "deploy/$d" --timeout=180s
done
log "cluster ready: context ${KUBE_CONTEXT}, registry ${REGISTRY_HOST}, ingress http://localhost:8080"
