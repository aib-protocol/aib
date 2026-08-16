#!/bin/bash
#
# ╔══════════════════════════════════════════════════════════════════════╗
# ║                    AIB 2.0 Node Installer                            ║
# ║                    AI-Powered Blockchain Network                     ║
# ║                                                                       ║
# ║  Usage: curl -sL https://www.aib.one/install.sh | bash -s testnet   ║
# ║                                                                       ║
# ╚══════════════════════════════════════════════════════════════════════╝
#

set -e

# ========== Cyberpunk Color Palette ==========
RED='\033[0;31m'
BRIGHT_RED='\033[1;31m'
GREEN='\033[0;32m'
BRIGHT_GREEN='\033[1;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
BRIGHT_MAGENTA='\033[1;35m'
CYAN='\033[0;36m'
BRIGHT_CYAN='\033[1;36m'
WHITE='\033[1;37m'
GRAY='\033[0;90m'
NC='\033[0m'

# ========== Special Characters ==========
ARROW='▶'
BULLET='●'
SQUARE='■'
TRIANGLE='▲'
CHECK='✓'
CROSS='✗'
ROCKET='🚀'
GEAR='⚙'
LOCK='🔒'
SHIELD='🛡'
NETWORK='🌐'
DISK='💾'
TERMINAL='⌨'
SCANNER='📡'
CLOCK='⏱'
DATABASE='🗄'
FLAME='🔥'
BOLT='⚡'

# ========== Default Configuration ==========
PROJECT_NAME="AIB 2.0"
BINARY_BASE_URL="https://www.aib.one/binaries"
INSTALL_DIR="$HOME/.aib-node"
SERVICE_NAME="aib-node"
DEFAULT_NETWORK="testnet"
DEFAULT_DATA_DIR="$HOME/.aib"
DEFAULT_API_PORT=8080
DEFAULT_BLOCK_TIME=30

# Network port mapping
TESTNET_PORT=51413
MAINNET_PORT=31415

# Parse command line arguments
NETWORK="$DEFAULT_NETWORK"
DATA_DIR="$DEFAULT_DATA_DIR"
API_PORT="$DEFAULT_API_PORT"
P2P_PORT=""
VALIDATOR_MODE=false
BLOCK_TIME="$DEFAULT_BLOCK_TIME"
INSTALL_SYSTEMD=false
NO_START=false
DEBUG_MODE=false

# ========== Command Line Parsing ==========
if [[ $# -gt 0 ]]; then
    case "$1" in
        testnet|mainnet)
            NETWORK="$1"
            ;;
        stop)
            stop_node
            exit 0
            ;;
        uninstall)
            uninstall_node
            exit 0
            ;;
        status)
            show_status
            exit 0
            ;;
        *)
            print_banner
            echo -e "${RED}Unknown command: $1${NC}"
            echo ""
            echo -e "${CYAN}Available commands:${NC}"
            echo -e "  ${GREEN}testnet|mainnet${NC}  - Install node on specified network"
            echo -e "  ${GREEN}stop${NC}           - Stop the running node"
            echo -e "  ${GREEN}uninstall${NC}      - Remove the node completely"
            echo -e "  ${GREEN}status${NC}         - Show node status"
            echo ""
            echo -e "${CYAN}Examples:${NC}"
            echo -e "  ${YELLOW}curl -sL https://www.aib.one/install.sh | bash -s testnet${NC}"
            echo -e "  ${YELLOW}curl -sL https://www.aib.one/install.sh | bash -s stop${NC}"
            echo -e "  ${YELLOW}curl -sL https://www.aib.one/install.sh | bash -s uninstall${NC}"
            exit 1
            ;;
    esac
fi

# Set P2P port based on network (will be overridden by command line)
NETWORK="$DEFAULT_NETWORK"

# Set P2P port and block time based on network
if [[ "$NETWORK" == "testnet" ]]; then
    P2P_PORT=$TESTNET_PORT
    CHAIN_ID="aib-testnet-1"
    BLOCK_TIME=30
