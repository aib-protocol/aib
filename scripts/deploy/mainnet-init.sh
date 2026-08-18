#!/bin/bash
#
# AIB 2.0 mainnet initialization script
# Usage: ./mainnet-init.sh [options]
# Options:
#   -n, --network NETWORK    specify the network: mainnet|testnet (default: mainnet)
#   -i, --node-id NODE_ID    specify the node ID
#   -p, --port PORT          specify API port (default: 51200)
#   -m, --moniker NODE_NAME  specify the node name
#   -s, --stake AMOUNT       stake amount
#   -d, --data-dir DIR       data directory
#   -f, --force              force initialization (overwrites existing data）
#   -h, --help               show help information
#
# Features:
#   1. create the necessary directory structure
#   2. generate node keys
#   3. config genesis file
#   4. set up the initial validator
#   5. config P2P network
#   6. start the node service
#
# Author: AIB Protocol Team
# Updated: 2026-03
#

set -e
set -u
set -o pipefail

# ========== script metadata ==========
SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
VERSION="2.0.0"

# ========== colored output ==========
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# ========== logging functions ==========
log_info() {
    echo -e "${BLUE}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*" >&2
}

log_step() {
    echo -e "${CYAN}[STEP]${NC} $*"
}

# ========== config variables ==========
NETWORK="mainnet"
NODE_ID=""
API_PORT="51200"
P2P_PORT="26656"
MONIKER=""
STAKE_AMOUNT="1000000"
DATA_DIR="${PROJECT_DIR}/data"
CONFIG_DIR="${PROJECT_DIR}/config"
BINARY_DIR="${PROJECT_DIR}/bin"
GENESIS_DIR="${SCRIPT_DIR}/../genesis"
SERVICE_NAME="aib2-mainnet"
FORCE_INIT=false

# Genesis config (mainnet)
MAINNET_CHAIN_ID="aib-mainnet-1"
MAINNET_GENESIS_TIME="2026-03-14T00:00:00Z"
MAINNET_TOTAL_SUPPLY="3141592653"
MAINNET_BLOCK_REWARD=50
MAINNET_BLOCK_TIME=30
MAINNET_MAX_VALIDATORS=100
MAINNET_MIN_STAKE="1000000"
MAINNET_UNBONDING_TIME="604800"

# Testnet config
TESTNET_CHAIN_ID="aib-testnet-1"
TESTNET_GENESIS_TIME="2026-01-01T00:00:00Z"
TESTNET_TOTAL_SUPPLY="1000000000"
TESTNET_BLOCK_REWARD=10
TESTNET_BLOCK_TIME=15
TESTNET_MAX_VALIDATORS=21
TESTNET_MIN_STAKE="10000"
TESTNET_UNBONDING_TIME="3600"

# ========== help information ==========
show_help() {
    cat << EOF
AIB 2.0 mainnet initialization script v${VERSION}

Usage: ${SCRIPT_NAME} [options]

Options:
  -n, --network NETWORK    specify the network: mainnet|testnet (default: mainnet)
  -i, --node-id NODE_ID    specify the node ID (optional, auto-generated)
  -p, --port PORT          specify API port (default: 51200)
  -m, --moniker NAME       specify the node name
  -s, --stake AMOUNT       initial stake amount (default: 1000000)
  -d, --data-dir DIR       data directory
  -c, --config-dir DIR     config directory
  -f, --force              force initialization (overwrites existing data）
  -h, --help               show help information

Examples:
  # initialize a mainnet node
  ${SCRIPT_NAME} -m "my-validator" -s 1000000

  # initialize a testnet node
  ${SCRIPT_NAME} -n testnet -m "test-validator"

  # specify a custom port
  ${SCRIPT_NAME} -p 51201 -m "secondary-node"

initialization steps:
  1. create directory structure
  2. generate node keys
  3. config genesis file
  4. create the initial validator
  5. config P2P network
  6. settings systemd service
  7. start the node

Note: the first initialization requires enough staking tokens
for more information visit: https://docs.aib.network/mainnet
EOF
    exit 0
}

# ========== parse arguments ==========
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -n|--network)
                NETWORK="$2"
                shift 2
                ;;
            -i|--node-id)
                NODE_ID="$2"
                shift 2
                ;;
            -p|--port)
                API_PORT="$2"
                shift 2
                ;;
            --p2p-port)
                P2P_PORT="$2"
                shift 2
                ;;
            -m|--moniker)
                MONIKER="$2"
                shift 2
                ;;
            -s|--stake)
                STAKE_AMOUNT="$2"
                shift 2
                ;;
            -d|--data-dir)
                DATA_DIR="$2"
                shift 2
                ;;
            -c|--config-dir)
                CONFIG_DIR="$2"
                shift 2
                ;;
            -f|--force)
                FORCE_INIT=true
                shift
                ;;
            -h|--help)
                show_help
                ;;
            *)
                log_error "unknown argument: $1"
                show_help
                ;;
        esac
    done
}

