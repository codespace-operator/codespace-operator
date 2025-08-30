#!/usr/bin/env bash
set -euo pipefail
SETUP_CONFIG="${SETUP_CONFIG:-misc/tests/config.sh}"
source "${SETUP_CONFIG}"

if [ "${BUILD_OPERATOR:-true}" != "false" ]; then
    echo ">>> Building operator image (${IMG})..."
    make docker-build IMG="${IMG}"
fi

if [ "${BUILD_SERVER:-true}" != "false" ]; then
    echo ">>> Building server image (${SERVER_IMG})..."
    make docker-build-server SERVER_IMG="${SERVER_IMG}"
fi

echo ">>> Loading images into kind..."
kind load docker-image "${IMG}" --name "${CLUSTER_NAME}"
kind load docker-image "${SERVER_IMG}" --name "${CLUSTER_NAME}"