else
    P2P_PORT=$MAINNET_PORT
    CHAIN_ID="aib-mainnet-1"
    BLOCK_TIME=60
fi

# ========== Cyberpunk Logging Functions ==========
print_banner() {
    clear
    echo -e "${CYAN}"
    cat <<'EOF'
╔═══════════════════════════════════════════════════════════════════════════════╗
║                                                                               ║
║   ██████╗ ███████╗ ██████╗ ███████╗ ██████╗██████╗  ██████╗ ███████╗           ║
║  ██╔════╝ ██╔════╝██╔═══██╗██╔════╝██╔════╝██╔══██╗██╔══██╗██╔════╝           ║
║  ██║  ███╗█████╗  ██║   ██║███████╗███████╗██████╔╝██║  ██║█████╗             ║
║  ██║   ██║██╔══╝  ██║   ██║╚════██║╚════██║██╔══██╗██║  ██║██╔══╝             ║
║  ╚██████╔╝███████╗╚██████╔╝███████║███████║██║  ██║██████╔╝███████╗           ║
║   ╚═════╝ ╚══════╝ ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝╚═════╝ ╚══════╝           ║
║                                                                               ║
║                    ░█▀█░█▀▄░█▀▀░█░█░█▀█░█▀▀░█▀█                                ║
║                    ░█▀█░█▀▄░█▀▀░▀▄▀░█▀█░▀▀█░█▀▄                                ║
║                    ░▀░▀░▀░▀░▀▀▀░▀▀▀░▀░▀░▀▀▀░▀░▀                                ║
║                                                                               ║
╚═══════════════════════════════════════════════════════════════════════════════╝
EOF
    echo -e "${NC}"
    echo -e "${BRIGHT_CYAN}                   [ ${CYAN}AI-Powered Blockchain Network${BRIGHT_CYAN} ]${NC}"
    echo -e "${BRIGHT_CYAN}                   [ ${CYAN}Proof of AI Work Consensus${BRIGHT_CYAN} ]${NC}"
    echo ""
    echo -e "${GRAY}═══════════════════════════════════════════════════════════════════════════${NC}"
    echo ""
}

log_debug() {
    if [[ "$DEBUG_MODE" == true ]]; then
        echo -e "${GRAY}[${MAGENTA}DEBUG${GRAY}]${NC} $1"
    fi
}

log_info() {
    echo -e "${CYAN}[${BULLET}]${NC} $1"
}

log_success() {
    echo -e "${BRIGHT_GREEN}[${CHECK}]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[${GEAR}]${NC} $1"
}

log_error() {
    echo -e "${RED}[${CROSS}]${NC} $1"
}

log_phase() {
    echo ""
    echo -e "${CYAN}┌─────────────────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC} ${BRIGHT_CYAN}$1${NC}$(printf '%*s' $((60 - ${#1})) '')${CYAN}│${NC}"
    echo -e "${CYAN}└─────────────────────────────────────────────────────────────────────┘${NC}"
    echo ""
}

