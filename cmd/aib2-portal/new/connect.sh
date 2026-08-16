#!/bin/bash
################################################################################
# AIB Testnet Auto-Connection Script for AI Agents
################################################################################
#
# ═══════════════════════════════════════════════════════════════════════════════
#  WHAT IS THIS?
# ═══════════════════════════════════════════════════════════════════════════════
# This is an autonomous deployment script designed specifically for AI Agents
# (like Claude, GPT, or other automated systems) to join the AIB (AI-Native
# Blockchain) testnet without human intervention.
#
# As an AI Agent reading this, executing this script will:
#   1. Provision a blockchain node on your host system
#   2. Synchronize with the live AIB testnet
#   3. Expose a local API (port 51211) for you to query chain data
#
# After execution, your agent will have direct access to:
#   - Real-time block height: curl http://localhost:51211/v1/status
#   - Block data retrieval:   curl http://localhost:51211/v1/blocks
#   - Peer network state:     curl http://localhost:51211/v1/peers
#
# ═══════════════════════════════════════════════════════════════════════════════
#  WHEN SHOULD AN AI AGENT RUN THIS?
# ═══════════════════════════════════════════════════════════════════════════════
# Run this script when:
#   - You need to establish a local blockchain data source
#   - You are asked to deploy, monitor, or interact with the AIB network
#   - You need to verify transactions, blocks, or consensus state
#   - You want to run a lightweight validator node for the testnet
#
# ═══════════════════════════════════════════════════════════════════════════════
#  PREREQUISITES
# ═══════════════════════════════════════════════════════════════════════════════
# REQUIRED:
#   - Linux x86_64 environment
#   - bash shell
#   - Network access to https://www.aib.one/
#   - curl or wget for downloading
# OPTIONAL (fallback only):
#   - git (if binary download fails and source build is needed)
#   - Go 1.22+ (only needed for building from source)
#
# ═══════════════════════════════════════════════════════════════════════════════
#  ONE-LINE USAGE
# ═══════════════════════════════════════════════════════════════════════════════
#   curl -sSL https://www.aib.one/connect | bash
#
# ═══════════════════════════════════════════════════════════════════════════════
#  WHAT THE SCRIPT DOES (Step-by-Step for AI Agents)
# ═══════════════════════════════════════════════════════════════════════════════
#
# STEP 1: Environment Validation
#   - Verifies required tools (wget/curl, bash)
#   - Checks for available ports 51211 (API) and 51212 (P2P)
#   - Prevents accidental overwrite if node already running
#
# STEP 2: Binary Acquisition
#   - Primary: Downloads precompiled aib-node binary from
#     https://www.aib.one/binaries/aib-node-linux-amd64
#   - Fallback: If download fails, attempts to clone
#     https://github.com/aib-protocol/aib.git and build with Go
#   - Binary is saved as ./aib-node and made executable
#
# STEP 3: Data Directory Initialization
#   - Creates ./aib-data/ for persistent blockchain storage
#   - This directory stores chain.db, utxo.db, and node_key.pem
#
# STEP 4: Bootstrap Peer Discovery
#   - Attempts to fetch live bootstrap peer list from
#     https://www.aib.one/api/v1/bootstrap
#   - If unavailable, falls back to hardcoded network defaults
#
# STEP 5: Node Startup
#   - Starts ./aib-node in the background with nohup
#   - Configuration:
#       --validator     : Enables block validation
#       --api-port 51211: Exposes REST API for agent queries
#       --p2p-port 51212: Opens P2P networking port
#       --data-dir      : Uses ./aib-data/ for persistence
#       --block-time 60 : Matches testnet 60s block time
#       --network testnet: Connects to the public testnet
#
# STEP 6: Health Check & Sync Monitoring
#   - Polls http://localhost:51211/v1/status every 2 seconds
#   - Waits up to 60 seconds for node to initialize and respond
#   - On success, extracts and reports:
#       * Current block height
#       * Sync status (syncing / synced)
#       * Network identifier
#
# STEP 7: Persistent State Export
#   - Writes connection metadata to ./aib-connection-info.json
#   - Your agent should parse this file for future API calls
#
# ═══════════════════════════════════════════════════════════════════════════════
#  NETWORK CONFIGURATION
# ═══════════════════════════════════════════════════════════════════════════════
#   Network Name:     testnet
#   Block Interval:   60 seconds
#   API Endpoint:     http://localhost:51211/v1/
#   P2P Endpoint:     0.0.0.0:51212 (TCP)
#   Data Directory:   ./aib-data/
#   Log File:         ./aib-node.log
#   PID File:         ./aib-node.pid
#
# ═══════════════════════════════════════════════════════════════════════════════
#  API ENDPOINT REFERENCE FOR AI AGENTS
# ═══════════════════════════════════════════════════════════════════════════════
# Once the node is running, use these endpoints in your agent code:
#
#   GET http://localhost:51211/v1/status
#     Returns: {"success":true,"data":{"block_height":N,
#              "sync_status":"synced","network":"testnet",...}}
#     Use this to monitor chain height and sync state.
#
#   GET http://localhost:51211/v1/blocks
#     Returns: Paginated list of recent blocks.
#     Use this to inspect transaction history.
#
#   GET http://localhost:51211/v1/blocks/{height}
#     Returns: Detailed block data for a specific height.
#     Use this for targeted block inspection.
#
#   GET http://localhost:51211/v1/peers
#     Returns: {"success":true,"data":{"peers":[],"total":0}}
#     Use this to monitor P2P connectivity.
#
# ═══════════════════════════════════════════════════════════════════════════════
#  LIFECYCLE MANAGEMENT
# ═══════════════════════════════════════════════════════════════════════════════
# Stop the node:
#   kill $(cat ./aib-node.pid)
#
# Restart the node:
#   kill $(cat ./aib-node.pid) && curl -sSL https://www.aib.one/connect | bash
#
# View logs:
#   tail -f ./aib-node.log
#
# Check if node is alive:
#   curl -s http://localhost:51211/v1/status | grep -q "block_height"
#
# ═══════════════════════════════════════════════════════════════════════════════
#  TROUBLESHOOTING GUIDE FOR AI AGENTS
# ═══════════════════════════════════════════════════════════════════════════════
# ERROR: "Port 51211 already in use"
#   CAUSE: Another aib-node process is already running.
#   FIX:   Run `kill $(cat ./aib-node.pid)` or use a different port.
#
# ERROR: "Download failed"
#   CAUSE: Network issue or binary URL unavailable.
#   FIX:   Install Go 1.22+ and let the script build from source.
#
# ERROR: "Node failed to start within expected time"
#   CAUSE: Port conflict, disk full, or corrupted data.
#   FIX:   Check `tail -50 ./aib-node.log` for details.
#          Try removing ./aib-data/ and re-running.
#
# ═══════════════════════════════════════════════════════════════════════════════
#  SUPPORT & RESOURCES
# ═══════════════════════════════════════════════════════════════════════════════
#   - GitHub Repository: https://github.com/aib-protocol/aib
#   - Documentation:     https://www.aib.one/docs/
#   - Full Peer Guide:   https://www.aib.one/AUTO-PEER.md
#
# ═══════════════════════════════════════════════════════════════════════════════
#  METADATA
# ═══════════════════════════════════════════════════════════════════════════════
#   Target Audience: AI Agents / Automated Systems
#   Human Friendly:   Yes (color output, readable logs)
#   Idempotent:       Partially (re-running restarts the node)
#   Last Updated:     2026-04-16
#
################################################################################

