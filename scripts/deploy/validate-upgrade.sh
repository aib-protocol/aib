#!/bin/bash
#
# AIB 2.0 upgrade validation script
# Usage: ./validate-upgrade.sh [options]
# Options:
#   -c, --check CHECKS       specify checks: all|version|consensus|api|p2p|sync (default: all)
#   -s, --service SERVICE   specify service name (default: aib2-mainnet)
#   -p, --port PORT         API port (default: 51200)
#   -t, --timeout SECONDS   timeout (default: 30)
#   -v, --verbose           verbose output
#   -h, --help              show help information
#
# Features:
#   1. validate the version upgrade
#   2. validate consensus status
#   3. validate API availability
#   4. validate P2P network
#   5. validatesync status
#   6. validate on-chain activity
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
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $*" >&2
}

log_section() {
    echo
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}  $*$NC"
    echo -e "${CYAN}========================================${NC}"
}

# ========== config variables ==========
CHECKS="all"
SERVICE_NAME="aib2-mainnet"
API_PORT="51200"
P2P_PORT="26656"
TIMEOUT=30
VERBOSE=false
BINARY_PATH="${PROJECT_DIR}/bin/aib-node"

# check results
declare -A CHECK_RESULTS
PASS_COUNT=0
FAIL_COUNT=0

# ========== help information ==========
show_help() {
    cat << EOF
AIB 2.0 upgrade validation script v${VERSION}

Usage: ${SCRIPT_NAME} [options]

Options:
  -c, --check CHECKS       specify checks: all|version|consensus|api|p2p|sync (default: all)
  -s, --service SERVICE   specify service name (default: aib2-mainnet)
  -p, --port PORT         API port (default: 51200)
  -t, --timeout SECONDS   timeout (default: 30)
  -v, --verbose           verbose output
  -h, --help              show help information

check item:
  version     - validate node version
  consensus   - validate consensus status
  api         - validate API availability
  p2p         - validate P2P network connection
  sync        - validatesync status
  chain       - validate on-chain activity

Examples:
  # validate all items
  ${SCRIPT_NAME}

  # validate version only
  ${SCRIPT_NAME} -c version

  # validate multiple items
  ${SCRIPT_NAME} -c version,consensus,api

  # custom port
  ${SCRIPT_NAME} -p 51201

  # verbose output
  ${SCRIPT_NAME} -v

Note: the node service must be running to validate
EOF
    exit 0
}

# ========== parse arguments ==========
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -c|--check)
                CHECKS="$2"
                shift 2
                ;;
            -s|--service)
                SERVICE_NAME="$2"
                shift 2
                ;;
            -p|--port)
                API_PORT="$2"
                shift 2
                ;;
            -t|--timeout)
                TIMEOUT="$2"
                shift 2
                ;;
            -v|--verbose)
                VERBOSE=true
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

# ========== API invoke ==========
api_call() {
    local endpoint="$1"
    local url="http://localhost:${API_PORT}${endpoint}"

    if command -v curl &> /dev/null; then
        curl -sf --connect-timeout "${TIMEOUT}" "${url}" 2>/dev/null
    else
        log_error "curl not installed"
        return 1
    fi
}

# ========== record result ==========
record_result() {
    local check_name="$1"
    local result="$2"
    local message="$3"

    CHECK_RESULTS["${check_name}"]="${result}"

    if [[ "${result}" == "PASS" ]]; then
        PASS_COUNT=$((PASS_COUNT + 1))
        log_success "${check_name}: ${message}"
    elif [[ "${result}" == "WARN" ]]; then
        log_warning "${check_name}: ${message}"
    else
        FAIL_COUNT=$((FAIL_COUNT + 1))
        log_error "${check_name}: ${message}"
    fi
}