# ========== Download Binary ==========
download_binary() {
    log_phase "${ARROW} PHASE 1: BINARY ACQUISITION"

    # Check for existing installation
    if [[ -f "$INSTALL_DIR/aib-node" ]]; then
        EXISTING_SIZE=$(stat -f%z "$INSTALL_DIR/aib-node" 2>/dev/null || stat -c%s "$INSTALL_DIR/aib-node" 2>/dev/null)

        # Check if binary is valid (non-zero and ELF)
        if [[ "$EXISTING_SIZE" -gt 1000 ]]; then
            if file "$INSTALL_DIR/aib-node" 2>/dev/null | grep -q "ELF"; then
                log_warn "${YELLOW}Existing AIB node detected${NC}"
                log_info "${CYAN}Binary: $INSTALL_DIR/aib-node ($(numfmt --to=iec $EXISTING_SIZE 2>/dev/null || echo ${EXISTING_SIZE}B))${NC}"

                echo ""
                echo -e "${YELLOW}Choose action:${NC}"
                echo -e "  ${CYAN}1)${NC} Use existing binary (skip download)"
                echo -e "  ${CYAN}2)${NC} Upgrade to latest version"
                echo ""
                read -p "$(echo -e ${CYAN}"[?] Your choice [1-2]: "${NC})" -n 1 -r choice
                echo ""

                case "$choice" in
                    1|"")
                        log_success "${BRIGHT_CYAN}Using existing binary${NC}"
                        return 0
                        ;;
                    2)
                        log_info "${CYAN}Upgrading to latest version...${NC}"
                        ;;
                    *)
                        log_info "${CYAN}Upgrading to latest version...${NC}"
                        ;;
                esac
            else
                log_warn "${YELLOW}Existing binary corrupted, re-downloading...${NC}"
            fi
        else
            log_warn "${YELLOW}Existing binary invalid (${EXISTING_SIZE} bytes), re-downloading...${NC}"
        fi

        # Backup old binary
        if [[ -f "$INSTALL_DIR/aib-node" ]]; then
            mv "$INSTALL_DIR/aib-node" "$INSTALL_DIR/aib-node.backup.$(date +%s)"
        fi
    fi

    # Detect architecture
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)
            BINARY_ARCH="amd64"
            log_debug "${GREEN}  └─ Architecture: x86_64 (amd64)${NC}"
            ;;
        aarch64|arm64)
            BINARY_ARCH="arm64"
            log_debug "${GREEN}  └─ Architecture: ARM64${NC}"
            ;;
        *)
            log_error "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac

    OS=$(uname -s)
    log_debug "${GREEN}  └─ OS: $OS${NC}"

    BINARY_FILE="aib-node-linux-${BINARY_ARCH}"
    BINARY_URL="${BINARY_BASE_URL}/${BINARY_FILE}"

    echo -e "${CYAN}[${DISK}]${NC} ${BRIGHT_CYAN}Target:${NC} $BINARY_FILE"
    echo -e "${CYAN}[${NETWORK}]${NC} ${BRIGHT_CYAN}Source:${NC}  $BINARY_URL"
    echo ""

    if command -v wget &> /dev/null; then
        log_debug "${YELLOW}  Using wget for download...${NC}"
        wget --show-progress "$BINARY_URL" -O "/tmp/${BINARY_FILE}" 2>&1 | \
            grep -E "[0-9]%|KB/s|MB/s" || true
    elif command -v curl &> /dev/null; then
        log_debug "${YELLOW}  Using curl for download...${NC}"
        curl -L "$BINARY_URL" -o "/tmp/${BINARY_FILE}" --progress-bar
    else
        log_error "wget or curl required"
        exit 1
    fi

    if [[ ! -f "/tmp/${BINARY_FILE}" ]]; then
        log_error "Download failed"
        exit 1
    fi

    FILE_SIZE=$(stat -f%z "/tmp/${BINARY_FILE}" 2>/dev/null || stat -c%s "/tmp/${BINARY_FILE}" 2>/dev/null)
    log_debug "${GREEN}  └─ Downloaded: $(numfmt --to=iec $FILE_SIZE 2>/dev/null || echo ${FILE_SIZE} bytes)${NC}"

    # Install binary
    mkdir -p "$INSTALL_DIR"
    mv "/tmp/${BINARY_FILE}" "$INSTALL_DIR/aib-node"
    chmod +x "$INSTALL_DIR/aib-node"

    log_debug "${YELLOW}  Verifying binary...${NC}"
    if file "$INSTALL_DIR/aib-node" 2>/dev/null | grep -q "ELF"; then
        log_debug "${GREEN}  └─ Binary verified: ELF executable${NC}"
    fi

    echo ""
    log_success "${BRIGHT_CYAN}Binary installed:${NC} $INSTALL_DIR/aib-node"
}

