#!/usr/bin/env bash
set -euo pipefail

: "${SETUP_CONFIG:=misc/tests/config.sh}"
source "${SETUP_CONFIG}"

need() { command -v "$1" >/dev/null || { echo "Missing '$1' in PATH"; exit 1; }; }
need kubectl; need helm

LDAP_NAMESPACE="${LDAP_NAMESPACE:-ldap}"
LDAP_ADMIN_PASSWORD="${LDAP_ADMIN_PASSWORD:-admin}"

echo ">>> Namespace '${LDAP_NAMESPACE}'..."
kubectl get ns "${LDAP_NAMESPACE}" >/dev/null 2>&1 || kubectl create ns "${LDAP_NAMESPACE}"

echo ">>> Bootstrap LDIF ConfigMap..."
kubectl -n "${LDAP_NAMESPACE}" create configmap openldap-bootstrap \
  --from-file=bootstrap.ldif="misc/tests/manifests/bootstrap.ldif" \
  --dry-run=client -o yaml | kubectl apply -f -

echo ">>> Helm repo bitnami..."
helm repo add bitnami https://charts.bitnami.com/bitnami >/dev/null
helm repo update >/dev/null

echo ">>> Install OpenLDAP..."
helm upgrade --install openldap bitnami/openldap \
  --namespace "${LDAP_NAMESPACE}" \
  -f misc/tests/manifests/openldap-values.yaml \
  --set auth.rootPassword="${LDAP_ADMIN_PASSWORD}"

echo ">>> Wait for OpenLDAP..."
kubectl -n "${LDAP_NAMESPACE}" rollout status statefulset/openldap --timeout=180s

echo ">>> OpenLDAP is up (svc: openldap.${LDAP_NAMESPACE}.svc.cluster.local:389)"
