#!/bin/bash
#
# ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
# ░                                                                  ░
# ░   ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄   ░
# ░   █▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ AIB 2.0 NODE INSTALLER ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓█   ░
# ░   █                                                               █   ░
# ░   █   > curl -sL https://www.aib.one/install.sh | bash -s testnet   █   ░
# ░   █                                                               █   ░
# ░   █   [testnet|mainnet|stop|uninstall|status]                      █   ░
# ░   ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀   ░
# ░                                                                  ░
# ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
#

set -e

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# ⚡ CYBERPANK COLOR PALETTE
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
BLACK='\033[0;38;5;0m'
RED='\033[0;38;5;196m'
GREEN='\033[0;38;5;46m'
YELLOW='\033[0;38;5;226m'
BLUE='\033[0;38;5;21m'
MAGENTA='\033[0;38;5;201m'
CYAN='\033[0;38;5;51m'
WHITE='\033[0;38;5;255m'

BRIGHT_RED='\033[1;38;5;196m'
BRIGHT_GREEN='\033[1;38;5;46m'
BRIGHT_YELLOW='\033[1;38;5;226m'
BRIGHT_CYAN='\033[1;38;5;51m'
BRIGHT_MAGENTA='\033[1;38;5;201m'
BRIGHT_WHITE='\033[1;38;5;255m'

NEON_GREEN='\033[38;5;82m'
NEON_CYAN='\033[38;5;50m'
NEON_PINK='\033[38;5;213m'
NEON_PURPLE='\033[38;5;141m'
NC='\033[0m'

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# ⚙️ CONFIG
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
INSTALL_DIR="$HOME/.aib-node"
DATA_DIR="$HOME/.aib"
BINARY_BASE_URL="https://www.aib.one/binaries"
NETWORK="testnet"
COMMAND="${1:-testnet}"

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🎨 CYBERPANK VISUAL EFFECTS
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

print_banner() {
    clear
    echo -e "${NEON_CYAN}"
    cat <<'EOF'

     █████╗    ██╗   ██████╗
    ██╔══██╗   ██║   ██╔══██╗
    ███████║   ██║   ██████╔╝
    ██╔══██║   ██║   ██╔══██╗
    ██║  ██║   ██║   ██████╔╝
    ╚═╝  ╚═╝   ╚═╝   ╚═════╝

        ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        AI-NATIVE BLOCKCHAIN PROTOCOL v2.0
        ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
EOF
    echo -e "${NC}"
}

print_glitch() {
    local text="$1"
    local delay=0.05
    for i in {1..3}; do
        echo -ne "\r${NEON_PINK}${text}${NC}   "
        sleep $delay
        echo -ne "\r${NEON_CYAN}${text}${NC}   "
        sleep $delay
        echo -ne "\r${BRIGHT_WHITE}${text}${NC}   "
        sleep $delay
    done
    echo -e "\r${NEON_GREEN}${text} ✓${NC}"
}

print_header() {
    echo ""
    echo -e "${NEON_CYAN}┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓${NC}"
    echo -e "${NEON_CYAN}┃${NC} ${BRIGHT_WHITE}$1${NC}"
    echo -e "${NEON_CYAN}┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛${NC}"
}

print_progress() {
    local current=$1
    local total=$2
    local width=50
    local percentage=$((current * 100 / total))
    local filled=$((width * current / total))
    local empty=$((width - filled))

    printf "\r${NEON_GREEN}[${NC}"
    printf "%${filled}s" | tr ' ' '█'
    printf "%${empty}s" | tr ' ' '░'
    printf "${NEON_GREEN}]${NC} ${BRIGHT_WHITE}%3d%%${NC}" $percentage
}

print_status() {
    local status="$1"
    local message="$2"
    case "$status" in
        "ok")   echo -e "${NEON_GREEN}[✓]${NC} $message" ;;
        "err")  echo -e "${BRIGHT_RED}[✗]${NC} $message" ;;
        "info") echo -e "${NEON_CYAN}[ℹ]${NC} $message" ;;
        "warn") echo -e "${BRIGHT_YELLOW}[!]${NC} $message" ;;
        "load") echo -e "${NEON_PURPLE}[⟳]${NC} $message" ;;
    esac
}

