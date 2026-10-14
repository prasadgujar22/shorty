#!/usr/bin/env bash
# Delete the k3d cluster and its registry. All data in the cluster is lost.
source "$(dirname "$0")/lib.sh"
need k3d
if cluster_exists; then
  log "deleting cluster '${CLUSTER_NAME}'"
  k3d cluster delete "$CLUSTER_NAME"
else
  log "cluster '${CLUSTER_NAME}' does not exist"
fi
# The registry created from the cluster config is deleted with the cluster;
# remove it explicitly in case it was left behind by an interrupted run.
k3d registry delete "shorty-registry" >/dev/null 2>&1 || true
