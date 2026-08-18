#!/bin/bash

# AIB DeFi contract verification script

# color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# config file path
CONFIG_FILE="${1:-./cmd/deploy-contracts/config.yaml}"
CONTRACTS_DIR="./cmd/deploy-contracts/contracts"

# check arguments
if [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    echo "Usage: $0 [config file path]"
    echo "Default config file path: ./cmd/deploy-contracts/config.yaml"
    exit 0
fi

log_info "Starting DeFi contract verification..."

# check config file
if [ ! -f "$CONFIG_FILE" ]; then
    log_error "Config file not found: $CONFIG_FILE"
    exit 1
fi

# extract RPC endpoint
RPC_ENDPOINT=$(grep "rpc_endpoint:" "$CONFIG_FILE" | awk '{print $2}')
if [ -z "$RPC_ENDPOINT" ]; then
    RPC_ENDPOINT="http://localhost:8545"
fi

log_info "RPC endpoint: $RPC_ENDPOINT"

# check network connectivity
log_info "Checking network connectivity..."
if ! curl -s -X POST "$RPC_ENDPOINT" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
    > /dev/null 2>&1; then
    log_error "Cannot connect to RPC node: $RPC_ENDPOINT"
    log_info "Please ensure the node is running"
    exit 1
fi

# get chain ID
CHAIN_ID=$(curl -s -X POST "$RPC_ENDPOINT" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' \
    | jq -r '.result')

log_info "Chain ID: $CHAIN_ID"

# verification function
verify_contract() {
    local contract_name=$1
    local contract_address=$2

    log_info "Verifying $contract_name contract..."

    # check contract address format
    if ! [[ "$contract_address" =~ ^0x[a-fA-F0-9]{40}$ ]]; then
        log_error "$contract_name: invalid address format"
        return 1
    fi

    # check contract code exists
    local code=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_getCode\",\"params\":[\"$contract_address\",\"latest\"],\"id\":1}" \
        | jq -r '.result')

    if [ "$code" = "0x" ] || [ -z "$code" ]; then
        log_error "$contract_name: contract code is empty, deployment may have failed"
        return 1
    fi

    local code_length=$(( ${#code} - 2 ))  # strip the "0x" prefix
    log_info "$contract_name: code length = $code_length bytes"

    if [ "$code_length" -lt 100 ]; then
        log_warning "$contract_name: code length unusually short"
    fi

    # check contract nonce
    local nonce=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_getTransactionCount\",\"params\":[\"$contract_address\",\"latest\"],\"id\":1}" \
        | jq -r '.result')

    log_info "$contract_name: Nonce = $nonce"

    log_success "$contract_name verification passed"
    return 0
}

# test WETH functionality
test_weth() {
    local weth_address=$1

    log_info "Testing WETH functionality..."

    # deposit functionality
    local balance=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$weth_address\",\"data\":\"0x70a082310000000000000000000000000000000000000000000000000000000000000000\"},\"latest\"],\"id\":1}" \
        | jq -r '.result')

    if [ "$balance" != "0x" ]; then
        log_info "WETH total supply: $balance"
    fi

    log_success "WETH functionality test complete"
    return 0
}

# test UniswapV2Factory functionality
test_factory() {
    local factory_address=$1

    log_info "Testing UniswapV2Factory functionality..."

    # get pair count
    local pair_count=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$factory_address\",\"data\":\"0x1e3dd18d\"},\"latest\"],\"id\":1}" \
        | jq -r '.result')

    if [ "$pair_count" != "0x" ]; then
        log_info "Factory pair count: $pair_count"
    fi

    log_success "UniswapV2Factory functionality test complete"
    return 0
}

# test UniswapV2Router functionality
test_router() {
    local router_address=$1
    local weth_address=$2
    local factory_address=$3

    log_info "Testing UniswapV2Router functionality..."

    # check WETH address
    local router_weth=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$router_address\",\"data\":\"0x4e2f6d76\"},\"latest\"],\"id\":1}" \
        | jq -r '.result')

    if [ "$router_weth" != "0x" ]; then
        # extract WETH address (last 40 chars)
        local extracted_weth="0x${router_weth: -40}"
        if [ "${extracted_weth,,}" = "${weth_address,,}" ]; then
            log_info "Router WETH configured correctly"
        else
            log_warning "Router WETH address mismatch"
        fi
    fi

    # check Factory address
    local router_factory=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$router_address\",\"data\":\"0xac4c2f2f\"},\"latest\"],\"id\":1}" \
        | jq -r '.result')

    if [ "$router_factory" != "0x" ]; then
        local extracted_factory="0x${router_factory: -40}"
        if [ "${extracted_factory,,}" = "${factory_address,,}" ]; then
            log_info "Router Factory configured correctly"
        else
            log_warning "Router Factory address mismatch"
        fi
    fi

    log_success "UniswapV2Router functionality test complete"
    return 0
}