matrix_rain() {
    local lines=10
    for i in $(seq 1 $lines); do
        local line=""
        for j in $(seq 1 80); do
            if (( RANDOM % 10 == 0 )); then
                line+="${NEON_GREEN}$((RANDOM % 2))${NC} "
            else
                line+="  "
            fi
        done
        echo -e "\r$line"
        sleep 0.02
    done
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 📥 DOWNLOAD
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
download_binary() {
    print_header "📡 ACQUIRING NEURAL LINK..."

    mkdir -p "$INSTALL_DIR"

    ARCH=$(uname -m)
    [[ "$ARCH" == "x86_64" ]] && BINARY_ARCH="amd64" || BINARY_ARCH="arm64"

    print_status "info" "Detecting architecture: ${BRIGHT_CYAN}${BINARY_ARCH}${NC}"
    sleep 0.5

    BINARY_FILE="aib-node-linux-${BINARY_ARCH}"
    BINARY_URL="${BINARY_BASE_URL}/${BINARY_FILE}"

    print_status "load" "Establishing secure connection to ${NEON_CYAN}www.aib.one${NC}"

    # Animated download
    if command -v wget &> /dev/null; then
        wget -q --show-progress "$BINARY_URL" -O "$INSTALL_DIR/aib-node" 2>&1 | \
        while read -r line; do
            if [[ $line =~ ([0-9]+%) ]]; then
                local pct="${BASH_REMATCH[1]}"
                printf "\r${NEON_PURPLE}[DOWNLOAD]${NC} ${NEON_CYAN}%s${NC}" "$pct"
            fi
        done
    elif command -v curl &> /dev/null; then
        curl -sL "$BINARY_URL" -o "$INSTALL_DIR/aib-node" \
            --progress-bar \
            -w "\n" 2>&1
    else
        print_status "err" "Neither wget nor curl found. Install one first."
        exit 1
    fi

    echo ""
    chmod +x "$INSTALL_DIR/aib-node"
    print_status "ok" "Binary installed: ${BRIGHT_CYAN}${INSTALL_DIR}/aib-node${NC}"
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🚀 START NODE
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
start_node() {
    print_header "🚀 INITIALIZING NODE SEQUENCE..."

    print_status "info" "Network: ${NEON_CYAN}${NETWORK^^}${NC}"

    # Stop existing
    if pgrep -f "aib-node" > /dev/null; then
        print_status "info" "Terminating existing instance..."
        pkill -f "aib-node" 2>/dev/null || true
        sleep 1
    fi

    mkdir -p "$DATA_DIR"

    # Network config
    if [[ "$NETWORK" == "mainnet" ]]; then
        P2P_PORT=31415
        BLOCK_TIME=60
        NET_COLOR="${BRIGHT_RED}"
    else
        P2P_PORT=51413
        BLOCK_TIME=30
        NET_COLOR="${NEON_GREEN}"
    fi

    print_status "info" "P2P Port: ${BRIGHT_CYAN}${P2P_PORT}${NC} | Block Time: ${BRIGHT_CYAN}${BLOCK_TIME}s${NC}"

    echo ""
    print_status "load" "Spawning validator process..."
    sleep 0.5

    nohup "$INSTALL_DIR/aib-node" \
        --network="$NETWORK" \
        --data-dir="$DATA_DIR" \
        --api-port=8080 \
        --p2p-port=$P2P_PORT \
        --validator \
        --block-time=$BLOCK_TIME \
        > "$DATA_DIR/node.log" 2>&1 &

    # Animated startup
    echo -n "  "
    for i in {1..20}; do
        echo -ne "${NEON_CYAN}▓${NC}"
        sleep 0.1
    done
    echo ""

    sleep 2

    if pgrep -f "aib-node" > /dev/null; then
        local PID=$(pgrep -f "aib-node" | head -1)
        print_status "ok" "Node active (PID: ${NEON_CYAN}${PID}${NC})"
        echo ""
        echo -e "  ${NEON_PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo -e "  ${BRIGHT_WHITE}📊 Monitor:  ${NEON_CYAN}tail -f ${DATA_DIR}/node.log${NC}"
        echo -e "  ${BRIGHT_WHITE}📡 Status:   ${NEON_CYAN}curl http://127.0.0.1:8080/v1/status${NC}"
        echo -e "  ${NEON_PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo ""
    else
        print_status "err" "Failed to start. Check logs: ${DATA_DIR}/node.log"
        exit 1
    fi
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🛑 STOP NODE
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
stop_node() {
    print_header "🛑 TERMINATION SEQUENCE"

    if pgrep -f "aib-node" > /dev/null; then
        print_status "info" "Sending SIGTERM to aib-node..."
        pkill -f "aib-node"
        sleep 2

        if pgrep -f "aib-node" > /dev/null; then
            print_status "warn" "Forcing termination..."
            pkill -9 -f "aib-node" 2>/dev/null || true
        fi

        print_status "ok" "Node terminated"
    else
        print_status "warn" "No active node found"
    fi
    echo ""
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 📊 STATUS
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
show_status() {
    print_header "📊 NODE STATUS"

    if pgrep -f "aib-node" > /dev/null; then
        local PID=$(pgrep -f "aib-node" | head -1)
        local UPTIME=$(ps -p $PID -o etime= 2>/dev/null | tr -d ' ')

        echo -e "  ${NEON_GREEN}●${NC} ${BRIGHT_WHITE}Node Running${NC}"
        echo -e "    PID:     ${NEON_CYAN}${PID}${NC}"
        echo -e "    Uptime:  ${NEON_CYAN}${UPTIME}${NC}"

        if command -v curl &> /dev/null; then
            local STATUS=$(curl -s "http://127.0.0.1:8080/v1/status" 2>/dev/null || echo "")
            if [[ -n "$STATUS" ]]; then
                local HEIGHT=$(echo "$STATUS" | grep -o '"block_height":[0-9]*' | cut -d: -f2)
                local PEERS=$(echo "$STATUS" | grep -o '"peers":[0-9]*' | cut -d: -f2)
                local CHAIN=$(echo "$STATUS" | grep -o '"chain_id":"[^"]*"' | cut -d'"' -f4)

                echo ""
                echo -e "  ${BRIGHT_WHITE}Blockchain State:${NC}"
                echo -e "    Chain:   ${NEON_PURPLE}${CHAIN}${NC}"
                echo -e "    Height:  ${NEON_CYAN}${HEIGHT}${NC}"
                echo -e "    Peers:   ${NEON_CYAN}${PEERS}${NC}"
            fi
        fi
    else
        echo -e "  ${BRIGHT_RED}●${NC} ${BRIGHT_WHITE}Node Offline${NC}"
    fi
    echo ""
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🗑️ UNINSTALL
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
uninstall_node() {
    print_header "⚠️  UNINSTALL PROTOCOL"

    echo ""
    echo -e "${BRIGHT_RED}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${BRIGHT_RED}  WARNING: This will DELETE ALL blockchain data!          ${NC}"
    echo -e "${BRIGHT_RED}═══════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -ne "  Type '${BRIGHT_YELLOW}yes${NC}' to confirm destruction: "
    read -r confirmation
    echo ""

    if [[ "$confirmation" != "yes" ]]; then
        print_status "info" "Aborted."
        exit 0
    fi

    print_status "load" "Terminating node..."
    pkill -f "aib-node" 2>/dev/null || true

    print_status "load" "Purging binaries..."
    rm -rf "$INSTALL_DIR"

    print_status "load" "Purging blockchain data..."
    rm -rf "$DATA_DIR"

    print_status "ok" "Node completely uninstalled"
    echo ""
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🎯 MAIN INSTALL
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
install_node() {
    print_banner
    sleep 0.3

    print_glitch "INITIALIZING INSTALLATION PROTOCOL"
    echo ""

    download_binary
    echo ""

    start_node

    # Verify
    print_header "🔍 VERIFICATION"
    sleep 1

    if command -v curl &> /dev/null; then
        local STATUS=$(curl -s "http://127.0.0.1:8080/v1/status" 2>/dev/null || echo "")
        if [[ -n "$STATUS" ]]; then
            print_status "ok" "Node responsive on API port 8080"
            echo ""

            echo -e "${NEON_PURPLE}╔════════════════════════════════════════════════════════════╗${NC}"
            echo -e "${NEON_PURPLE}║${NC} ${BRIGHT_WHITE}Installation Complete! Node is LIVE.${NC}                    ${NEON_PURPLE}║${NC}"
            echo -e "${NEON_PURPLE}╠════════════════════════════════════════════════════════════╣${NC}"
            echo -e "${NEON_PURPLE}║${NC} ${BRIGHT_WHITE}Quick Commands:${NC}"
            echo -e "${NEON_PURPLE}║${NC}   Stop:    ${NEON_CYAN}curl -sL https://www.aib.one/install.sh | bash -s stop${NC}"
            echo -e "${NEON_PURPLE}║${NC}   Status:  ${NEON_CYAN}curl -sL https://www.aib.one/install.sh | bash -s status${NC}"
            echo -e "${NEON_PURPLE}║${NC}   CLI:     ${NEON_CYAN}aib-cli node status${NC}"
            echo -e "${NEON_PURPLE}╚════════════════════════════════════════════════════════════╝${NC}"
        fi
    fi
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🔀 COMMAND ROUTER
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
case "$COMMAND" in
    testnet|mainnet)
        NETWORK="$COMMAND"
        install_node
        ;;
    stop)
        print_banner
        stop_node
        ;;
    uninstall)
        print_banner
        uninstall_node
        ;;
    status)
        print_banner
        show_status
        ;;
    *)
        print_banner
        echo -e "${BRIGHT_WHITE}Usage:${NC}"
        echo -e "  ${NEON_CYAN}testnet${NC}    - Install testnet node (default)"
        echo -e "  ${NEON_CYAN}mainnet${NC}    - Install mainnet node"
        echo -e "  ${NEON_CYAN}stop${NC}       - Stop running node"
        echo -e "  ${NEON_CYAN}status${NC}     - Show node status"
        echo -e "  ${NEON_CYAN}uninstall${NC}  - Remove node completely"
        echo ""
        exit 1
        ;;
esac

exit 0
