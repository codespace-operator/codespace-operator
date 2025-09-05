#!/usr/bin/env bash
set -euo pipefail

SETUP_CONFIG="${SETUP_CONFIG:-misc/tests/config.sh}"
source "${SETUP_CONFIG}"

echo ">>> Install CRD..."
make install

kubectl create namespace "${NAMESPACE_KEYCLOAK}" --dry-run=client -o yaml | kubectl apply -f -


# ----- Deploy Codespace Operator & Server via Helm -----
echo ">>> Deploying Codespace Operator & Server..."
helm upgrade --install codespace ${HELM_CHART} \
	--namespace "${NAMESPACE_SYS}" --create-namespace \
	--set operator.image.repository="${IMG%:*}" \
	--set operator.image.tag="${IMG##*:}" \
	--set server.enabled=true \
	--set server.image.repository="${SERVER_IMG%:*}" \
	--set server.image.tag="${SERVER_IMG##*:}" \
	--set server.ingress.enabled=true \
	--set server.ingress.hosts[0].host="${CONSOLE_HOST}" \
	--set server.ingress.hosts[0].path="/" \
	--set server.auth.ldap.enabled=true \
	--set server.auth.ldap.bindPassword='admin' \
	--set server.auth.enableLocalLogin=true
	# --set server.oidc.scopes="{openid,profile,email}"
	# --set server.oidc.insecureSkipVerify=true \
	# --set server.oidc.issuerURL="${ISSUER_URL}" \
	# --set server.oidc.clientID="${OIDC_CLIENT_ID}" \
	# --set server.oidc.clientSecret="${OIDC_CLIENT_SECRET}" \
	# --set server.oidc.redirectURL="${REDIRECT_URL}" \

echo ">>> Waiting for operator + server..."
kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-operator --timeout=180s
kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-server --timeout=180s