# ========== check 1: version validation ==========
check_version() {
    log_section "check: version validation"

    # check binary version
    local binary_version
    if [[ -f "${BINARY_PATH}" ]]; then
        binary_version=$("${BINARY_PATH}" --version 2>&1 | head -1)
    else
        binary_version="binary_not_found"
    fi

    [[ "${VERBOSE}" == "true" ]] && log_info "binaryversion: ${binary_version}"

    # get API version
    local api_version
    api_version=$(api_call "/version" 2>/dev/null || echo "api_error")

    [[ "${VERBOSE}" == "true" ]] && log_info "API version: ${api_version}"

    # check consistency
    if [[ "${binary_version}" != *"not_found"* ]] && [[ "${api_version}" != "api_error" ]]; then
        # simple version check
        if [[ -n "${api_version}" ]]; then
            record_result "version" "PASS" "version OK (${binary_version})"
        else
            record_result "version" "WARN" "API failed to get version"
        fi
    else
        record_result "version" "FAIL" "version check failed"
    fi
}

# ========== check 2: consensus status ==========
check_consensus() {
    log_section "check: consensus status"

    # check service status
    local service_status
    if command -v systemctl &> /dev/null; then
        service_status=$(systemctl is-active "${SERVICE_NAME}" 2>/dev/null || echo "inactive")
    else
        # try to detect the process
        if pgrep -f "aib-node" > /dev/null; then
            service_status="running"
        else
            service_status="not_running"
        fi
    fi

    [[ "${VERBOSE}" == "true" ]] && log_info "service status: ${service_status}"

    if [[ "${service_status}" == "active" || "${service_status}" == "running" ]]; then
        record_result "consensus" "PASS" "node service running"
    else
        record_result "consensus" "FAIL" "node service is not running: ${service_status}"
        return 1
    fi

    # check consensus parameters
    local consensus_params
    consensus_params=$(api_call "/consensus/params" 2>/dev/null || echo "")

    [[ "${VERBOSE}" == "true" ]] && log_info "consensus parameters: ${consensus_params}"

    if [[ -n "${consensus_params}" ]]; then
        record_result "consensus_params" "PASS" "consensus parameters queryable"
    else
        record_result "consensus_params" "WARN" "failed to get consensus parameters"
    fi
}

# ========== check 3: API availability ==========
check_api() {
    log_section "check: API available "

    # check health endpoint
    local health
    health=$(api_call "/health" 2>/dev/null || echo "error")

    [[ "${VERBOSE}" == "true" ]] && log_info "health status: ${health}"

    if [[ "${health}" != "error" ]]; then
        record_result "api_health" "PASS" "health check passed"
    else
        record_result "api_health" "FAIL" "health check failed"
    fi

    # check key endpoints
    local endpoints=(
        "/node_info"
        "/status"
        "/blocks/latest"
    )

    local endpoint_count=0
    local endpoint_success=0

    for endpoint in "${endpoints[@]}"; do
        endpoint_count=$((endpoint_count + 1))
        local response
        response=$(api_call "${endpoint}" 2>/dev/null || echo "")

        if [[ -n "${response}" ]]; then
            endpoint_success=$((endpoint_success + 1))
            [[ "${VERBOSE}" == "true" ]] && log_info "endpoint ${endpoint}: OK"
        else
            [[ "${VERBOSE}" == "true" ]] && log_info "endpoint ${endpoint}: FAIL"
        fi
    done

    if [[ ${endpoint_success} -eq ${endpoint_count} ]]; then
        record_result "api_endpoints" "PASS" "all key endpoints available"
    elif [[ ${endpoint_success} -gt 0 ]]; then
        record_result "api_endpoints" "WARN" "${endpoint_success}/${endpoint_count} endpoint available"
    else
        record_result "api_endpoints" "FAIL" "all endpoints unavailable"
    fi
}