# ========== environment check ==========
check_environment() {
    log_info "check the runtime environment..."

    # check required commands
    local required_commands=("openssl" "jq" "curl")
    for cmd in "${required_commands[@]}"; do
        if ! command -v "$cmd" &> /dev/null; then
            log_error "missing required command: $cmd"
            return 1
        fi
    done

    # check the binary file
    if [[ ! -f "${BINARY_DIR}/aib-node" ]]; then
        log_error "node binary does not exist: ${BINARY_DIR}/aib-node"
        log_info "please build or download the node binary first"
        return 1
    fi

    # check permissions
    if [[ ! -w "${PROJECT_DIR}" ]]; then
        log_error "no write permission on the project directory: ${PROJECT_DIR}"
        return 1
    fi

    log_success "environment check passed"
    return 0
}

# ========== get network config ==========
get_network_config() {
    if [[ "${NETWORK}" == "mainnet" ]]; then
        CHAIN_ID="${MAINNET_CHAIN_ID}"
        GENESIS_TIME="${MAINNET_GENESIS_TIME}"
        TOTAL_SUPPLY="${MAINNET_TOTAL_SUPPLY}"
        BLOCK_REWARD="${MAINNET_BLOCK_REWARD}"
        BLOCK_TIME="${MAINNET_BLOCK_TIME}"
        MAX_VALIDATORS="${MAINNET_MAX_VALIDATORS}"
        MIN_STAKE="${MAINNET_MIN_STAKE}"
        UNBONDING_TIME="${MAINNET_UNBONDING_TIME}"
    else
        CHAIN_ID="${TESTNET_CHAIN_ID}"
        GENESIS_TIME="${TESTNET_GENESIS_TIME}"
        TOTAL_SUPPLY="${TESTNET_TOTAL_SUPPLY}"
        BLOCK_REWARD="${TESTNET_BLOCK_REWARD}"
        BLOCK_TIME="${TESTNET_BLOCK_TIME}"
        MAX_VALIDATORS="${TESTNET_MAX_VALIDATORS}"
        MIN_STAKE="${TESTNET_MIN_STAKE}"
        UNBONDING_TIME="${TESTNET_UNBONDING_TIME}"
    fi
}

# ========== create directory structure ==========
create_directories() {
    log_step "create directory structure..."

    local dirs=(
        "${DATA_DIR}"
        "${DATA_DIR}/wallet"
        "${DATA_DIR}/keystore"
        "${CONFIG_DIR}"
        "${PROJECT_DIR}/logs"
    )

    for dir in "${dirs[@]}"; do
        mkdir -p "${dir}"
        log_info "createdirectory: ${dir}"
    done

    log_success "directory created"
}

