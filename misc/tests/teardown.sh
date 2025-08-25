#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-codespace}"
NAMESPACE_SYS="codespace-operator"
DEMO_NAME="${DEMO_NAME:-demo}"
APPLY_DEMO="${APPLY_DEMO:-true}"

# echo ">>> Cleaning up demo resources..."
# if [[ "${APPLY_DEMO}" == "true" ]]; then
#   kubectl -n default delete session "${DEMO_NAME}" --ignore-not-found
#   kubectl -n default delete certificate "${DEMO_NAME}-cert" --ignore-not-found || true
#   kubectl -n default delete secret "${DEMO_NAME}-tls" --ignore-not-found || true
# fi

# echo ">>> Undeploying operator..."
# make undeploy || true

# echo ">>> Uninstalling CRDs..."
# make uninstall || true

echo ">>> Deleting kind cluster '${CLUSTER_NAME}'..."
kind delete cluster --name "${CLUSTER_NAME}" || true

echo ">>> Done."