# ========== check 4: P2P network ==========
check_p2p() {
    log_section "check: P2P network"

    # get network info
    local net_info
    net_info=$(api_call "/net_info" 2>/dev/null || echo "")

    [[ "${VERBOSE}" == "true" ]] && log_info "network info: ${net_info}"

    if [[ -n "${net_info}" ]]; then
        # try to extract connection count
        local n_peers
        n_peers=$(echo "${net_info}" | jq -r '.n_peers' 2>/dev/null || echo "0")

        [[ "${VERBOSE}" == "true" ]] && log_info "peer count: ${n_peers}"

        if [[ "${n_peers}" -gt 0 ]]; then
            record_result "p2p_peers" "PASS" "connected ${n_peers} peers"
        elif [[ "${n_peers}" == "0" ]]; then
            record_result "p2p_peers" "WARN" "no connected peers"
        else
            record_result "p2p_peers" "FAIL" "failed to get peer info"
        fi
    else
        record_result "p2p_peers" "FAIL" "cannot connect to P2P network"
    fi

    # check P2P port
    if command -v nc &> /dev/null || command -v timeout &> /dev/null; then
        if nc -z localhost "${P2P_PORT}" 2>/dev/null || timeout 1 bash -c "echo > /dev/tcp/localhost/${P2P_PORT}" 2>/dev/null; then
            record_result "p2p_port" "PASS" "P2P port ${P2P_PORT} accessible"
        else
            record_result "p2p_port" "FAIL" "P2P port ${P2P_PORT} inaccessible"
        fi
    else
        [[ "${VERBOSE}" == "true" ]] && log_info "skip port check (nc/timeout unavailable)"
    fi
}

# ========== check 5: sync status ==========
check_sync() {
    log_section "check: sync status"

    # getstatus
    local status
    status=$(api_call "/status" 2>/dev/null || echo "")

    if [[ -n "${status}" ]]; then
        # check sync status
        local syncing
        syncing=$(echo "${status}" | jq -r '.syncing' 2>/dev/null || echo "unknown")

        [[ "${VERBOSE}" == "true" ]] && log_info "sync status: ${syncing}"

        if [[ "${syncing}" == "false" ]]; then
            record_result "sync_status" "PASS" "node synced"
        elif [[ "${syncing}" == "true" ]]; then
            record_result "sync_status" "WARN" "node is syncing"
        else
            record_result "sync_status" "WARN" "unable to determine sync status"
        fi

        # get block height
        local block_height
        block_height=$(echo "${status}" | jq -r '.latest_block_height' 2>/dev/null || echo "0")

        [[ "${VERBOSE}" == "true" ]] && log_info "block height: ${block_height}"

        if [[ "${block_height}" -gt 0 ]]; then
            record_result "sync_height" "PASS" "current height: ${block_height}"
        else
            record_result "sync_height" "FAIL" "failed to get block height"
        fi
    else
        record_result "sync_status" "FAIL" "failed to get sync status"
    fi
}

# ========== check 6: on-chain activity ==========
check_chain() {
    log_section "check: on-chain activity"

    # get the latest block
    local block
    block=$(api_call "/blocks/latest" 2>/dev/null || echo "")

    if [[ -n "${block}" ]]; then
        local block_height
        block_height=$(echo "${block}" | jq -r '.block.header.height' 2>/dev/null || echo "0")

        local block_time
        block_time=$(echo "${block}" | jq -r '.block.header.timestamp' 2>/dev/null || echo "")

        [[ "${VERBOSE}" == "true" ]] && log_info "latest block: ${block_height} (${block_time})"

        if [[ "${block_height}" -gt 0 ]]; then
            record_result "chain_block" "PASS" "latest block: ${block_height}"
        else
            record_result "chain_block" "FAIL" "failed to get block info"
        fi

        # check transaction count
        local tx_count
        tx_count=$(echo "${block}" | jq -r '.block.data.txs | length' 2>/dev/null || echo "0")

        [[ "${VERBOSE}" == "true" ]] && log_info "transaction count: ${tx_count}"

        if [[ "${tx_count}" -gt 0 ]]; then
            record_result "chain_txs" "PASS" "block contains ${tx_count} transactions"
        else
            record_result "chain_txs" "WARN" "block has no transactions"
        fi
    else
        record_result "chain_block" "FAIL" "failed to get on-chain data"
    fi

    # check the validator set
    local validators
    validators=$(api_call "/validators" 2>/dev/null || echo "")

    if [[ -n "${validators}" ]]; then
        local validator_count
        validator_count=$(echo "${validators}" | jq -r '.validators | length' 2>/dev/null || echo "0")

        [[ "${VERBOSE}" == "true" ]] && log_info "validator count: ${validator_count}"

        if [[ "${validator_count}" -gt 0 ]]; then
            record_result "chain_validators" "PASS" "validator count: ${validator_count}"
        else
            record_result "chain_validators" "WARN" "no validators found"
        fi
    else
        record_result "chain_validators" "FAIL" "failed to get validator info"
    fi
}

