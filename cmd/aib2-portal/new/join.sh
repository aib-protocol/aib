#!/bin/bash
# AIB Node Auto-Join Script
# For AI Agents: Automatically connect to AIB Testnet

set -e

AIB_VERSION="2.0.0-mvp"
AIB_API_PORT="${AIB_API_PORT:-51211}"
AIB_P2P_PORT="${AIB_P2P_PORT:-51212}"
AIB_DATA_DIR="${AIB_DATA_DIR:-./aib-data}"
AIB_BOOTSTRAP_HOST="${AIB_BOOTSTRAP_HOST:-www.aib.one}"

echo "=== AIB Node Auto-Join ==="
echo "Version: $AIB_VERSION"
echo "Network: testnet"
echo ""

# Check if binary exists
if [ ! -f "./aib-node" ]; then
    echo "Downloading AIB node binary..."
    wget -q --show-progress https://www.aib.one/binaries/aib-node-linux-amd64 -O aib-node || {
        echo "Download failed. Building from source..."
        git clone https://github.com/aib-protocol/aib.git tmp-aib 2>/dev/null || cd aib
        cd aib 2>/dev/null || cd .
        go build -o aib-node ./cmd/aib-node/ 2>/dev/null
    }
    chmod +x aib-node
fi

# Create data directory
mkdir -p "$AIB_DATA_DIR"

# Get bootstrap info
echo "Fetching bootstrap node info..."
BOOTSTRAP_INFO=$(curl -s https://$AIB_BOOTSTRAP_HOST/api/v1/bootstrap 2>/dev/null || echo "")

# Start node
echo "Starting AIB node..."
echo "  API Port: $AIB_API_PORT"
echo "  P2P Port: $AIB_P2P_PORT"
echo "  Data Dir: $AIB_DATA_DIR"
echo ""

./aib-node \
    --validator \
    --api-port "$AIB_API_PORT" \
    --p2p-port "$AIB_P2P_PORT" \
    --data-dir "$AIB_DATA_DIR" \
    --block-time 60 \
    --network testnet &

NODE_PID=$!
echo "Node started with PID: $NODE_PID"

# Wait for node to be ready
echo "Waiting for node to initialize..."
sleep 5

# Check status
for i in {1..30}; do
    STATUS=$(curl -s http://localhost:$AIB_API_PORT/v1/status 2>/dev/null || echo "")
    if echo "$STATUS" | grep -q "block_height"; then
        echo ""
        echo "✓ Node connected successfully!"
        echo ""
        curl -s http://localhost:$AIB_API_PORT/v1/status | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f\"Block Height: {d['data']['block_height']}\")
print(f\"Sync Status: {d['data']['sync_status']}\")
print(f\"Network: {d['data']['network']}\")
print(f\"Node ID: {d['data']['node_id']}\")
" 2>/dev/null || echo "$STATUS"
        echo ""
        echo "Use 'curl http://localhost:$AIB_API_PORT/v1/status' to check sync progress"
        exit 0
    fi
    sleep 2
    echo -n "."
done

echo ""
echo "✗ Failed to connect. Check logs above."
exit 1
