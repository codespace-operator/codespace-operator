#!/usr/bin/env bash
set -euo pipefail

SETUP_CONFIG="${SETUP_CONFIG:-misc/tests/config.sh}"
[[ -f "${SETUP_CONFIG}" ]] && source "${SETUP_CONFIG}"

CLUSTER_NAME="${CLUSTER_NAME:-codespace}"
HOST_DOMAIN="${HOST_DOMAIN:-codespace.test}"
DEMO_NAME="${DEMO_NAME:-demo}"

# Function to get Kind container's Docker network IP
get_kind_container_ip() {
    local container_name="${CLUSTER_NAME}-control-plane"
    docker inspect "$container_name" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null || echo ""
}

# Function to check if we have sudo/root access
check_sudo() {
    if [[ $EUID -eq 0 ]]; then
        return 0
    elif command -v sudo >/dev/null; then
        return 0
    else
        echo "ERROR: Need root access or sudo to modify /etc/hosts"
        exit 1
    fi
}

# Function to run command with appropriate privileges
run_privileged() {
    if [[ $EUID -eq 0 ]]; then
        "$@"
    else
        sudo "$@"
    fi
}

case "${1:-ip}" in
    "ip")
        DOCKER_IP=$(get_kind_container_ip)
        if [[ -z "${DOCKER_IP}" ]]; then
            echo "ERROR: Could not get Docker network IP for Kind container '${CLUSTER_NAME}-control-plane'" >&2
            echo "Make sure the Kind cluster is running: kind get clusters" >&2
            exit 1
        fi
        echo "${DOCKER_IP}"
        ;;
        
    "hosts-add")
        DOCKER_IP=$(get_kind_container_ip)
        if [[ -z "${DOCKER_IP}" ]]; then
            echo "ERROR: Could not get Docker network IP" >&2
            exit 1
        fi
        
        check_sudo
        echo "Adding entries to /etc/hosts..."
        
        # Define the hosts we want to add
        HOSTS=(
            "console.${HOST_DOMAIN}"
            "keycloak.${HOST_DOMAIN}"
            "${DEMO_NAME}.${HOST_DOMAIN}"
        )
        
        # Remove existing entries for our domains (cleanup)
        for host in "${HOSTS[@]}"; do
            run_privileged sed -i.backup "/[[:space:]]${host}$/d" /etc/hosts
        done
        
        # Add new entries
        {
            echo ""
            echo "# Kind cluster entries (added by kind-network.sh)"
            for host in "${HOSTS[@]}"; do
                echo "${DOCKER_IP}    ${host}"
            done
        } | run_privileged tee -a /etc/hosts > /dev/null
        
        echo "✅ Added entries to /etc/hosts:"
        for host in "${HOSTS[@]}"; do
            echo "   ${DOCKER_IP}    ${host}"
        done
        ;;
        
    "hosts-remove")
        check_sudo
        echo "Removing Kind entries from /etc/hosts..."
        
        HOSTS=(
            "console.${HOST_DOMAIN}"
            "keycloak.${HOST_DOMAIN}"
            "${DEMO_NAME}.${HOST_DOMAIN}"
        )
        
        # Remove our entries
        for host in "${HOSTS[@]}"; do
            run_privileged sed -i.backup "/[[:space:]]${host}$/d" /etc/hosts
        done
        
        # Remove our comment line
        run_privileged sed -i.backup '/# Kind cluster entries (added by kind-network.sh)/d' /etc/hosts
        
        echo "✅ Removed Kind entries from /etc/hosts"
        ;;
        
    "hosts-show")
        echo "Current Kind-related entries in /etc/hosts:"
        HOSTS=(
            "console.${HOST_DOMAIN}"
            "keycloak.${HOST_DOMAIN}"
            "${DEMO_NAME}.${HOST_DOMAIN}"
        )
        
        for host in "${HOSTS[@]}"; do
            grep "${host}" /etc/hosts 2>/dev/null || echo "  ${host} - NOT FOUND"
        done
        ;;
        
    "test")
        DOCKER_IP=$(get_kind_container_ip)
        if [[ -z "${DOCKER_IP}" ]]; then
            echo "ERROR: Could not get Docker network IP" >&2
            exit 1
        fi
        
        echo "Testing connectivity to Kind cluster at ${DOCKER_IP}..."
        
        # Test basic connectivity
        if curl -s --connect-timeout 5 "http://${DOCKER_IP}" >/dev/null; then
            echo "✅ Port 80 is accessible"
        else
            echo "❌ Port 80 is not accessible"
        fi
        
        # Test with Host header (ingress)
        if curl -s --connect-timeout 5 -H "Host: console.${HOST_DOMAIN}" "http://${DOCKER_IP}" >/dev/null; then
            echo "✅ Ingress is responding"
        else
            echo "❌ Ingress is not responding"
        fi
        ;;
        
    "info")
        DOCKER_IP=$(get_kind_container_ip)
        if [[ -z "${DOCKER_IP}" ]]; then
            echo "ERROR: Kind cluster '${CLUSTER_NAME}' is not running" >&2
            exit 1
        fi
        
        echo "Kind Cluster Network Information"
        echo "================================"
        echo "Cluster Name: ${CLUSTER_NAME}"
        echo "Docker IP: ${DOCKER_IP}"
        echo "Host Domain: ${HOST_DOMAIN}"
        echo ""
        echo "Services:"
        echo "  - Console: http://console.${HOST_DOMAIN}"
        echo "  - Keycloak: http://keycloak.${HOST_DOMAIN}"
        echo "  - Demo: http://${DEMO_NAME}.${HOST_DOMAIN}"
        echo ""
        echo "To add to /etc/hosts: $0 hosts-add"
        echo "To test connectivity: $0 test"
        ;;
        
    *)
        echo "Usage: $0 [ip|hosts-add|hosts-remove|hosts-show|test|info]"
        echo ""
        echo "Commands:"
        echo "  ip           - Show Docker network IP of Kind cluster"
        echo "  hosts-add    - Add entries to /etc/hosts"
        echo "  hosts-remove - Remove entries from /etc/hosts"
        echo "  hosts-show   - Show current entries in /etc/hosts"
        echo "  test         - Test connectivity to the cluster"
        echo "  info         - Show cluster network information (default)"
        echo ""
        exit 1
        ;;
esac