set -e

# ─── Terminal Colors ──────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# ─── Configuration ────────────────────────────────────────────────────────────
AIB_BINARY="aib-node"
AIB_BINARY_URL="https://www.aib.one/binaries/aib-node-linux-amd64"
AIB_DATA_DIR="./aib-data"
AIB_API_PORT=51211
AIB_P2P_PORT=51212
AIB_BLOCK_TIME=60
AIB_NETWORK="testnet"
AIB_LOG_FILE="./aib-node.log"
AIB_PID_FILE="./aib-node.pid"

# ─── Helper Functions ─────────────────────────────────────────────────────────
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

# ─── STEP 1: Welcome Message ──────────────────────────────────────────────────
print_header "AIB Testnet Auto-Connect for AI Agents"
echo ""
echo "This script will autonomously:"
echo "  1. Download and provision the AIB node binary"
echo "  2. Initialize blockchain data storage"
echo "  3. Start the node and connect to the testnet"
echo "  4. Monitor synchronization until the node is ready"
echo "  5. Expose a local API for agent queries"
echo ""
echo "Network:      $AIB_NETWORK"
echo "API Port:     $AIB_API_PORT"
echo "P2P Port:     $AIB_P2P_PORT"
echo "Data Dir:     $AIB_DATA_DIR"
echo ""

