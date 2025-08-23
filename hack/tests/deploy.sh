#!/usr/bin/env bash
SETUP_CONFIG="${KIND_CONFIG:-hack/tests/config.sh}"
source "${SETUP_CONFIG}"

echo ">>> Install CRD..."
make install

echo ">>> Deploying via Helm..."
helm upgrade --install codespace-operator ./helm \
  --namespace "${NAMESPACE_SYS}" --create-namespace \
  --set image.repository="${IMG%:*}" \
  --set image.tag="${IMG##*:}" \
  --set server.enabled=true \
  --set server.image.repository="${SERVER_IMG%:*}" \
  --set server.image.tag="${SERVER_IMG##*:}" \
  --set server.ingress.enabled=true \
  --set server.ingress.hosts[0].host="console.${HOST_DOMAIN}" \
  --set server.ingress.hosts[0].path="/"

kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-operator --timeout=180s
kubectl -n "${NAMESPACE_SYS}" rollout status deploy/codespace-operator-server --timeout=180s

if [[ "${APPLY_DEMO}" == "true" ]]; then
  echo ">>> Applying demo Session '${DEMO_NAME}'..."
  HOST="${DEMO_NAME}.${HOST_DOMAIN}"

  # Render a demo manifest if file missing
  if [[ ! -f "${DEMO_SESSION_FILE}" ]]; then
    mkdir -p "$(dirname "${DEMO_SESSION_FILE}")"
    cat > "${DEMO_SESSION_FILE}" <<'YAML'
apiVersion: codespace.codespace.dev/v1
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