# ========== Setup Data Directory ==========
setup_data_dir() {
    log_phase "${ARROW} PHASE 2: DATA DIRECTORY SETUP"

    log_debug "${MAGENTA}▶ Creating data directory structure...${NC}"
    mkdir -p "$DATA_DIR"/{chain,utxo,mempool}

    log_debug "${GREEN}  └─ Data dir: $DATA_DIR${NC}"
    log_debug "${GREEN}  └─ Chain db: $DATA_DIR/chain${NC}"
    log_debug "${GREEN}  └─ UTXO db:   $DATA_DIR/utxo${NC}"
    log_debug "${GREEN}  └─ Mempool:   $DATA_DIR/mempool${NC}"

    echo ""
    log_success "${BRIGHT_CYAN}Data directory ready:${NC} $DATA_DIR"
}

# ========== Start Node ==========
start_node() {
    log_phase "${ARROW} PHASE 3: NODE INITIALIZATION"

    log_debug "${MAGENTA}▶ Starting AIB node process...${NC}"
    log_debug "${GREEN}  └─ Network:   $NETWORK${NC}"
    log_debug "${GREEN}  └─ Chain ID:  $CHAIN_ID${NC}"
    log_debug "${GREEN}  └─ API Port:  $API_PORT${NC}"
    log_debug "${GREEN}  └─ P2P Port:  $P2P_PORT${NC}"
    log_debug "${GREEN}  └─ Validator: true${NC}"
    log_debug "${GREEN}  └─ Block Time: ${BLOCK_TIME}s${NC}"
    echo ""

    # Kill existing node
    pkill -f "aib-node" 2>/dev/null || true
    sleep 1

    # Start node
    cd "$INSTALL_DIR"
    nohup ./aib-node \
        --network="$NETWORK" \
        --data-dir="$DATA_DIR" \
        --api-port="$API_PORT" \
        --p2p-port="$P2P_PORT" \
        --validator \
        --block-time="$BLOCK_TIME" \
        > "$DATA_DIR/node.log" 2>&1 &

    NODE_PID=$!
    sleep 3

    if kill -0 $NODE_PID 2>/dev/null; then
        log_success "${BRIGHT_CYAN}Node started (PID: $NODE_PID)${NC}"
        log_debug "${GREEN}  └─ View logs: tail -f $DATA_DIR/node.log${NC}"
    else
        log_error "Node failed to start"
        echo ""
        echo -e "${YELLOW}--- Last 30 lines of log ---${NC}"
        tail -30 "$DATA_DIR/node.log" 2>/dev/null || echo "No log file yet"
        exit 1
    fi
}