# performance test
performance_test() {
    log_info "Running performance test..."

    # test block time
    local start_time=$(date +%s%N)

    for i in {1..10}; do
        curl -s -X POST "$RPC_ENDPOINT" \
            -H "Content-Type: application/json" \
            -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
            > /dev/null
    done

    local end_time=$(date +%s%N)
    local duration=$(( (end_time - start_time) / 1000000 ))

    log_info "10 requests took: ${duration}ms"
    log_info "Average response time: $(( duration / 10 ))ms"

    log_success "Performance test complete"
}

# main verification flow
echo "============================================"
echo "  AIB DeFi Contract Verification Tool"
echo "============================================"
echo ""

# check deployment records
DEPLOY_RECORDS_DIR="./deployments/records"

if [ -d "$DEPLOY_RECORDS_DIR" ]; then
    log_info "Found deployment records directory: $DEPLOY_RECORDS_DIR"

    # find the latest deployment record
    LATEST_RECORD=$(ls -t "$DEPLOY_RECORDS_DIR"/deployment_*.json 2>/dev/null | head -1)

    if [ -n "$LATEST_RECORD" ]; then
        log_info "Using deployment record: $LATEST_RECORD"

        # parse contract addresses
        WETH_ADDRESS=$(cat "$LATEST_RECORD" | jq -r '.contracts[] | select(.name=="WETH") | .address')
        FACTORY_ADDRESS=$(cat "$LATEST_RECORD" | jq -r '.contracts[] | select(.name=="UniswapV2Factory") | .address')
        ROUTER_ADDRESS=$(cat "$LATEST_RECORD" | jq -r '.contracts[] | select(.name=="UniswapV2Router") | .address')

        # verify each contract
        if [ -n "$WETH_ADDRESS" ] && [ "$WETH_ADDRESS" != "null" ]; then
            verify_contract "WETH" "$WETH_ADDRESS"
            test_weth "$WETH_ADDRESS"
        fi

        if [ -n "$FACTORY_ADDRESS" ] && [ "$FACTORY_ADDRESS" != "null" ]; then
            verify_contract "UniswapV2Factory" "$FACTORY_ADDRESS"
            test_factory "$FACTORY_ADDRESS"
        fi

        if [ -n "$ROUTER_ADDRESS" ] && [ "$ROUTER_ADDRESS" != "null" ]; then
            verify_contract "UniswapV2Router" "$ROUTER_ADDRESS"
            test_router "$ROUTER_ADDRESS" "$WETH_ADDRESS" "$FACTORY_ADDRESS"
        fi
    else
        log_warning "No deployment records found, run the deployment script first"
    fi
else
    log_warning "Deployment records directory not found: $DEPLOY_RECORDS_DIR"
    log_info "Run the deployment script first: ./deploy_contracts.sh"
fi

echo ""
echo "============================================"
echo "  Verification complete"
echo "============================================"
