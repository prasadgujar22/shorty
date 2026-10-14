#!/usr/bin/env bash
# Same as kill-pods.sh but SIGKILL: no preStop, no drain, no buffer flush.
FORCE=1 exec "$(dirname "$0")/kill-pods.sh"
