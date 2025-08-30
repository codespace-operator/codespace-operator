#!/usr/bin/env bash
# --- Config & helpers ---
: "${SETUP_CONFIG:=misc/tests/config.sh}"
: "${DEPLOY_SCRIPT:=misc/tests/deploy.sh}"
: "${BUILD_SCRIPT:=misc/tests/build.sh}"
: "${CREATE_SESSION_SCRIPT:=misc/tests/create-session.sh}"

source "${SETUP_CONFIG}"

need() { command -v "$1" >/dev/null || { echo "Missing '$1' in PATH"; exit 1; }; }
need kind; need kubectl; need docker; need helm; need sed

# --- Create or recreate kind cluster with required port maps ---
echo ">>> Ensuring kind cluster '${CLUSTER_NAME}' has host-port maps 80/443..."
create_cluster() { kind create cluster --config "${KIND_CONFIG}"; }

if ! kind get clusters | grep -qx "${CLUSTER_NAME}"; then
  create_cluster
else
  cid="$(docker ps --filter "name=${CLUSTER_NAME}-control-plane" --format '{{.ID}}' || true)"
  if [[ -z "${cid}" ]]; then
    echo "Control-plane container missing; recreating..."
    kind delete cluster --name "${CLUSTER_NAME}" || true
    create_cluster
  else
    ports_json="$(docker inspect "${cid}" --format '{{json .HostConfig.PortBindings}}' || echo '{}')"
    need_recreate=0
    for p in 80/tcp 443/tcp; do
      echo "${ports_json}" | grep -q "\"${p}\":" || { echo "Missing host port mapping for ${p}"; need_recreate=1; }
    done
    if [[ "${need_recreate}" -eq 1 ]]; then
      echo "Recreating cluster to apply port mappings from ${KIND_CONFIG}..."
      kind delete cluster --name "${CLUSTER_NAME}" || true
      create_cluster
    else
      echo "Cluster exists with correct host-port maps."
    fi
  fi
fi

echo ">>> Ensuring default StorageClass (local-path)..."
kubectl get sc >/dev/null 2>&1 || { echo "kubectl not connected to cluster?"; exit 1; }
if ! kubectl get sc | grep -q 'local-path'; then
  kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml
  kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
fi

echo ">>> Installing ingress-nginx..."
kubectl apply  -f https://kind.sigs.k8s.io/examples/ingress/deploy-ingress-nginx.yaml

# wait for readiness
kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=180s
kubectl -n ingress-nginx wait --for=condition=ready pod \
  -l app.kubernetes.io/component=controller --timeout=90s

if [[ "${WITH_TLS}" == "true" ]]; then
  echo ">>> Installing cert-manager (for local TLS)..."
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.3/cert-manager.yaml
  kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=5m
  kubectl apply -f - <<'YAML'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata: { name: selfsigned }
spec: { selfSigned: {} }
YAML
fi

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



./${BUILD_SCRIPT}


echo ">>> Installing CRDs via Helm chart (or comment this and use 'make install')..."
./${DEPLOY_SCRIPT}


./${CREATE_SESSION_SCRIPT}
