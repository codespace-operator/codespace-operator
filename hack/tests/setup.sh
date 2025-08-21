#!/usr/bin/env bash
set -euo pipefail

# ---------- Config ----------
CLUSTER_NAME="${CLUSTER_NAME:-codespace}"
KIND_CONFIG="${KIND_CONFIG:-hack/kind.yaml}"
NAMESPACE_SYS="codespace-operator-system"
IMG="${IMG:-example.com/codespace-operator:dev}"
WITH_TLS="${WITH_TLS:-false}"                 # "true" to enable cert-manager + self-signed issuer
HOST_DOMAIN="${HOST_DOMAIN:-localtest.me}"    # e.g. localtest.me / localhost.direct / sslip.io
DEMO_NAME="${DEMO_NAME:-demo}"               # demo session name
APPLY_DEMO="${APPLY_DEMO:-true}"             # set to "false" to skip creating a demo Session

# ---------- Kind config (created if missing) ----------
mkdir -p hack
if [[ ! -f "${KIND_CONFIG}" ]]; then
  cat > "${KIND_CONFIG}" <<'YAML'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: codespace
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 8080
    protocol: TCP
  - containerPort: 443
    hostPort: 8443
    protocol: TCP
YAML
fi

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
  kubectl patch storageclass local-path -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
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

echo ">>> Loading image into kind..."
kind load docker-image "${IMG}" --name "${CLUSTER_NAME}"

echo ">>> Installing CRDs..."
make install

echo ">>> Deploying operator..."
make deploy IMG="${IMG}"
kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-operator-controller-manager --timeout=180s

if [[ "${APPLY_DEMO}" == "true" ]]; then
  echo ">>> Applying demo Session '${DEMO_NAME}'..."
  HOST="${DEMO_NAME}.${HOST_DOMAIN}"
  TLS_FIELD=""
  if [[ "${WITH_TLS}" == "true" ]]; then
    # create a cert for the host and reference it
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
    TLS_FIELD="tlsSecretName: ${DEMO_NAME}-tls"
  fi

  kubectl apply -n default -f - <<YAML
apiVersion: codespace.codespace.dev/v1alpha1
kind: Session
metadata:
  name: ${DEMO_NAME}
spec:
  profile:
    ide: jupyterlab
    image: jupyter/minimal-notebook:latest
    cmd: ["start-notebook.sh","--NotebookApp.token="]
  networking:
    host: ${HOST}
    ${TLS_FIELD}
YAML

  echo "Waiting for Session to be Ready (2m timeout)..."
  kubectl -n default wait --for=jsonpath='{.status.phase}'=Ready "session/${DEMO_NAME}" --timeout=120s || true

  echo ">>> Demo endpoints:"
  echo "  HTTP : http://${HOST}:8080"
  echo "  HTTPS: https://${HOST}:8443 (if TLS enabled)"
  echo "  (Or open http://localhost:8080 / https://localhost:8443 if your browser ignores Host)"
fi

echo ">>> Done."