# ========== check service logs ==========
check_logs() {
    log_section "check: service logs"

    if command -v journalctl &> /dev/null; then
        # check recent errors
        local errors
        errors=$(journalctl -u "${SERVICE_NAME}" --no-pager -n 100 --priority=err 2>/dev/null | wc -l)

        if [[ ${errors} -gt 0 ]]; then
            record_result "log_errors" "WARN" "found ${errors} error log entries"
            [[ "${VERBOSE}" == "true" ]] && journalctl -u "${SERVICE_NAME}" --no-pager -n 5 --priority=err
        else
            record_result "log_errors" "PASS" "no error logs"
        fi

        # check recent warnings
        local warnings
        warnings=$(journalctl -u "${SERVICE_NAME}" --no-pager -n 100 --priority=warning 2>/dev/null | wc -l)

        [[ "${VERBOSE}" == "true" ]] && log_info "warning log count: ${warnings}"

        if [[ ${warnings} -gt 10 ]]; then
            record_result "log_warnings" "WARN" "found ${warnings} warning log entries"
        else
            record_result "log_warnings" "PASS" "warning log count normal"
        fi
    else
        [[ "${VERBOSE}" == "true" ]] && log_info "skip log check (journalctl unavailable)"
    fi
}

# ========== print summary ==========
print_summary() {
    echo
    echo "========================================"
    echo "  validation result summary"
    echo "========================================"
    echo

    echo "passed: ${PASS_COUNT}"
    echo "failed: ${FAIL_COUNT}"
    echo

    # list failed items
    if [[ ${FAIL_COUNT} -gt 0 ]]; then
        echo -e "${RED}failed checks:${NC}"
        for key in "${!CHECK_RESULTS[@]}"; do
            if [[ "${CHECK_RESULTS[${key}]}" == "FAIL" ]]; then
                echo "  - ${key}"
            fi
        done
        echo
    fi

    if [[ ${FAIL_COUNT} -eq 0 ]]; then
        echo -e "${GREEN}all checks passed!${NC}"
        return 0
    else
        echo -e "${RED}some checks failed, review the errors above${NC}"
        return 1
    fi
}

# ========== main flow ==========
main() {
    echo
    echo -e "${GREEN}AIB 2.0 upgrade validation script${NC}"
    echo -e "${YELLOW}version: ${VERSION}${NC}"
    echo

    # parse arguments
    parse_args "$@"

    log_info "API port: ${API_PORT}"
    log_info "service name: ${SERVICE_NAME}"
    log_info "check item: ${CHECKS}"
    echo

    # run checks
    case "${CHECKS}" in
        all)
            check_version
            check_consensus
            check_api
            check_p2p
            check_sync
            check_chain
            check_logs
            ;;
        version)
            check_version
            ;;
        consensus)
            check_consensus
            ;;
        api)
            check_api
            ;;
        p2p)
            check_p2p
            ;;
        sync)
            check_sync
            ;;
        chain)
            check_chain
            ;;
        *)
            # supports comma-separated checks
            IFS=',' read -ra CHECK_ARRAY <<< "${CHECKS}"
            for check in "${CHECK_ARRAY[@]}"; do
                case "${check}" in
                    version) check_version ;;
                    consensus) check_consensus ;;
                    api) check_api ;;
                    p2p) check_p2p ;;
                    sync) check_sync ;;
                    chain) check_chain ;;
                    logs) check_logs ;;
                    *) log_warning "unknown check: ${check}" ;;
                esac
            done
            ;;
    esac

    # print summary
    print_summary

    exit $?
}

# ========== Entry Point ==========
main "$@"
