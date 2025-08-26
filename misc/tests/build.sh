#!/usr/bin/env bash
set -euo pipefail
SETUP_CONFIG="${SETUP_CONFIG:-misc/tests/config.sh}"
source "${SETUP_CONFIG}"

echo ">>> Building operator image (${IMG})..."
make docker-build IMG="${IMG}"

echo ">>> Building gateway image (${SERVER_IMG})..."
make docker-build-server SERVER_IMG="${SERVER_IMG}"

echo ">>> Loading images into kind..."
kind load docker-image "${IMG}" --name "${CLUSTER_NAME}"
kind load docker-image "${SERVER_IMG}" --name "${CLUSTER_NAME}"