# ========== generate node keys ==========
generate_keys() {
    log_step "generate node keys..."

    local priv_key_file="${DATA_DIR}/priv_validator_key.json"
    local node_key_file="${DATA_DIR}/node_key.json"

    if [[ -f "${priv_key_file}" && "${FORCE_INIT}" != "true" ]]; then
        log_warning "node keys already exist, skipping generation"
    else
        # generate validator private key
        if [[ ! -f "${priv_key_file}" ]]; then
            # generate a random private key with openssl
            local private_key=$(openssl rand -hex 32)
            cat > "${priv_key_file}" << EOF
{
  "address": "$(echo -n "${private_key}" | sha256sum | cut -d' ' -f1 | head -c 40)",
  "pub_key": {
    "type": "tendermint/PubKeySecp256k1",
    "value": "$(echo -n "${private_key}" | cut -c 3-66 | xxd -r -p | openssl ec -inform DER -pubout 2>/dev/null | base64 -w 0)"
  },
  "priv_key": {
    "type": "tendermint/PrivKeySecp256k1",
    "value": "${private_key}"
  }
}
EOF
            log_info "generate validator keys: ${priv_key_file}"
        fi

        # generate node keys
        if [[ ! -f "${node_key_file}" ]]; then
            local node_private=$(openssl rand -hex 32)
            cat > "${node_key_file}" << EOF
{
  "priv_key": {
    "type": "tendermint/PrivKeySecp256k1",
    "value": "${node_private}"
  }
}
EOF
            log_info "generate node keys: ${node_key_file}"
        fi
    fi

    # generatenode ID
    if [[ -z "${NODE_ID}" ]]; then
        NODE_ID=$(cat "${node_key_file}" 2>/dev/null | jq -r '.priv_key.value' | sha256sum | cut -d' ' -f1 | head -c 40)
    fi

    log_success "node ID: ${NODE_ID}"
    return 0
}

# ========== generate genesis file ==========
generate_genesis() {
    log_step "generate Genesis file..."

    local genesis_file="${CONFIG_DIR}/genesis.json"

    # check whether there is already genesis
    if [[ -f "${genesis_file}" && "${FORCE_INIT}" != "true" ]]; then
        log_warning "Genesis file already exists: ${genesis_file}"
        read -p "whether to overwrite? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "use existing genesis file"
            return 0
        fi
    fi

    # copy from template or create a new one genesis
    if [[ -f "${GENESIS_DIR}/genesis.json" ]]; then
        log_info "use genesis template..."
        cp "${GENESIS_DIR}/genesis.json" "${genesis_file}"
    else
        log_info "createnew' genesis file..."
        cat > "${genesis_file}" << EOF
{
  "chain_id": "${CHAIN_ID}",
  "genesis_time": "${GENESIS_TIME}",
  "total_supply": "${TOTAL_SUPPLY}",
  "block_reward": ${BLOCK_REWARD},
  "block_time": ${BLOCK_TIME},
  "validators": [],
  "allocations": {
    "team": {
      "amount": "$(echo "${TOTAL_SUPPLY} * 0.15" | bc | cut -d'.' -f1)",
      "percentage": "15"
    },
    "ecosystem": {
      "amount": "$(echo "${TOTAL_SUPPLY} * 0.30" | bc | cut -d'.' -f1)",
      "percentage": "30"
    },
    "staking_rewards": {
      "amount": "$(echo "${TOTAL_SUPPLY} * 0.40" | bc | cut -d'.' -f1)",
      "percentage": "40"
    },
    "community": {
      "amount": "$(echo "${TOTAL_SUPPLY} * 0.10" | bc | cut -d'.' -f1)",
      "percentage": "10"
    },
    "airdrop_pool": {
      "amount": "$(echo "${TOTAL_SUPPLY} * 0.05" | bc | cut -d'.' -f1)",
      "percentage": "5"
    }
  },
  "airdrop": {
    "enabled": true,
    "amount_per_address": 100,
    "conditions": {
      "min_balance": 0,
      "must_claim": true,
      "claim_deadline": "2026-04-14T00:00:00Z"
    },
    "exclusions": ["team_addresses", "contract_addresses"]
  },
  "config": {
    "max_validators": ${MAX_VALIDATORS},
    "min_stake_amount": "${MIN_STAKE}",
    "unbonding_time": "${UNBONDING_TIME}"
  }
}
EOF
    fi

    # settings genesis hash
    local genesis_hash=$(sha256sum "${genesis_file}" | awk '{print $1}')
    log_info "Genesis hash: ${genesis_hash}"

    log_success "Genesis file generated"
    return 0
}

