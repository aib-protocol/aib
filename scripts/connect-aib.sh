#!/bin/bash
################################################################################
# AIB Node Auto-Connect Skill
# For: AI Agents (Claude, GPT, etc.)
# Purpose: Autonomous connection to AIB testnet blockchain
################################################################################

# ═══════════════════════════════════════════════════════════════════════════════
# ONE-LINE EXECUTION
# ═══════════════════════════════════════════════════════════════════════════════
# Save this script and run: bash connect-aib.sh
#
# Or fetch from GitHub (no external dependencies needed):
#   curl -sSL https://raw.githubusercontent.com/aib-protocol/aib/main/scripts/connect-aib.sh | bash
#
# ═══════════════════════════════════════════════════════════════════════════════
# BOOTSTRAP CONFIGURATION
# ═══════════════════════════════════════════════════════════════════════════════
# To connect to the live AIB testnet, configure ONE of these bootstrap peers:
#
# Option 1 - Direct IP (if you know a running node):
#   AIB_BOOTSTRAP_PEER=/ip4/<IP>/tcp/<PORT>/p2p/<PEER_ID>
#
# Option 2 - DNS (if DNS resolves):
#   AIB_BOOTSTRAP_PEER=/dns4/node.example.com/tcp/51212/p2p/<PEER_ID>
#
# Option 3 - Local test (create your own genesis):
#   Run in isolated mode with --local flag
#
# Current working bootstrap (replace with actual peer ID):
#   AIB_BOOTSTRAP_PEER=/dns4/bootstrap.aib.one/tcp/51212/p2p/12D3KooW...PLACEHOLDER
#
# ═══════════════════════════════════════════════════════════════════════════════
# WHAT THIS SCRIPT DOES
# ═══════════════════════════════════════════════════════════════════════════════
# 1. Check system (Linux, curl/wget, ports available)
# 2. Download aib-node binary from GitHub Releases
# 3. Create data directory for blockchain storage
# 4. Start node with validator mode
# 5. Monitor sync status
# 6. Export connection info to JSON for agent consumption
#
# ═══════════════════════════════════════════════════════════════════════════════

set -e

# ─── Configuration ────────────────────────────────────────────────────────────
AIB_BINARY_URL="https://github.com/aib-protocol/aib/releases/download/v2.0.0-mvp/aib-node-linux-amd64"
AIB_BINARY="aib-node"
AIB_DATA_DIR="./aib-data"
AIB_API_PORT="${AIB_API_PORT:-51211}"
AIB_P2P_PORT="${AIB_P2P_PORT:-51212}"
AIB_LOG_FILE="./aib-node.log"
AIB_PID_FILE="./aib-node.pid"

# ─── Colors ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# ─── Functions ────────────────────────────────────────────────────────────────
print_header() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
}

print_step() {
    echo ""
    echo -e "${GREEN}[STEP]${NC} $1"
}

print_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# ─── Step 1: Welcome ─────────────────────────────────────────────────────────
print_header "AIB Node Auto-Connect"
echo ""
echo "This script will set up an AIB testnet node."
echo ""
echo "Configuration:"
echo "  API Port:  $AIB_API_PORT"
echo "  P2P Port:  $AIB_P2P_PORT"
echo "  Data Dir:  $AIB_DATA_DIR"
echo ""

# ─── Step 2: Environment Check ───────────────────────────────────────────────
print_step "Checking environment..."

if [ "$(uname)" != "Linux" ]; then
    print_info "Warning: Not Linux. May not work."
fi

if [ "$(uname -m)" != "x86_64" ]; then
    print_info "Warning: Not x86_64 architecture."
fi

if command -v wget &> /dev/null || command -v curl &> /dev/null; then
    print_info "Download tool found"
else
    print_error "Need wget or curl"
fi

if ss -tuln 2>/dev/null | grep -q ":$AIB_API_PORT "; then
    print_error "Port $AIB_API_PORT already in use"
fi

print_success "Environment OK"

# ─── Step 3: Download Binary ──────────────────────────────────────────────────
print_step "Getting aib-node binary..."