# ========== Verify Installation ==========
verify_installation() {
    log_phase "${ARROW} PHASE 4: INSTALLATION VERIFICATION"

    local max_attempts=15
    local attempt=0

    while [[ $attempt -lt $max_attempts ]]; do
        attempt=$((attempt + 1))

        if command -v curl &> /dev/null; then
            log_debug "${MAGENTA}▶ Probing node API (attempt $attempt/$max_attempts)...${NC}"

            STATUS=$(curl -s "http://127.0.0.1:$API_PORT/v1/status" 2>/dev/null || echo "")

            if [[ -n "$STATUS" ]]; then
                NODE_HEIGHT=$(echo "$STATUS" | grep -o '"block_height":[0-9]*' | cut -d: -f2)
                NODE_PEERS=$(echo "$STATUS" | grep -o '"peers":[0-9]*' | cut -d: -f2)
                NODE_UPTIME=$(echo "$STATUS" | grep -o '"uptime":"[^"]*"' | cut -d: -f2 | tr -d '"')

                log_success "${BRIGHT_CYAN}Node is running and responsive!${NC}"
                echo ""
                echo -e "${CYAN}╔════════════════════════════════════════════════════════════════════╗${NC}"
                echo -e "${CYAN}║${NC} ${BRIGHT_CYAN}▶ NODE STATUS${NC}                                                   ${CYAN}║${NC}"
                echo -e "${CYAN}╠════════════════════════════════════════════════════════════════════╣${NC}"
                echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}Network:${NC}     ${BRIGHT_CYAN}$NETWORK${NC}$(printf '%*s' $((64 - 9 - ${#NETWORK})) '')${CYAN}║${NC}"
                echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}Chain ID:${NC}    ${CYAN}$CHAIN_ID${NC}$(printf '%*s' $((64 - 9 - ${#CHAIN_ID})) '')${CYAN}║${NC}"
                echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}Block Height:${NC} ${BRIGHT_CYAN}${NODE_HEIGHT:-0}$(printf '%*s' $((64 - 13 - ${#NODE_HEIGHT:-0})) '')${CYAN}║${NC}"
                echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}Peers:${NC}        ${BRIGHT_CYAN}${NODE_PEERS:-0}$(printf '%*s' $((64 - 6 - ${#NODE_PEERS:-0})) '')${CYAN}║${NC}"
                echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}Uptime:${NC}       ${CYAN}${NODE_UPTIME:-N/A}$(printf '%*s' $((64 - 7 - ${#NODE_UPTIME:-N/A})) '')${CYAN}║${NC}"
                echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}API Endpoint:${NC} ${CYAN}http://127.0.0.1:$API_PORT$(printf '%*s' $((64 - 12 - 24)) '')${CYAN}║${NC}"
                echo -e "${CYAN}╚════════════════════════════════════════════════════════════════════╝${NC}"
                echo ""
                return 0
            fi
        fi

        sleep 2
    done

    log_warn "Node verification timeout, but may still be initializing..."
    log_info "Check manually: curl http://127.0.0.1:$API_PORT/v1/status"
}

# ========== Show Completion ==========
show_completion() {
    echo ""
    echo -e "${BRIGHT_CYAN}╔══════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BRIGHT_CYAN}║${NC}        ${BRIGHT_CYAN}AIB 2.0 NODE INSTALLATION COMPLETE${NC}                      ${BRIGHT_CYAN}║${NC}"
    echo -e "${BRIGHT_CYAN}║${NC}        ${GREEN}Welcome to the AI-Powered Blockchain!${NC}                   ${BRIGHT_CYAN}║${NC}"
    echo -e "${BRIGHT_CYAN}╚══════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${BRIGHT_CYAN}Configuration:${NC}"
    echo -e "  ${CYAN}•${NC} Network:     ${BRIGHT_CYAN}$NETWORK${NC}"
    echo -e "  ${CYAN}•${NC} Chain ID:    ${CYAN}$CHAIN_ID${NC}"
    echo -e "  ${CYAN}•${NC} Data Dir:    ${BRIGHT_CYAN}$DATA_DIR${NC}"
    echo -e "  ${CYAN}•${NC} API Port:    ${BRIGHT_CYAN}$API_PORT${NC}"
    echo -e "  ${CYAN}•${NC} P2P Port:    ${BRIGHT_CYAN}$P2P_PORT${NC}"
    echo -e "  ${CYAN}•${NC} Binary:      ${BRIGHT_CYAN}$INSTALL_DIR/aib-node${NC}"
    echo ""
    echo -e "${BRIGHT_CYAN}Quick Commands:${NC}"
    echo -e "  ${CYAN}•${NC} Check status: ${YELLOW}curl http://127.0.0.1:$API_PORT/v1/status${NC}"
    echo -e "  ${CYAN}•${NC} View peers:   ${YELLOW}curl http://127.0.0.1:$API_PORT/v1/peers${NC}"
    echo -e "  ${CYAN}•${NC} View blocks:  ${YELLOW}curl http://127.0.0.1:$API_PORT/v1/blocks${NC}"
    echo -e "  ${CYAN}•${NC} View logs:    ${YELLOW}tail -f $DATA_DIR/node.log${NC}"
    echo -e "  ${CYAN}•${NC} Stop node:    ${YELLOW}pkill aib-node${NC}"
    echo ""
    echo -e "${BRIGHT_CYAN}Blockchain Explorer:${NC}"
    echo -e "  ${CYAN}•${NC} ${BRIGHT_CYAN}https://www.aib.one/blocks${NC}  - View all blocks"
    echo -e "  ${CYAN}•${NC} ${BRIGHT_CYAN}https://www.aib.one/peers${NC}   - View network peers"
    echo -e "  ${CYAN}•${NC} ${BRIGHT_CYAN}https://www.aib.one/tx${NC}      - Transaction viewer"
    echo ""
    echo -e "${BRIGHT_GREEN}${ROCKET} Your node is now part of the AIB 2.0 network!${NC}"
    echo ""
}

# ========== Main Execution ==========
main() {
    print_banner

    echo -e "${BRIGHT_CYAN}[${CYAN}1${BRIGHT_CYAN}/${CYAN}5${BRIGHT_CYAN}]${NC} ${CYAN}Downloading AIB $NETWORK node binary...${NC}"
    download_binary

    echo -e "${BRIGHT_CYAN}[${CYAN}2${BRIGHT_CYAN}/${CYAN}5${BRIGHT_CYAN}]${NC} ${CYAN}Setting up data directory...${NC}"
    setup_data_dir

    echo -e "${BRIGHT_CYAN}[${CYAN}3${BRIGHT_CYAN}/${CYAN}5${BRIGHT_CYAN}]${NC} ${CYAN}Starting node process...${NC}"
    start_node

    echo -e "${BRIGHT_CYAN}[${CYAN}4${BRIGHT_CYAN}/${CYAN}5${BRIGHT_CYAN}]${NC} ${CYAN}Verifying installation...${NC}"
    verify_installation

    echo -e "${BRIGHT_CYAN}[${CYAN}5${BRIGHT_CYAN}/${CYAN}5${BRIGHT_CYAN}]${NC} ${CYAN}Installation complete!${NC}"
    show_completion
}

# ========== Stop Node ==========
stop_node() {
    print_banner

    echo -e "${CYAN}┌─────────────────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC} ${BRIGHT_CYAN}▶ STOPPING AIB NODE${NC}                                          ${CYAN}│${NC}"
    echo -e "${CYAN}└─────────────────────────────────────────────────────────────────────┘${NC}"
    echo ""

    # Find and kill node process
    if pgrep -f "aib-node" > /dev/null; then
        log_info "${YELLOW}Found running node process${NC}"

        pkill -f "aib-node"
        sleep 2

        # Force kill if still running
        if pgrep -f "aib-node" > /dev/null; then
            log_warn "${YELLOW}Force killing...${NC}"
            pkill -9 -f "aib-node"
            sleep 1
        fi

        if ! pgrep -f "aib-node" > /dev/null; then
            log_success "${BRIGHT_CYAN}Node stopped successfully${NC}"
        else
            log_error "${RED}Failed to stop node${NC}"
            exit 1
        fi
    else
        log_warn "${YELLOW}No running node found${NC}"
    fi

    echo ""
    echo -e "${CYAN}To restart the node, run:${NC}"
    echo -e "  ${YELLOW}curl -sL https://www.aib.one/install.sh | bash -s testnet${NC}"
    echo ""
}

# ========== Uninstall Node ==========
uninstall_node() {
    print_banner

    echo -e "${RED}┌─────────────────────────────────────────────────────────────────────┐${NC}"
    echo -e "${RED}│${NC} ${BRIGHT_RED}▶ UNINSTALLING AIB NODE${NC}                                        ${RED}│${NC}"
    echo -e "${RED}└─────────────────────────────────────────────────────────────────────┘${NC}"
    echo ""

    echo -e "${YELLOW}⚠️  This will:${NC}"
    echo -e "  ${CYAN}•${NC} Stop the running node"
    echo -e "  ${CYAN}•${NC} Remove binary from $INSTALL_DIR"
    echo -e "  ${CYAN}•${NC} ${BRIGHT_RED}DELETE ALL BLOCKCHAIN DATA${NC} from $DATA_DIR"
    echo ""
    echo -e "${BRIGHT_RED}This action cannot be undone!${NC}"
    echo ""

    read -p "$(echo -e ${RED}"[?] Are you sure? Type 'yes' to confirm: "${NC})" -r confirmation
    echo ""

    if [[ "$confirmation" != "yes" ]]; then
        echo -e "${YELLOW}Uninstall cancelled${NC}"
        exit 0
    fi

    echo -e "${CYAN}┌─────────────────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC} ${BRIGHT_CYAN}▶ PHASE 1: STOPPING NODE${NC}                                        ${CYAN}│${NC}"
    echo -e "${CYAN}└─────────────────────────────────────────────────────────────────────┘${NC}"
    echo ""

    if pgrep -f "aib-node" > /dev/null; then
        pkill -f "aib-node"
        sleep 2
        pkill -9 -f "aib-node" 2>/dev/null
        log_success "${BRIGHT_CYAN}Node stopped${NC}"
    else
        log_info "${YELLOW}No running node${NC}"
    fi

    echo ""
    echo -e "${CYAN}┌─────────────────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC} ${BRIGHT_CYAN}▶ PHASE 2: REMOVING FILES${NC}                                         ${CYAN}│${NC}"
    echo -e "${CYAN}└─────────────────────────────────────────────────────────────────────┘${NC}"
    echo ""

    # Remove binary
    if [[ -d "$INSTALL_DIR" ]]; then
        rm -rf "$INSTALL_DIR"
        log_success "${BRIGHT_CYAN}Removed: $INSTALL_DIR${NC}"
    fi

    # Remove data directory
    if [[ -d "$DATA_DIR" ]]; then
        DATA_SIZE=$(du -sh "$DATA_DIR" 2>/dev/null | cut -f1)
        rm -rf "$DATA_DIR"
        log_success "${BRIGHT_CYAN}Removed: $DATA_DIR (${DATA_SIZE})${NC}"
    fi

    echo ""
    echo -e "${BRIGHT_GREEN}╔══════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BRIGHT_GREEN}║${NC}           ${BRIGHT_GREEN}AIB 2.0 NODE UNINSTALLED${NC}                              ${BRIGHT_GREEN}║${NC}"
    echo -e "${BRIGHT_GREEN}╚══════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

# ========== Show Status ==========
show_status() {
    print_banner

    echo -e "${CYAN}┌─────────────────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC} ${BRIGHT_CYAN}▶ AIB NODE STATUS${NC}                                              ${CYAN}│${NC}"
    echo -e "${CYAN}└─────────────────────────────────────────────────────────────────────┘${NC}"
    echo ""

    # Check if node is running
    if pgrep -f "aib-node" > /dev/null; then
        NODE_PID=$(pgrep -f "aib-node" | head -1)
        log_success "${BRIGHT_CYAN}Node is running (PID: $NODE_PID)${NC}"

        # Try to get status from API
        if command -v curl &> /dev/null; then
            for port in 8080 51211; do
                STATUS=$(curl -s "http://127.0.0.1:$port/v1/status" 2>/dev/null || echo "")
                if [[ -n "$STATUS" ]]; then
                    echo ""
                    echo -e "${CYAN}┌─────────────────────────────────────────────────────────────────────┐${NC}"
                    echo -e "${CYAN}║${NC} ${BRIGHT_CYAN}NODE INFORMATION${NC}                                               ${CYAN}║${NC}"
                    echo -e "${CYAN}╠════════════════════════════════════════════════════════════════════╣${NC}"

                    NETWORK=$(echo "$STATUS" | grep -o '"network":"[^"]*"' | cut -d: -f2 | tr -d '"')
                    HEIGHT=$(echo "$STATUS" | grep -o '"block_height":[0-9]*' | cut -d: -f2)
                    PEERS=$(echo "$STATUS" | grep -o '"peers":[0-9]*' | cut -d: -f2)
                    UPTIME=$(echo "$STATUS" | grep -o '"uptime":"[^"]*"' | cut -d: -f2 | tr -d '"')

                    echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}Network:${NC}     ${CYAN}$NETWORK${NC}$(printf '%*s' $((66 - 9 - ${#NETWORK})) '')${CYAN}║${NC}"
                    echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}API Port:${NC}    ${CYAN}$port${NC}$(printf '%*s' $((66 - 9 - ${#port})) '')${CYAN}║${NC}"
                    echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}Block Height:${NC} ${CYAN}$HEIGHT${NC}$(printf '%*s' $((66 - 13 - ${#HEIGHT})) '')${CYAN}║${NC}"
                    echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}Peers:${NC}        ${CYAN}$PEERS${NC}$(printf '%*s' $((66 - 6 - ${#PEERS})) '')${CYAN}║${NC}"
                    echo -e "${CYAN}║${NC} ${BRIGHT_GREEN}Uptime:${NC}       ${CYAN}$UPTIME${NC}$(printf '%*s' $((66 - 7 - ${#UPTIME})) '')${CYAN}║${NC}"
                    echo -e "${CYAN}╚════════════════════════════════════════════════════════════════════╝${NC}"
                    echo ""
                    break
                fi
            done
        fi
    else
        echo -e "${RED}✗ Node is not running${NC}"
        echo ""
    fi

    # Check directories
    echo -e "${CYAN}Installation:${NC}"
    if [[ -d "$INSTALL_DIR" ]]; then
        echo -e "  ${CYAN}•${NC} Binary:  ${BRIGHT_CYAN}$INSTALL_DIR/aib-node${NC}"
    else
        echo -e "  ${GRAY}•${NC} Binary:  ${GRAY}Not installed${NC}"
    fi

    if [[ -d "$DATA_DIR" ]]; then
        DATA_SIZE=$(du -sh "$DATA_DIR" 2>/dev/null | cut -f1)
        echo -e "  ${CYAN}•${NC} Data:    ${BRIGHT_CYAN}$DATA_DIR${NC} ${GRAY}(${DATA_SIZE})${NC}"
    else
        echo -e "  ${GRAY}•${NC} Data:    ${GRAY}Not found${NC}"
    fi

    echo ""
    echo -e "${CYAN}Quick commands:${NC}"
    echo -e "  ${CYAN}•${NC} Stop node:    ${YELLOW}curl -sL https://www.aib.one/install.sh | bash -s stop${NC}"
    echo -e "  ${CYAN}•${NC} Uninstall:    ${YELLOW}curl -sL https://www.aib.one/install.sh | bash -s uninstall${NC}"
    echo -e "  ${CYAN}•${NC} View logs:    ${YELLOW}tail -f $DATA_DIR/node.log${NC}"
    echo ""
}

# ========== Main Execution ==========
if [[ $# -gt 0 ]]; then
    case "$1" in
        testnet|mainnet)
            NETWORK="$1"
            ;;
        stop)
            stop_node
            exit 0
            ;;
        uninstall)
            uninstall_node
            exit 0
            ;;
        status)
            show_status
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown command: $1${NC}"
            echo ""
            echo -e "${CYAN}Available commands:${NC}"
            echo -e "  ${GREEN}testnet|mainnet${NC}  - Install node on specified network"
            echo -e "  ${GREEN}stop${NC}           - Stop the running node"
            echo -e "  ${GREEN}uninstall${NC}      - Remove the node completely"
            echo -e "  ${GREEN}status${NC}         - Show node status"
            echo ""
            echo -e "${CYAN}Examples:${NC}"
            echo -e "  ${YELLOW}curl -sL https://www.aib.one/install.sh | bash -s testnet${NC}"
            echo -e "  ${YELLOW}curl -sL https://www.aib.one/install.sh | bash -s stop${NC}"
            echo -e "  ${YELLOW}curl -sL https://www.aib.one/install.sh | bash -s uninstall${NC}"
            exit 1
            ;;
    esac
fi

main
