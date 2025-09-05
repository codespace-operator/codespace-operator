#!/usr/bin/env bash
set -euo pipefail

: "${SETUP_CONFIG:=misc/tests/config.sh}"
source "${SETUP_CONFIG}"

need() { command -v "$1" >/dev/null || { echo "Missing '$1' in PATH"; exit 1; }; }
need kubectl; need helm

LDAP_NAMESPACE="${LDAP_NAMESPACE:-ldap}"
LDAP_ADMIN_PASSWORD="${LDAP_ADMIN_PASSWORD:-admin}"
LDAP_CONFIG_PASSWORD="${LDAP_CONFIG_PASSWORD:-admin}"

echo ">>> Namespace '${LDAP_NAMESPACE}'..."
kubectl get ns "${LDAP_NAMESPACE}" >/dev/null 2>&1 || kubectl create ns "${LDAP_NAMESPACE}"

echo ">>> Add jp-gouin/openldap chart repo..."
helm repo add helm-openldap https://jp-gouin.github.io/helm-openldap/ >/dev/null
helm repo update >/dev/null

echo ">>> Install openldap-stack-ha (single replica)..."
helm upgrade --install openldap helm-openldap/openldap-stack-ha \
  --version 3.0.2 \
  -n "${LDAP_NAMESPACE}" --create-namespace \
  -f misc/tests/manifests/openldap-values.yaml


echo ">>> Wait for OpenLDAP to be Ready..."
# Try Deployment first, fall back to StatefulSet label selector
if ! kubectl -n "${LDAP_NAMESPACE}" wait --for=condition=available --timeout=300s deploy -l app.kubernetes.io/name=openldap 2>/dev/null; then
  kubectl -n "${LDAP_NAMESPACE}" rollout status statefulset -l app.kubernetes.io/name=openldap --timeout=300s
fi

echo ">>> OpenLDAP service: openldap.${LDAP_NAMESPACE}.svc.cluster.local:389"
echo "    Base DN: dc=codespace,dc=test"
echo "    Bind DN: cn=admin,dc=codespace,dc=test"
echo "    Users:   admin/admin, alice/alice, bob/bob"