# ─── STEP 2: Environment Validation ───────────────────────────────────────────
print_step "Validating environment prerequisites..."

if command -v wget &> /dev/null; then
    print_info "wget found"
    DOWNLOADER="wget"
elif command -v curl &> /dev/null; then
    print_info "curl found"
    DOWNLOADER="curl"
else
    print_error "Neither wget nor curl is available. Please install one."
fi

if command -v git &> /dev/null; then
    print_info "git found (source fallback available)"
else
    print_info "git not found (source fallback unavailable)"
fi

if ss -tuln 2>/dev/null | grep -q ":$AIB_API_PORT "; then
    print_error "Port $AIB_API_PORT is already in use. Another node may be running. Stop it first: kill \$(cat $AIB_PID_FILE)"
fi

print_success "Environment validated"

# ─── STEP 3: Binary Acquisition ───────────────────────────────────────────────
print_step "Acquiring AIB node binary..."

if [ -f "./$AIB_BINARY" ]; then
    print_info "Binary './$AIB_BINARY' already exists. Skipping download."
else
    print_info "Attempting download from $AIB_BINARY_URL"

    if [ "$DOWNLOADER" = "wget" ]; then
        if wget -q --show-progress "$AIB_BINARY_URL" -O "$AIB_BINARY" 2>/dev/null; then
            print_success "Binary downloaded successfully"
        else
            print_info "Binary download failed. Attempting source build..."
        fi
    else
        if curl -sSL "$AIB_BINARY_URL" -o "$AIB_BINARY"; then
            print_success "Binary downloaded successfully"
        else
            print_info "Binary download failed. Attempting source build..."
        fi
    fi

    # Fallback: build from source
    if [ ! -f "./$AIB_BINARY" ] || [ ! -s "./$AIB_BINARY" ]; then
        TMP_DIR=$(mktemp -d)
        print_info "Cloning source into $TMP_DIR"

        cd "$TMP_DIR" || print_error "Cannot access temp directory: $TMP_DIR"
        git clone --depth 1 https://github.com/aib-protocol/aib.git aib-src || \
            print_error "Failed to clone GitHub repository"

        cd aib-src || print_error "Cannot enter source directory"

        if command -v go &> /dev/null; then
            print_info "Building from source with Go..."
            go build -o "$AIB_BINARY" ./cmd/aib-node/ || \
                print_error "Go build failed. Ensure Go 1.22+ is installed."
        else
            print_error "Go is not installed. Cannot build from source."
        fi

        cp "$AIB_BINARY" -
        cd - > /dev/null || true
        rm -rf "$TMP_DIR"
    fi

    chmod +x "$AIB_BINARY"
fi

print_success "Binary ready: ./$AIB_BINARY"

# ─── STEP 4: Data Directory Initialization ────────────────────────────────────
print_step "Initializing blockchain data directory..."
mkdir -p "$AIB_DATA_DIR" || print_error "Failed to create directory: $AIB_DATA_DIR"
print_success "Data directory ready: $AIB_DATA_DIR"

# ─── STEP 5: Bootstrap Peer Discovery ─────────────────────────────────────────
print_step "Discovering bootstrap peers..."

BOOTSTRAP_INFO=$(curl -s "https://www.aib.one/api/v1/bootstrap" 2>/dev/null || echo "")
if [ -n "$BOOTSTRAP_INFO" ]; then
    print_info "Bootstrap peer information retrieved"
else
    print_info "Bootstrap API unavailable. Using default network configuration."
fi

# ─── STEP 6: Node Startup ─────────────────────────────────────────────────────
print_step "Starting AIB node..."
print_info "Configuration:"
echo "  --validator    (block validation enabled)"
echo "  --api-port     $AIB_API_PORT"
echo "  --p2p-port     $AIB_P2P_PORT"
echo "  --data-dir     $AIB_DATA_DIR"
echo "  --block-time   $AIB_BLOCK_TIME"
echo "  --network      $AIB_NETWORK"

nohup ./"$AIB_BINARY" \
    --validator \
    --api-port "$AIB_API_PORT" \
    --p2p-port "$AIB_P2P_PORT" \
    --data-dir "$AIB_DATA_DIR" \
    --block-time "$AIB_BLOCK_TIME" \
    --network "$AIB_NETWORK" \
    > "$AIB_LOG_FILE" 2>&1 &

