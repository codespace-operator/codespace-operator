#!/usr/bin/env bash
: "${SETUP_CONFIG:=contrib/scripts/config.sh}"
source "${SETUP_CONFIG}"

need() { command -v "$1" >/dev/null || { echo "Missing '$1' in PATH"; exit 1; }; }
need kubectl; need helm
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

# Minimal PoC values; use in-memory or local storage for now
cat > loki-values.yaml <<'YAML'
loki:
  auth_enabled: true

gateway:
  enabled: true
  service:
    type: ClusterIP

# optional: expose a /loki endpoint via Ingress (we'll protect it below)
ingress:
  enabled: true
  ingressClassName: nginx
  hosts:
    - host: loki.codespace.test
      paths:
        - path: /
          pathType: Prefix
  tls: []  # off for PoC since you're using http
YAML

helm upgrade --install loki grafana/loki-distributed -n monitoring --create-namespace -f loki-values.yaml