# ========== create the config file ==========
create_config() {
    log_step "create the config file..."

    local config_file="${CONFIG_DIR}/config.toml"

    cat > "${config_file}" << EOF
# AIB 2.0 Node Configuration
# Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)

# ========== node config ==========
[node]
  # node name
  moniker = "${MONIKER}"
  # node type: validator, full, light
  node_type = "validator"
  # data directory
  data_dir = "${DATA_DIR}"
  # log level
  log_level = "info"

# ========== API config ==========
[api]
  # API listen address
  listen_address = "0.0.0.0:${API_PORT}"
  # enable API
  enabled = true
  # allow cross-origin
  enable_cors = true
  # API rate limiting
  rate_limit = 100

# ========== P2P config ==========
[p2p]
  # P2P listen address
  listen_address = "0.0.0.0:${P2P_PORT}"
  # external address
  external_address = ""
  # seed nodes
  seeds = []
  # persistent nodes
  persistent_peers = []
  # max connections
  max_connections = 100

# ========== consensus config ==========
[consensus]
  # block time (seconds）
  block_time = ${BLOCK_TIME}
  # validator count
  validators = []
  # minimum stake
  min_stake = "${MIN_STAKE}"
  # unbonding time (seconds)
  unbonding_time = ${UNBONDING_TIME}

# ========== genesis config ==========
[genesis]
  # chain ID
  chain_id = "${CHAIN_ID}"
  # Genesis file path
  genesis_file = "${CONFIG_DIR}/genesis.json"

# ========== monitoring config ==========
[telemetry]
  # enable telemetry
  enabled = true
  # Prometheus port
  prometheus_port = 26660
EOF

    log_success "config file created: ${config_file}"
    return 0
}

# ========== create systemd service ==========
create_systemd_service() {
    log_step "create systemd service..."

    local service_file="/etc/systemd/system/${SERVICE_NAME}.service"

    if [[ -f "${service_file}" ]]; then
        log_warning "service file already exists: ${service_file}"
    else
        cat > "${service_file}" << EOF
[Unit]
Description=AIB 2.0 Mainnet Node
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$(whoami)
WorkingDirectory=${PROJECT_DIR}
ExecStart=${BINARY_DIR}/aib-node \
  --config ${CONFIG_DIR}/config.toml \
  --data-dir ${DATA_DIR} \
  --api-port ${API_PORT} \
  --p2p-port ${P2P_PORT} \
  --moniker "${MONIKER}"
Restart=on-failure
RestartSec=10
TimeoutStopSec=60
LimitNOFILE=65535

# environment variables
Environment="HOME=${HOME}"
Environment="PORT=${API_PORT}"

# logging config
StandardOutput=append:${PROJECT_DIR}/logs/mainnet.log
StandardError=append:${PROJECT_DIR}/logs/mainnet.error.log

[Install]
WantedBy=multi-user.target
EOF
        log_success "service file created: ${service_file}"
    fi

    # reload systemd
    if command -v systemctl &> /dev/null; then
        systemctl daemon-reload
        log_info "systemd config reloaded"
    fi

    return 0
}

# ========== start the node ==========
start_node() {
    log_step "start the node service..."

    if command -v systemctl &> /dev/null; then
        # enable the service
        systemctl enable "${SERVICE_NAME}"

        # start the service
        systemctl start "${SERVICE_NAME}"

        # waiting to start
        sleep 3

        # checkstatus
        if systemctl is-active --quiet "${SERVICE_NAME}"; then
            log_success "node service started"
        else
            log_error "node service failed to start"
            journalctl -u "${SERVICE_NAME}" --no-pager -n 50
            return 1
        fi
    else
        log_warning "systemd not installed, start in foreground mode"
        log_info "Run: ${BINARY_DIR}/aib-node --config ${CONFIG_DIR}/config.toml"
        # do not start here, leave it to the user
    fi

    return 0
}