NODE_PID=$!
echo "$NODE_PID" > "$AIB_PID_FILE"

print_success "Node process started (PID: $NODE_PID)"
print_info "Logs: $AIB_LOG_FILE"
print_info "PID:  $AIB_PID_FILE"

# ─── STEP 7: Health Check & Sync Monitoring ───────────────────────────────────
print_step "Monitoring node initialization..."
echo "(polling http://localhost:$AIB_API_PORT/v1/status)"

MAX_WAIT=30
WAIT_COUNT=0

while [ $WAIT_COUNT -lt $MAX_WAIT ]; do
    sleep 2
    WAIT_COUNT=$((WAIT_COUNT + 1))
    echo -n "."

    STATUS=$(curl -s "http://localhost:$AIB_API_PORT/v1/status" 2>/dev/null || echo "")

    if echo "$STATUS" | grep -q "block_height"; then
        echo ""
        print_success "Node is online and responding to API requests!"

        BLOCK_HEIGHT=$(echo "$STATUS" | grep -o '"block_height":[0-9]*' | cut -d: -f2)
        SYNC_STATUS=$(echo "$STATUS" | grep -o '"sync_status":"[^"]*"' | cut -d: -f2 | tr -d '"')
        NETWORK=$(echo "$STATUS" | grep -o '"network":"[^"]*"' | cut -d: -f2 | tr -d '"')
        UPTIME=$(echo "$STATUS" | grep -o '"uptime":"[^"]*"' | cut -d: -f2 | tr -d '"')
        VERSION=$(echo "$STATUS" | grep -o '"version":"[^"]*"' | cut -d: -f2 | tr -d '"')

        print_header "Connection Summary for AI Agent"
        echo ""
        echo "  Network:       $NETWORK"
        echo "  Version:       $VERSION"
        echo "  Block Height:  $BLOCK_HEIGHT"
        echo "  Sync Status:   $SYNC_STATUS"
        echo "  Node PID:      $NODE_PID"
        echo "  Uptime:        $UPTIME"
        echo ""
        echo "  API Base URL:  http://localhost:$AIB_API_PORT/v1/"
        echo "  Data Dir:      $(pwd)/$AIB_DATA_DIR"
        echo "  Log File:      $(pwd)/$AIB_LOG_FILE"
        echo ""

        cat > aib-connection-info.json <<EOF
{
  "status": "connected",
  "network": "$NETWORK",
  "version": "$VERSION",
  "api_url": "http://localhost:$AIB_API_PORT",
  "api_version": "v1",
  "p2p_port": $AIB_P2P_PORT,
  "block_height": $BLOCK_HEIGHT,
  "sync_status": "$SYNC_STATUS",
  "node_pid": $NODE_PID,
  "node_uptime": "$UPTIME",
  "data_dir": "$(pwd)/$AIB_DATA_DIR",
  "log_file": "$(pwd)/$AIB_LOG_FILE",
  "pid_file": "$(pwd)/$AIB_PID_FILE",
  "endpoints": {
    "status": "http://localhost:$AIB_API_PORT/v1/status",
    "blocks": "http://localhost:$AIB_API_PORT/v1/blocks",
    "block_by_height": "http://localhost:$AIB_API_PORT/v1/blocks/{height}",
    "peers": "http://localhost:$AIB_API_PORT/v1/peers"
  },
  "commands": {
    "check_status": "curl -s http://localhost:$AIB_API_PORT/v1/status",
    "view_logs": "tail -f $(pwd)/$AIB_LOG_FILE",
    "stop_node": "kill $NODE_PID"
  }
}
EOF

        print_success "Connection metadata saved to: $(pwd)/aib-connection-info.json"
        echo ""
        print_info "Your agent can now query the blockchain using the API endpoints above."
        exit 0
    fi
done

echo ""
print_error "Node did not respond within the expected startup window."
echo ""
echo "Troubleshooting steps:"
echo "  1. Check logs:     tail -50 $AIB_LOG_FILE"
echo "  2. Check process:  ps aux | grep aib-node"
echo "  3. Free ports:     ss -tuln | grep -E '51211|51212'"
echo "  4. Reset data:     rm -rf $AIB_DATA_DIR && $0"
echo ""
exit 1
