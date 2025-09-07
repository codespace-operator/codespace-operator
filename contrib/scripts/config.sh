# ---------- Config ----------
CLUSTER_NAME="${CLUSTER_NAME:-codespace}"
KIND_CONFIG="${KIND_CONFIG:-contrib/manifests/kind.yaml}"
BUILD_SCRIPT="${BUILD_SCRIPT:-contrib/scripts/build.sh}"
DEPLOY_SCRIPT="${DEPLOY_SCRIPT:-contrib/scripts/deploy.sh}"
SETUP_CONFIG="${SETUP_CONFIG:-contrib/scripts/config.sh}"
CREATE_SESSION_SCRIPT="${CREATE_SESSION_SCRIPT:-contrib/scripts/create-session.sh}"

NAMESPACE_SYS="${NAMESPACE_SYS:-codespace}"
NAMESPACE_KEYCLOAK="${NAMESPACE_KEYCLOAK:-keycloak}"

IMG="${IMG:-ghcr.io/codespace-operator/codespace-operator:dev}"
SERVER_IMG="${SERVER_IMG:-ghcr.io/codespace-operator/codespace-server:dev}"

WITH_TLS="${WITH_TLS:-false}"
HOST_DOMAIN="${HOST_DOMAIN:-codespace.test}"
DEMO_NAME="${DEMO_NAME:-demo}"
APPLY_DEMO="${APPLY_DEMO:-true}"

DEMO_SESSION_FILE="${DEMO_SESSION_FILE:-contrib/manifests/demo-session.yaml}"
HELM_CHART="${HELM_CHART:-../charts/charts/codespace}"

# ----- OIDC (must match realm.json) -----
OIDC_CLIENT_ID="${OIDC_CLIENT_ID:-codespace-server}"
OIDC_CLIENT_SECRET="${OIDC_CLIENT_SECRET:-dev-secret}"
OIDC_SCOPES="${OIDC_SCOPES:-openid,profile,email}"

# ----- LDAP (dev) -----
LDAP_NAMESPACE="${LDAP_NAMESPACE:-ldap}"
LDAP_BIND_DN="${LDAP_BIND_DN:-cn=admin,dc=codespace,dc=test}"
LDAP_BIND_PASSWORD="${LDAP_BIND_PASSWORD:-admin}"
LDAP_BASE_DN="${LDAP_BASE_DN:-dc=codespace,dc=test}"
LDAP_USER_BASE_DN="${LDAP_USER_BASE_DN:-ou=people,dc=codespace,dc=test}"
LDAP_GROUP_BASE_DN="${LDAP_GROUP_BASE_DN:-ou=groups,dc=codespace,dc=test}"
LDAP_SERVICE_HOST="openldap.${LDAP_NAMESPACE}.svc.cluster.local"
LDAP_URL="ldap://${LDAP_SERVICE_HOST}:389"

# ----- Determine scheme & hosts -----
SCHEME="http"
CONSOLE_HOST="console.${HOST_DOMAIN}"
KEYCLOAK_HOST="keycloak.${HOST_DOMAIN}"
KEYCLOAK_INTERNAL_HOST="keycloak-keycloakx-http.keycloak.svc.cluster.local"
REDIRECT_URL="${SCHEME}://${CONSOLE_HOST}/auth/sso/callback"
ISSUER_URL="${SCHEME}://${KEYCLOAK_INTERNAL_HOST}/realms/Codespace-DEV"
HOSTNAME_URL="${SCHEME}://${KEYCLOAK_HOST}"

# ----- Keycloak manifests/templates -----
KEYCLOAK_REALM_TMPL="${KEYCLOAK_REALM_TMPL:-contrib/manifests/realm.json}"
KEYCLOAK_VALUES_TMPL="${KEYCLOAK_VALUES_TMPL:-contrib/manifests/keycloak-values.yaml}"
LDAP_VALUES="${LDAP_VALUES:-contrib/manifests/openldap-values.yaml}"
