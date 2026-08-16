#!/bin/bash
################################################################################
# AIB Node Auto-Connect Script for AI Agents
#
# IMPORTANT: This is a standalone script. Save and run directly.
# No external downloads needed - you can build from source.
#
# Usage:
#   1. Save this script as connect-aib.sh
#   2. chmod +x connect-aib.sh
#   3. ./connect-aib.sh
#
# OR if you have Go installed, build directly:
#   git clone https://github.com/YOUR_GITHUB/aib.git
#   cd aib && go build -o aib-node ./cmd/aib-node/ && ./aib-node --validator
#
################################################################################

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

AIB_BINARY="aib-node"
AIB_DATA_DIR="./aib-data"
AIB_API_PORT="${AIB_API_PORT:-51211}"
AIB_P2P_PORT="${AIB_P2P_PORT:-51212}"
AIB_LOG_FILE="./aib-node.log"
AIB_PID_FILE="./aib-node.pid"

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
}

# ─── Welcome ───────────────────────────────────────────────────────────────
print_header "AIB Node Auto-Connect for AI Agents"
echo ""
echo "This script will set up an AIB testnet node."
echo ""
echo "Configuration:"
echo "  API Port:  $AIB_API_PORT"
echo "  P2P Port:  $AIB_P2P_PORT"
echo "  Data Dir:  $AIB_DATA_DIR"
echo ""

# ─── Check Environment ────────────────────────────────────────────────────
print_step "Checking environment..."

if ! command -v go &> /dev/null; then
    print_error "Go is required but not installed. Install from https://go.dev/dl/"
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
print_info "Go version: $GO_VERSION"

if ss -tuln 2>/dev/null | grep -q ":$AIB_API_PORT "; then
    print_error "Port $AIB_API_PORT already in use"
fi

print_success "Environment OK"

# ─── Build from Source ─────────────────────────────────────────────────────
print_step "Building aib-node from source..."

# Check if we're in the aib repo
if [ -f "go.mod" ] && grep -q "module github.com/aib-protocol/aib" go.mod 2>/dev/null; then
    print_info "Already in aib repository"
elif [ -d "./aib" ]; then
    print_info "Using existing ./aib directory"
    cd ./aib || print_error "Cannot enter aib directory"
else
    # Try to clone (will fail if repo doesn't exist - user needs to create it)
    print_info "Cloning aib repository..."

    # Try main GitHub, then fallback to local path
    if git clone https://github.com/aib-protocol/aib.git ./aib 2>/dev/null; then
        print_success "Repository cloned"
        cd ./aib || print_error "Cannot enter aib directory"
    else
        print_info "GitHub clone failed. Looking for local source..."

        # Check if we can find the source locally
        if [ -f "cmd/aib-node/main.go" ]; then
            print_info "Found source in current directory"
        else
            print_error "Cannot find aib source. Please ensure you're in the aib repository."
        fi
    fi
fi

# Build
print_info "Compiling aib-node..."
go build -o "$AIB_BINARY" ./cmd/aib-node/ || print_error "Build failed"

chmod +x "$AIB_BINARY"
print_success "Binary built: ./$AIB_BINARY"

# ─── Setup Data Dir ───────────────────────────────────────────────────────
print_step "Initializing data directory..."
mkdir -p "$AIB_DATA_DIR"
print_success "Data dir: $AIB_DATA_DIR"

# ─── Start Node ───────────────────────────────────────────────────────────
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

# ─── Wait for Ready ───────────────────────────────────────────────────────
print_step "Waiting for node to respond..."

for i in $(seq 1 30); do
    sleep 2
    echo -n "."

    STATUS=$(curl -s "http://127.0.0.1:$AIB_API_PORT/v1/status" 2>/dev/null || echo "")

    if echo "$STATUS" | grep -q "block_height"; then
        echo ""
        print_success "Node is online!"

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
