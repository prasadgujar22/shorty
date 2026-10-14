#!/usr/bin/env bash
# Verify (and on macOS, install via Homebrew) the CLI tools Shorty needs.
source "$(dirname "$0")/lib.sh"

required=(docker k3d kubectl helm go node npm k6 golangci-lint kubeconform trivy gitleaks argocd yq jq)
brew_pkg() { case "$1" in kubectl) echo kubernetes-cli ;; npm) echo node ;; *) echo "$1" ;; esac; }

missing=()
for bin in "${required[@]}"; do
  command -v "$bin" >/dev/null 2>&1 || missing+=("$bin")
done

if [[ ${#missing[@]} -gt 0 ]]; then
  if [[ "$(uname)" == "Darwin" ]] && command -v brew >/dev/null; then
    pkgs=""
    for bin in "${missing[@]}"; do
      [[ "$bin" == docker ]] && die "Docker Desktop (or OrbStack) must be installed manually"
      pkgs="$pkgs $(brew_pkg "$bin")"
    done
    log "installing:$pkgs"
    # shellcheck disable=SC2086
    brew install $pkgs
  else
    die "missing tools: ${missing[*]}"
  fi
fi

docker info >/dev/null 2>&1 || die "Docker daemon is not running"
mem_gib=$(( $(docker info --format '{{.MemTotal}}') / 1024 / 1024 / 1024 ))
if (( mem_gib < 10 )); then
  warn "Docker has ${mem_gib} GiB; 12 GiB is recommended for the full stack plus load tests"
fi
log "tools OK (docker ${mem_gib} GiB, $(go version | awk '{print $3}'), helm $(helm version --short))"
