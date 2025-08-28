#!/usr/bin/env bash
set -euo pipefail

SETUP_CONFIG="${SETUP_CONFIG:-misc/tests/config.sh}"
source "${SETUP_CONFIG}"

echo ">>> Install CRD..."
make install

kubectl create namespace "${NAMESPACE_KEYCLOAK}" --dry-run=client -o yaml | kubectl apply -f -

# ----- Prepare Keycloak realm ConfigMap -----
echo ">>> Rendering Keycloak realm (redirect=${REDIRECT_URL})..."
rendered_realm="$(mktemp)"
sed -e "s#__REDIRECT_URL__#${REDIRECT_URL}#g" \
	-e "s#__CLIENT_SECRET__#${OIDC_CLIENT_SECRET}#g" \
	"${KEYCLOAK_REALM_TMPL}" >"${rendered_realm}"

kubectl -n "${NAMESPACE_KEYCLOAK}" create configmap keycloak-realm \
	--from-file=realm.json="${rendered_realm}" \
	--dry-run=client -o yaml | kubectl apply -f -

# ----- Admin creds (dev only) -----
kubectl -n "${NAMESPACE_KEYCLOAK}" apply -f - <<'YAML'
apiVersion: v1
kind: Secret
metadata:
  name: keycloak-admin
type: Opaque
stringData:
  KEYCLOAK_ADMIN: admin
  KEYCLOAK_ADMIN_PASSWORD: admin
YAML

# ----- Deploy Keycloak via codecentric/keycloakx -----
echo ">>> Deploying Keycloak (codecentric/keycloakx)..."
helm repo add codecentric https://codecentric.github.io/helm-charts >/dev/null
helm repo update >/dev/null

rendered_vals="$(mktemp)"
sed -e "s#__HOST_DOMAIN__#${HOST_DOMAIN}#g" \
	-e "s#__HOSTNAME_URL__#${HOSTNAME_URL}#g" \
	"${KEYCLOAK_VALUES_TMPL}" >"${rendered_vals}"

helm upgrade --install keycloak codecentric/keycloakx \
	--namespace "${NAMESPACE_KEYCLOAK}" --create-namespace \
	-f "${rendered_vals}"

echo ">>> Waiting for Keycloak to be Ready..."
# Prefer Deployment availability; fallback to any pod readiness for this release
if ! kubectl -n "${NAMESPACE_KEYCLOAK}" wait --for=condition=available --timeout=300s deploy -l "app.kubernetes.io/instance=keycloak"; then
	kubectl -n "${NAMESPACE_KEYCLOAK}" wait --for=condition=ready --timeout=300s pod -l "app.kubernetes.io/instance=keycloak"
fi

# ----- Deploy Codespace Operator & Server via Helm -----
echo ">>> Deploying Codespace Operator & Server..."
helm upgrade --install codespace-operator ${HELM_CHART} \
	--namespace "${NAMESPACE_SYS}" --create-namespace \
	--set image.repository="${IMG%:*}" \
	--set image.tag="${IMG##*:}" \
	--set server.enabled=true \
	--set server.image.repository="${SERVER_IMG%:*}" \
	--set server.image.tag="${SERVER_IMG##*:}" \
	--set server.ingress.enabled=true \
	--set server.ingress.hosts[0].host="${CONSOLE_HOST}" \
	--set server.ingress.hosts[0].path="/" \
	--set server.enableLocalLogin=true \
	--set server.oidc.insecureSkipVerify=true \
	--set server.oidc.issuer="${ISSUER}" \
	--set server.oidc.clientID="${OIDC_CLIENT_ID}" \
	--set server.oidc.clientSecret="${OIDC_CLIENT_SECRET}" \
	--set server.oidc.redirectURL="${REDIRECT_URL}" \
	--set server.oidc.scopes="{openid,profile,email}"

echo ">>> Waiting for operator + server..."
kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-operator --timeout=180s
kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-operator-server --timeout=180s
