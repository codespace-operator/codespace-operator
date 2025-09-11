#!/usr/bin/env bash
set -euo pipefail

SETUP_CONFIG="${SETUP_CONFIG:-contrib/scripts/config.sh}"
source "${SETUP_CONFIG}"

echo ">>> Install CRD..."
make install

kubectl create namespace "${NAMESPACE_KEYCLOAK}" --dry-run=client -o yaml | kubectl apply -f -

export LDAP_NAMESPACE=${LDAP_NAMESPACE:-ldap}
export LDAP_BIND_PASSWORD="$(kubectl -n "$LDAP_NAMESPACE" \
  get secret openldap -o jsonpath='{.data.LDAP_ADMIN_PASSWORD}' | base64 -d)"
echo "LDAP_BIND_PASSWORD=${LDAP_BIND_PASSWORD}"

# ----- Deploy Codespace Operator & Server via Helm -----
echo ">>> Deploying Codespace Operator & Server..."
helm upgrade --install codespace ${HELM_CHART} \
	--namespace "${NAMESPACE_SYS}" --create-namespace \
	--set operator.image.repository="${IMG%:*}" \
	--set operator.image.tag="${IMG##*:}" \
	--set server.enabled=true \
	--set server.logLevel="debug" \
	--set server.image.repository="${SERVER_IMG%:*}" \
	--set server.image.tag="${SERVER_IMG##*:}" \
	--set server.ingress.enabled=true \
	--set server.ingress.hosts[0].host="${CONSOLE_HOST}" \
	--set server.ingress.hosts[0].path="/" \
	--set-string server.auth.providers.ldap.url="${LDAP_URL}" \
	--set-string server.auth.providers.ldap.bind_dn="${LDAP_BIND_DN}" \
	--set-string server.auth.providers.ldap.bind_password="${LDAP_BIND_PASSWORD}" \
	--set-string server.auth.providers.ldap.user.base_dn="${LDAP_USER_BASE_DN}" \
	--set-string server.auth.providers.ldap.user.filter="(|(uid={username})(mail={username}))" \
	--set-string server.auth.providers.ldap.group.base_dn="${LDAP_GROUP_BASE_DN}" \
	--set-string server.auth.providers.ldap.group.filter="(member={userDN})" \
	--set-string server.auth.providers.ldap.group.attr="cn" \
	--set server.networkPolicy.enabled=false \
	--set-json server.auth.providers.ldap.roles.mapping='{"codespace-operator:admin":["admin"],"codespace-operator:editor":["editor"],"codespace-operator:viewer":["viewer"]}' \
	--set-json server.auth.providers.ldap.roles.default='["viewer"]'
	# --set server.auth.providers.oidc.scopes="{openid,profile,email}"
	# --set server.auth.providers.oidc.insecureSkipVerify=true \
	# --set server.auth.providers.oidc.issuerURL="${ISSUER_URL}" \
	# --set server.auth.providers.oidc.clientID="${OIDC_CLIENT_ID}" \
	# --set server.auth.providers.oidc.clientSecret="${OIDC_CLIENT_SECRET}" \
	# --set server.auth.providers.oidc.redirectURL="${REDIRECT_URL}" \

echo ">>> Waiting for operator + server..."
kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-operator --timeout=180s
kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-server --timeout=180s
