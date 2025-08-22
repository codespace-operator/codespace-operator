#!/usr/bin/env bash
SETUP_CONFIG="${KIND_CONFIG:-hack/tests/config.sh}"
DEPLOY_SCRIPT="${DEPLOY_SCRIPT:-hack/tests/deploy.sh}"
source "${SETUP_CONFIG}"

set -euo pipefail

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

echo ">>> Building gateway image (${SERVER_IMG})..."
make docker-build-server SERVER_IMG="${SERVER_IMG}"

echo ">>> Loading images into kind..."
kind load docker-image "${IMG}" --name "${CLUSTER_NAME}"
kind load docker-image "${SERVER_IMG}" --name "${CLUSTER_NAME}"

echo ">>> Installing CRDs via Helm chart (or comment this and use 'make install')..."
# If you prefer kubebuilder CRD install, uncomment the next line and remove '--create-namespace' helm install of CRDs
# make install

./${DEPLOY_SCRIPT}