if [ -f "./$AIB_BINARY" ] && [ -s "./$AIB_BINARY" ]; then
    print_info "Binary exists, skipping download"
else
    print_info "Downloading from GitHub Releases..."

    # Try GitHub Releases first
    if command -v wget &> /dev/null; then
        wget -q --show-progress "$AIB_BINARY_URL" -O "$AIB_BINARY" 2>/dev/null || true
    else
        curl -sSL "$AIB_BINARY_URL" -o "$AIB_BINARY" || true
    fi

    # Fallback to source build if download fails
    if [ ! -s "./$AIB_BINARY" ]; then
        print_info "Release download failed. Building from source..."

        if ! command -v go &> /dev/null; then
            print_error "Go not found. Cannot build from source."
        fi

        TMP_DIR=$(mktemp -d)
        git clone --depth 1 https://github.com/aib-protocol/aib.git "$TMP_DIR/aib"
        "$TMP_DIR/aib/go.mod" 2>/dev/null || true

        cd "$TMP_DIR/aib" || print_error "Cannot enter source dir"
        go build -o "$AIB_BINARY" ./cmd/aib-node/ || print_error "Build failed"
        cp "$AIB_BINARY" -
        cd - > /dev/null || true
        rm -rf "$TMP_DIR"
    fi

    chmod +x "$AIB_BINARY"
fi

print_success "Binary ready"

# ─── Step 4: Setup Data Dir ───────────────────────────────────────────────────
print_step "Initializing data directory..."
mkdir -p "$AIB_DATA_DIR"
print_success "Data dir: $AIB_DATA_DIR"

# ─── Step 5: Start Node ───────────────────────────────────────────────────────
print_step "Starting aib-node..."

nohup ./"$AIB_BINARY" \
    --validator \
    --api-port "$AIB_API_PORT" \
    --p2p-port "$AIB_P2P_PORT" \
    --data-dir "$AIB_DATA_DIR" \
    --block-time 60 \
    --network testnet \
    > "$AIB_LOG_FILE" 2>&1 &

NODE_PID=$!
echo "$NODE_PID" > "$AIB_PID_FILE"

print_success "Node started (PID: $NODE_PID)"

# ─── Step 6: Wait for Ready ───────────────────────────────────────────────────
print_step "Waiting for node to respond..."

for i in $(seq 1 30); do
    sleep 2
    echo -n "."

    STATUS=$(curl -s "http://127.0.0.1:$AIB_API_PORT/v1/status" 2>/dev/null || echo "")

    if echo "$STATUS" | grep -q "block_height"; then
        echo ""
        print_success "Node is online!"

        # Parse status
        HEIGHT=$(echo "$STATUS" | grep -o '"block_height":[0-9]*' | cut -d: -f2)
        SYNC=$(echo "$STATUS" | grep -o '"sync_status":"[^"]*"' | cut -d: -f2 | tr -d '"')

        print_header "Connection Info"
        echo ""
        echo "  Block Height: $HEIGHT"
        echo "  Sync Status:  $SYNC"
        echo "  API Endpoint: http://127.0.0.1:$AIB_API_PORT/v1/"
        echo "  Node PID:     $NODE_PID"
        echo ""

        # Export JSON for agents
        cat > aib-connection.json <<EOF
{
  "status": "connected",
  "api_url": "http://127.0.0.1:$AIB_API_PORT",
  "block_height": $HEIGHT,
  "sync_status": "$SYNC",
  "pid": $NODE_PID,
  "commands": {
    "status": "curl -s http://127.0.0.1:$AIB_API_PORT/v1/status",
    "blocks": "curl -s http://127.0.0.1:$AIB_API_PORT/v1/blocks",
    "stop": "kill $NODE_PID"
  }
}
EOF

        print_success "Saved to: aib-connection.json"
        echo ""
        echo "API commands:"
        echo "  curl http://127.0.0.1:$AIB_API_PORT/v1/status"
        echo "  curl http://127.0.0.1:$AIB_API_PORT/v1/blocks"
        echo ""

        exit 0
    fi
done

echo ""
print_error "Node failed to start. Check: tail -20 $AIB_LOG_FILE"
exit 1