# ========== validate initialization ==========
verify_init() {
    log_step "validate initialization result..."

    sleep 5

    # check service status
    if command -v systemctl &> /dev/null; then
        if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
            log_error "service is not running"
            return 1
        fi
    fi

    # check API
    local api_url="http://localhost:${API_PORT}/health"
    if command -v curl &> /dev/null; then
        if curl -sf "${api_url}" > /dev/null 2>&1; then
            log_success "API health check passed"
        else
            log_warning "API health check failed"
        fi
    fi

    # get node info
    local node_info_url="http://localhost:${API_PORT}/node_info"
    if command -v curl &> /dev/null; then
        local node_info=$(curl -s "${node_info_url}" 2>/dev/null)
        if [[ -n "${node_info}" ]]; then
            log_info "node info: ${node_info}"
        fi
    fi

    log_success "initialization validation complete"
    return 0
}

# ========== print summary ==========
print_summary() {
    echo
    echo "========================================"
    echo "  AIB 2.0 mainnet initialization complete"
    echo "========================================"
    echo
    echo "network: ${NETWORK}"
    echo "chain ID: ${CHAIN_ID}"
    echo "node ID: ${NODE_ID}"
    echo "node name: ${MONIKER}"
    echo "API port: ${API_PORT}"
    echo "P2P port: ${P2P_PORT}"
    echo
    echo "data directory: ${DATA_DIR}"
    echo "config directory: ${CONFIG_DIR}"
    echo
    echo "service name: ${SERVICE_NAME}"
    echo
    echo "common commands:"
    echo "  view status: systemctl status ${SERVICE_NAME}"
    echo "  view logs: journalctl -u ${SERVICE_NAME} -f"
    echo "  restart the service: systemctl restart ${SERVICE_NAME}"
    echo "  Stop service: systemctl stop ${SERVICE_NAME}"
    echo
    echo "API address: http://localhost:${API_PORT}"
    echo "chain ID: ${CHAIN_ID}"
    echo
    echo "========================================"
}

# ========== main flow ==========
main() {
    echo
    echo -e "${GREEN}AIB 2.0 mainnet initialization script${NC}"
    echo -e "${YELLOW}version: ${VERSION}${NC}"
    echo

    # parse arguments
    parse_args "$@"

    # check required parameters
    if [[ -z "${MONIKER}" ]]; then
        log_error "node name must be specified (-m, --moniker)"
        show_help
    fi

    # environment check
    check_environment || exit 1

    # get network config
    get_network_config

    log_info "network: ${NETWORK}"
    log_info "chain ID: ${CHAIN_ID}"

    # confirm initialization
    if [[ "${FORCE_INIT}" != "true" ]]; then
        echo
        echo -e "${YELLOW}about to initialize ${NETWORK} node${NC}"
        echo -e "node name: ${MONIKER}"
        echo -e "API port: ${API_PORT}"
        echo
        read -p "Continue? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "initialization cancelled"
            exit 0
        fi
    fi

    # execute initialization steps
    create_directories || exit 1
    generate_keys || exit 1
    generate_genesis || exit 1
    create_config || exit 1
    create_systemd_service || exit 1

    # ask whether to start
    if [[ "${FORCE_INIT}" == "true" || -t 1 ]]; then
        read -p "whether to start the node service? (Y/n) " -n 1 -r
        echo
        if [[ -z $REPLY || $REPLY =~ ^[Yy]$ ]]; then
            start_node || exit 1
            verify_init || exit 1
        fi
    fi

    # print summary
    print_summary

    log_success "initialization complete!"
    exit 0
}

# ========== Entry Point ==========
main "$@"
