#!/bin/sh
set -e

# Default values
NODE_ID=${NODE_ID:-validator-1}
LISTEN_PORT=${LISTEN_PORT:-4001}
API_PORT=${API_PORT:-8080}
DATA_DIR=${DATA_DIR:-/root/data}
TOTAL_NODES=${TOTAL_NODES:-4}
BOOTSTRAP_PEER=${BOOTSTRAP_PEER:-}

# Build command
CMD="./validator -id ${NODE_ID} -port ${LISTEN_PORT} -api-port ${API_PORT} -data-dir ${DATA_DIR} -total-nodes ${TOTAL_NODES}"

# Add primary flag if set
if [ "${IS_PRIMARY}" = "true" ]; then
    CMD="${CMD} -primary"
fi

# Add bootstrap peer if provided and we're not the bootstrap node
if [ -n "${BOOTSTRAP_PEER}" ] && [ "${IS_PRIMARY}" != "true" ]; then
    # Wait for bootstrap node to be ready
    echo "Waiting for bootstrap node..."
    sleep 5

    # Try to resolve the bootstrap peer DNS
    BOOTSTRAP_HOST=$(echo ${BOOTSTRAP_PEER} | cut -d':' -f1)
    BOOTSTRAP_IP=$(getent hosts ${BOOTSTRAP_HOST} | awk '{ print $1 }' | head -1)

    if [ -n "${BOOTSTRAP_IP}" ]; then
        echo "Resolved ${BOOTSTRAP_HOST} to ${BOOTSTRAP_IP}"
        # Note: We still can't use this without the peer ID
        # DHT discovery will handle peer finding
    fi
fi

echo "Starting validator with command: ${CMD}"
exec ${CMD}
