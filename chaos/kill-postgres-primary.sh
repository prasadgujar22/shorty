#!/usr/bin/env bash
# SIGKILL the CloudNativePG primary; the operator promotes the synchronous
# standby. links-svc writes and analytics batches fail until the -rw Service
# points at the new primary, then retry.
source "$(dirname "$0")/lib.sh"
p=$(pg_primary)
say "SIGKILL postgres primary $p"
k delete pod "$p" --grace-period=0 --force --wait=false >/dev/null 2>&1
