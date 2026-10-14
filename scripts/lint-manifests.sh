#!/usr/bin/env bash
# helm lint every chart, then render the umbrella chart for each env and
# validate the output with kubeconform (CRDs via the datreeio catalog).
source "$(dirname "$0")/lib.sh"
need helm kubeconform
K8S_VERSION="${K8S_VERSION:-1.34.0}"
charts="$ROOT_DIR/deploy/charts"

for c in shorty-lib links-svc redirect-svc analytics-svc web shorty; do
  helm dependency build "$charts/$c" >/dev/null 2>&1 || helm dependency update "$charts/$c" >/dev/null
  helm lint --quiet "$charts/$c" >/dev/null || die "helm lint failed: $c"
done
log "helm lint: ok"

for env in dev ha; do
  # Guard: numbers must not render in scientific notation (Helm float64 trap).
  if helm template shorty "$charts/shorty" -f "$ROOT_DIR/deploy/envs/$env/values.yaml" -f "$ROOT_DIR/deploy/envs/$env/images.yaml" \
      | grep -nE 'value: "[0-9.]+e[+-][0-9]+"'; then
    die "env $env renders a number in scientific notation (quote it or fix the template)"
  fi
  # Render with monitoring CRDs "present" so ServiceMonitors/PodMonitors are validated too.
  helm template shorty "$charts/shorty" -n shorty \
      -f "$ROOT_DIR/deploy/envs/$env/values.yaml" -f "$ROOT_DIR/deploy/envs/$env/images.yaml" \
      --api-versions monitoring.coreos.com/v1/ServiceMonitor --api-versions monitoring.coreos.com/v1/PodMonitor \
    | kubeconform -strict -summary -kubernetes-version "$K8S_VERSION" \
        -schema-location default \
        -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
    || die "kubeconform failed for env $env"
done
# Argo CD manifests (if present).
if [[ -d "$ROOT_DIR/deploy/argocd" ]] && compgen -G "$ROOT_DIR/deploy/argocd/**/*.yaml" >/dev/null; then
  find "$ROOT_DIR/deploy/argocd" -name '*.yaml' -not -path '*/bootstrap/*' -print0 | xargs -0 kubeconform -strict -summary \
    -schema-location default \
    -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'
fi
log "kubeconform: ok"
