#!/usr/bin/env bash
# Build one service image and push it to the local k3d registry.
#   scripts/build-image.sh <service>     (TAG, REGISTRY, PLATFORMS from env)
# Multi-arch (linux/arm64,linux/amd64) builds are done by CI with buildx;
# locally we build the host architecture only, which is all k3d needs.
source "$(dirname "$0")/lib.sh"
need docker
svc="$1"
tag="${TAG:-dev}"
registry="${REGISTRY:-$REGISTRY_HOST}"
image="${registry}/shorty-${svc}:${tag}"
[[ -f "$ROOT_DIR/services/$svc/Dockerfile" ]] || die "unknown service: $svc"

log "build $image"
DOCKER_BUILDKIT=1 docker build -q \
  -f "$ROOT_DIR/services/$svc/Dockerfile" \
  --build-arg VERSION="$tag" \
  --build-arg COMMIT="$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || echo unknown)" \
  -t "$image" "$ROOT_DIR" >/dev/null
docker push -q "$image" >/dev/null
log "pushed $image"
