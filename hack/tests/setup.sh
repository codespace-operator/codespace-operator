#!/usr/bin/env bash
set -euo pipefail

# ---------- Config ----------
CLUSTER_NAME="${CLUSTER_NAME:-codespace}"
KIND_CONFIG="${KIND_CONFIG:-hack/tests/kind.yaml}"
NAMESPACE_SYS="${NAMESPACE_SYS:-codespace-operator-system}"

# images
IMG="${IMG:-ghcr.io/codespace-operator:dev}"
GATEWAY_IMG="${GATEWAY_IMG:-ghcr.io/codespace-operator/codespace-gateway:dev}"

WITH_TLS="${WITH_TLS:-false}"                 # "true" to enable cert-manager + self-signed issuer
HOST_DOMAIN="${HOST_DOMAIN:-localtest.me}"    # e.g. localtest.me / localhost.direct / sslip.io
DEMO_NAME="${DEMO_NAME:-demo}"                # demo session name
APPLY_DEMO="${APPLY_DEMO:-true}"              # set to "false" to skip creating a demo Session

# demo manifest (templated below if not provided)
DEMO_SESSION_FILE="${DEMO_SESSION_FILE:-hack/tests/demo-session.yaml}"

echo ">>> Creating kind cluster '${CLUSTER_NAME}'..."
if ! kind get clusters | grep -qx "${CLUSTER_NAME}"; then
  kind create cluster --config "${KIND_CONFIG}"
else
  echo "kind cluster '${CLUSTER_NAME}' already exists; skipping create."
fi

echo ">>> Ensuring default StorageClass (local-path)..."
if ! kubectl get sc >/dev/null 2>&1; then
  echo "kubectl not connected to cluster?"; exit 1
fi
if ! kubectl get sc | grep -q 'local-path'; then
  kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml
  kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
fi

echo ">>> Installing ingress-nginx..."
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=180s

if [[ "${WITH_TLS}" == "true" ]]; then
  echo ">>> Installing cert-manager (for local TLS)..."
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.3/cert-manager.yaml
  kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=5m
  cat <<'YAML' | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned
spec:
  selfSigned: {}
YAML
fi

echo ">>> Building operator image (${IMG})..."
make docker-build IMG="${IMG}"

echo ">>> Building gateway image (${GATEWAY_IMG})..."
make docker-build-gateway GATEWAY_IMG="${GATEWAY_IMG}"

echo ">>> Loading images into kind..."
kind load docker-image "${IMG}" --name "${CLUSTER_NAME}"
kind load docker-image "${GATEWAY_IMG}" --name "${CLUSTER_NAME}"

echo ">>> Installing CRDs via Helm chart (or comment this and use 'make install')..."
# If you prefer kubebuilder CRD install, uncomment the next line and remove '--create-namespace' helm install of CRDs
# make install

echo ">>> Deploying via Helm..."
helm upgrade --install codespace-operator ./helm \
  --namespace "${NAMESPACE_SYS}" --create-namespace \
  --set image.repository="${IMG%:*}" \
  --set image.tag="${IMG##*:}" \
  --set gateway.enabled=true \
  --set gateway.image.repository="${GATEWAY_IMG%:*}" \
  --set gateway.image.tag="${GATEWAY_IMG##*:}" \
  --set gateway.ingress.enabled=true \
  --set gateway.ingress.hosts[0].host="console.${HOST_DOMAIN}" \
  --set gateway.ingress.hosts[0].path="/"

kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-operator-controller-manager --timeout=180s
kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-operator-gateway --timeout=180s

echo ">>> Quick gateway smoke test (ClusterIP)..."
kubectl -n "${NAMESPACE_SYS}" run curl-gw --image=curlimages/curl:8.10.1 -i --rm -q --restart=Never -- \
  sh -lc 'curl -sf http://codespace-operator-gateway:8080/ | head -c 80 && echo' || true

if [[ "${APPLY_DEMO}" == "true" ]]; then
  echo ">>> Applying demo Session '${DEMO_NAME}'..."
  HOST="${DEMO_NAME}.${HOST_DOMAIN}"

  # Render a demo manifest if file missing
  if [[ ! -f "${DEMO_SESSION_FILE}" ]]; then
    mkdir -p "$(dirname "${DEMO_SESSION_FILE}")"
    cat > "${DEMO_SESSION_FILE}" <<'YAML'
apiVersion: codespace.codespace.dev/v1alpha1
kind: Session
metadata:
  name: __DEMO_NAME__
  namespace: default
spec:
  profile:
    ide: jupyterlab
    image: jupyter/minimal-notebook:latest
    cmd: ["start-notebook.sh","--NotebookApp.token="]
  networking:
    host: __HOST__
    # tlsSecretName: __TLS_SECRET__   # uncomment if WITH_TLS=true
YAML
  fi

  if [[ "${WITH_TLS}" == "true" ]]; then
    echo ">>> Creating TLS cert for ${HOST}..."
    kubectl apply -n default -f - <<YAML
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ${DEMO_NAME}-cert
spec:
  secretName: ${DEMO_NAME}-tls
  issuerRef:
    name: selfsigned
    kind: ClusterIssuer
  dnsNames:
  - ${HOST}
YAML
    TLS_SECRET="${DEMO_NAME}-tls"
  else
    TLS_SECRET=""
  fi

  # Render and apply
  sed -e "s/__DEMO_NAME__/${DEMO_NAME}/g" \
      -e "s/__HOST__/${HOST}/g" \
      -e "s/__TLS_SECRET__/${TLS_SECRET}/g" \
      "${DEMO_SESSION_FILE}" | kubectl apply -f -

  echo "Waiting for Session to be Ready (2m timeout)..."
  kubectl -n default wait --for=jsonpath='{.status.phase}'=Ready "session/${DEMO_NAME}" --timeout=120s || true

  echo ">>> Demo endpoints:"
  echo "  UI    : http://console.${HOST_DOMAIN}/"
  echo "  App   : http://${HOST}   (Ingress)"
  echo "  HTTPS : https://${HOST}  (if TLS enabled)"
fi

echo ">>> Done."
