#!/bin/sh
set -e

# Default values
NODE_ID=${NODE_ID:-validator-1}
LISTEN_PORT=${LISTEN_PORT:-4001}
API_PORT=${API_PORT:-8080}
DATA_DIR=${DATA_DIR:-/home/validator/data}
TOTAL_NODES=${TOTAL_NODES:-4}
BOOTSTRAP_PEER=${BOOTSTRAP_PEER:-}

# Ensure data directory exists and has correct permissions
mkdir -p "${DATA_DIR}"

# Build command
CMD="./validator -id ${NODE_ID} -port ${LISTEN_PORT} -api-port ${API_PORT} -data-dir ${DATA_DIR} -total-nodes ${TOTAL_NODES}"

# Add primary flag if set
if [ "${IS_PRIMARY}" = "true" ]; then
    CMD="${CMD} -primary"
    echo "Starting as PRIMARY validator node: ${NODE_ID}"
else
    echo "Starting as validator node: ${NODE_ID}"
fi

# Add bootstrap peer if provided and we're not the bootstrap node
if [ -n "${BOOTSTRAP_PEER}" ] && [ "${IS_PRIMARY}" != "true" ]; then
    # Wait for bootstrap node to be ready
    echo "Waiting for bootstrap node to be ready..."
    sleep 5

    # Try to resolve the bootstrap peer DNS
    BOOTSTRAP_HOST=$(echo ${BOOTSTRAP_PEER} | cut -d':' -f1)

    # Check if getent is available (may not be in minimal Alpine)
    if command -v getent >/dev/null 2>&1; then
        BOOTSTRAP_IP=$(getent hosts ${BOOTSTRAP_HOST} | awk '{ print $1 }' | head -1)
    else
        # Fallback to nslookup if getent is not available
        BOOTSTRAP_IP=$(nslookup ${BOOTSTRAP_HOST} 2>/dev/null | grep 'Address:' | tail -n1 | awk '{print $2}')
    fi

    if [ -n "${BOOTSTRAP_IP}" ]; then
        echo "Resolved ${BOOTSTRAP_HOST} to ${BOOTSTRAP_IP}"
    else
        echo "Warning: Could not resolve ${BOOTSTRAP_HOST}, relying on Docker DNS"
    fi
fi

echo "Configuration:"
echo "  Node ID: ${NODE_ID}"
echo "  Listen Port: ${LISTEN_PORT}"
echo "  API Port: ${API_PORT}"
echo "  Data Directory: ${DATA_DIR}"
echo "  Total Nodes: ${TOTAL_NODES}"
echo ""
echo "Starting validator with command: ${CMD}"
echo "==========================================\n"

exec ${CMD}
