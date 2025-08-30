#!/usr/bin/env bash
set -euo pipefail

SETUP_CONFIG="${SETUP_CONFIG:-misc/tests/config.sh}"
[[ -f "${SETUP_CONFIG}" ]] && source "${SETUP_CONFIG}"

CLUSTER_NAME="${CLUSTER_NAME:-codespace}"
NAMESPACE_SYS="${NAMESPACE_SYS:-codespace}"
DEMO_NAME="${DEMO_NAME:-demo}"
APPLY_DEMO="${APPLY_DEMO:-true}"

echo ">>> Deleting kind cluster '${CLUSTER_NAME}'..."
kind delete cluster --name "${CLUSTER_NAME}" || true

# Clean up temporary files
rm -f /tmp/kind-network-ip.env

echo ">>> Done."