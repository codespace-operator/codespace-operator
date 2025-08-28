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
# Replace the manual Session creation with API calls
if [[ ${APPLY_DEMO} == "true" ]]; then
	echo ">>> Creating demo session via API..."
	HOST="${DEMO_NAME}.${HOST_DOMAIN}"

	# Wait for server to be accessible
	echo "Waiting for server API to be ready..."
	for i in {1..30}; do
		if curl -s -o /dev/null -w "%{http_code}" "http://console.${HOST_DOMAIN}/healthz" | grep -q "200"; then
			break
		fi
		echo "Waiting for API... (attempt $i/30)"
		sleep 5
	done

	# Login and get session token
  # In deploy.sh, replace the login section with:
  echo "Logging in with bootstrap credentials..."
  login_response=$(curl -s -w "HTTP_STATUS:%{http_code}" -X POST "http://console.${HOST_DOMAIN}/auth/local/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin"}')

  http_status=$(echo "$login_response" | grep -o "HTTP_STATUS:[0-9]*" | cut -d: -f2)
  response_body=$(echo "$login_response" | sed 's/HTTP_STATUS:[0-9]*$//')

  if [[ "$http_status" != "200" ]]; then
    echo "Login failed with HTTP $http_status"
    echo "Response: $response_body"
    echo "Checking auth features..."
    curl -s "http://console.${HOST_DOMAIN}/auth/features" | jq .
    exit 1
  fi

  token=$(echo "$response_body" | jq -r '.token // empty')
  if [[ -z "$token" ]]; then
    echo "No token in response: $response_body"
    exit 1
  fi
	token=$(echo "$login_response" | jq -r '.token // empty')

	if [[ -z $token ]]; then
		echo "Failed to login. Response: $login_response"
		exit 1
	fi

	# Create session via API
	echo "Creating session '$DEMO_NAME' via API..."
	session_payload=$(
		cat <<EOF
{
  "name": "${DEMO_NAME}",
  "namespace": "default",
  "profile": {
    "ide": "jupyterlab",
    "image": "jupyter/minimal-notebook:latest",
    "cmd": ["start-notebook.sh", "--NotebookApp.token="]
  },
  "networking": {
    "host": "${HOST}"$(if [[ ${WITH_TLS} == "true" ]]; then echo ', "tlsSecretName": "'${DEMO_NAME}'-tls"'; fi)
  }
}
EOF
	)

	create_response=$(curl -s -X POST "http://console.${HOST_DOMAIN}/api/v1/server/sessions" \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $token" \
		-d "$session_payload")

	if echo "$create_response" | jq -e '.metadata.name' >/dev/null 2>&1; then
		echo "Session created successfully"
	else
		echo "Failed to create session. Response: $create_response"
		exit 1
	fi

	# Wait for session to be ready via API
	echo "Waiting for session to be ready..."
	for i in {1..24}; do # 2 minutes total
		session_status=$(curl -s -H "Authorization: Bearer $token" \
			"http://console.${HOST_DOMAIN}/api/v1/server/sessions/default/${DEMO_NAME}")

		phase=$(echo "$session_status" | jq -r '.status.phase // "Pending"')
		if [[ $phase == "Ready" ]]; then
			echo "Session is ready!"
			break
		elif [[ $phase == "Error" ]]; then
			echo "Session failed: $(echo "$session_status" | jq -r '.status.reason // "Unknown error"')"
			exit 1
		fi

		echo "Session status: $phase (attempt $i/24)"
		sleep 5
	done

	echo ">>> Demo endpoints:"
	echo "  UI    : http://console.${HOST_DOMAIN}/"
	echo "  App   : http://${HOST}   (Ingress)"
	echo "  HTTPS : https://${HOST}  (if TLS enabled)"
fi
