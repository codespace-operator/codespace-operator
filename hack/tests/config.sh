# ---------- Config ----------
CLUSTER_NAME="${CLUSTER_NAME:-codespace}"
KIND_CONFIG="${KIND_CONFIG:-hack/tests/kind.yaml}"
NAMESPACE_SYS="${NAMESPACE_SYS:-codespace-operator}"

IMG="${IMG:-ghcr.io/codespace-operator/codespace-operator:dev}"
SERVER_IMG="${SERVER_IMG:-ghcr.io/codespace-operator/codespace-server:dev}"

WITH_TLS="${WITH_TLS:-false}"                 # "true" to enable cert-manager + self-signed issuer
HOST_DOMAIN="${HOST_DOMAIN:-codespace.test}"    # e.g. codespace.test / localhost.direct / sslip.io
DEMO_NAME="${DEMO_NAME:-demo}"                # demo session name
APPLY_DEMO="${APPLY_DEMO:-true}"              # set to "false" to skip creating a demo Session

# demo manifest (templated below if not provided)
DEMO_SESSION_FILE="${DEMO_SESSION_FILE:-hack/tests/demo-session.yaml}"