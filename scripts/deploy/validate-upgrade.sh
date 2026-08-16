#!/bin/bash
#
# AIB 2.0 升级验证脚本
# 用法: ./validate-upgrade.sh [选项]
# 选项:
#   -c, --check CHECKS       指定检查项: all|version|consensus|api|p2p|sync (默认: all)
#   -s, --service SERVICE   指定服务名 (默认: aib2-mainnet)
#   -p, --port PORT         API 端口 (默认: 51200)
#   -t, --timeout SECONDS   超时时间 (默认: 30)
#   -v, --verbose           详细输出
#   -h, --help              显示帮助信息
#
# 功能:
#   1. 验证版本升级
#   2. 验证共识状态
#   3. 验证 API 可用性
#   4. 验证 P2P 网络
#   5. 验证同步状态
#   6. 验证链上活动
#
# 作者: AIB Protocol Team
# 更新: 2026-03
#

set -e
set -u
set -o pipefail

# ========== 脚本元数据 ==========
SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
VERSION="2.0.0"

# ========== 颜色输出 ==========
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# ========== 日志函数 ==========
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

# ========== 配置变量 ==========
CHECKS="all"
SERVICE_NAME="aib2-mainnet"
API_PORT="51200"
P2P_PORT="26656"
TIMEOUT=30
VERBOSE=false
BINARY_PATH="${PROJECT_DIR}/bin/aib-node"

# 检查结果
declare -A CHECK_RESULTS
PASS_COUNT=0
FAIL_COUNT=0

# ========== 帮助信息 ==========
show_help() {
    cat << EOF
AIB 2.0 升级验证脚本 v${VERSION}

用法: ${SCRIPT_NAME} [选项]

选项:
  -c, --check CHECKS       指定检查项: all|version|consensus|api|p2p|sync (默认: all)
  -s, --service SERVICE   指定服务名 (默认: aib2-mainnet)
  -p, --port PORT         API 端口 (默认: 51200)
  -t, --timeout SECONDS   超时时间 (默认: 30)
  -v, --verbose           详细输出
  -h, --help              显示帮助信息

检查项:
  version     - 验证节点版本
  consensus   - 验证共识状态
  api         - 验证 API 可用性
  p2p         - 验证 P2P 网络连接
  sync        - 验证同步状态
  chain       - 验证链上活动

示例:
  # 验证所有项目
  ${SCRIPT_NAME}

  # 仅验证版本
  ${SCRIPT_NAME} -c version

  # 验证多个项目
  ${SCRIPT_NAME} -c version,consensus,api

  # 自定义端口
  ${SCRIPT_NAME} -p 51201

  # 详细输出
  ${SCRIPT_NAME} -v

注意: 需要节点服务正在运行才能进行验证
EOF
    exit 0
}

# ========== 参数解析 ==========
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
                log_error "未知参数: $1"
                show_help
                ;;
        esac
    done
}

# ========== API 调用 ==========
api_call() {
    local endpoint="$1"
    local url="http://localhost:${API_PORT}${endpoint}"

    if command -v curl &> /dev/null; then
        curl -sf --connect-timeout "${TIMEOUT}" "${url}" 2>/dev/null
    else
        log_error "curl 未安装"
        return 1
    fi
}

# ========== 记录结果 ==========
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

# ========== 检查 1: 版本验证 ==========
check_version() {
    log_section "检查: 版本验证"

    # 检查二进制版本
    local binary_version
    if [[ -f "${BINARY_PATH}" ]]; then
        binary_version=$("${BINARY_PATH}" --version 2>&1 | head -1)
    else
        binary_version="binary_not_found"
    fi

    [[ "${VERBOSE}" == "true" ]] && log_info "二进制版本: ${binary_version}"

    # 获取 API 版本
    local api_version
    api_version=$(api_call "/version" 2>/dev/null || echo "api_error")

    [[ "${VERBOSE}" == "true" ]] && log_info "API 版本: ${api_version}"

    # 检查一致性
    if [[ "${binary_version}" != *"not_found"* ]] && [[ "${api_version}" != "api_error" ]]; then
        # 简单的版本检查
        if [[ -n "${api_version}" ]]; then
            record_result "version" "PASS" "版本正常 (${binary_version})"
        else
            record_result "version" "WARN" "API 版本获取失败"
        fi
    else
        record_result "version" "FAIL" "版本检查失败"
    fi
}

# ========== 检查 2: 共识状态 ==========
check_consensus() {
    log_section "检查: 共识状态"

    # 检查服务状态
    local service_status
    if command -v systemctl &> /dev/null; then
        service_status=$(systemctl is-active "${SERVICE_NAME}" 2>/dev/null || echo "inactive")
    else
        # 尝试检测进程
        if pgrep -f "aib-node" > /dev/null; then
            service_status="running"
        else
            service_status="not_running"
        fi
    fi

    [[ "${VERBOSE}" == "true" ]] && log_info "服务状态: ${service_status}"

    if [[ "${service_status}" == "active" || "${service_status}" == "running" ]]; then
        record_result "consensus" "PASS" "节点服务运行中"
    else
        record_result "consensus" "FAIL" "节点服务未运行: ${service_status}"
        return 1
    fi

    # 检查共识参数
    local consensus_params
    consensus_params=$(api_call "/consensus/params" 2>/dev/null || echo "")

    [[ "${VERBOSE}" == "true" ]] && log_info "共识参数: ${consensus_params}"

    if [[ -n "${consensus_params}" ]]; then
        record_result "consensus_params" "PASS" "共识参数可查询"
    else
        record_result "consensus_params" "WARN" "共识参数获取失败"
    fi
}

# ========== 检查 3: API 可用性 ==========
check_api() {
    log_section "检查: API 可用性"

    # 检查健康端点
    local health
    health=$(api_call "/health" 2>/dev/null || echo "error")

    [[ "${VERBOSE}" == "true" ]] && log_info "健康状态: ${health}"

    if [[ "${health}" != "error" ]]; then
        record_result "api_health" "PASS" "健康检查通过"
    else
        record_result "api_health" "FAIL" "健康检查失败"
    fi

    # 检查关键端点
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
            [[ "${VERBOSE}" == "true" ]] && log_info "端点 ${endpoint}: OK"
        else
            [[ "${VERBOSE}" == "true" ]] && log_info "端点 ${endpoint}: FAIL"
        fi
    done

    if [[ ${endpoint_success} -eq ${endpoint_count} ]]; then
        record_result "api_endpoints" "PASS" "所有关键端点可用"
    elif [[ ${endpoint_success} -gt 0 ]]; then
        record_result "api_endpoints" "WARN" "${endpoint_success}/${endpoint_count} 端点可用"
    else
        record_result "api_endpoints" "FAIL" "所有端点不可用"
    fi
}

# ========== 检查 4: P2P 网络 ==========
check_p2p() {
    log_section "检查: P2P 网络"

    # 获取网络信息
    local net_info
    net_info=$(api_call "/net_info" 2>/dev/null || echo "")

    [[ "${VERBOSE}" == "true" ]] && log_info "网络信息: ${net_info}"

    if [[ -n "${net_info}" ]]; then
        # 尝试提取连接数
        local n_peers
        n_peers=$(echo "${net_info}" | jq -r '.n_peers' 2>/dev/null || echo "0")

        [[ "${VERBOSE}" == "true" ]] && log_info "对等节点数: ${n_peers}"

        if [[ "${n_peers}" -gt 0 ]]; then
            record_result "p2p_peers" "PASS" "已连接 ${n_peers} 个对等节点"
        elif [[ "${n_peers}" == "0" ]]; then
            record_result "p2p_peers" "WARN" "没有连接的对等节点"
        else
            record_result "p2p_peers" "FAIL" "无法获取对等节点信息"
        fi
    else
        record_result "p2p_peers" "FAIL" "无法连接到 P2P 网络"
    fi

    # 检查 P2P 端口
    if command -v nc &> /dev/null || command -v timeout &> /dev/null; then
        if nc -z localhost "${P2P_PORT}" 2>/dev/null || timeout 1 bash -c "echo > /dev/tcp/localhost/${P2P_PORT}" 2>/dev/null; then
            record_result "p2p_port" "PASS" "P2P 端口 ${P2P_PORT} 可访问"
        else
            record_result "p2p_port" "FAIL" "P2P 端口 ${P2P_PORT} 不可访问"
        fi
    else
        [[ "${VERBOSE}" == "true" ]] && log_info "跳过端口检查 (nc/timeout 不可用)"
    fi
}

# ========== 检查 5: 同步状态 ==========
check_sync() {
    log_section "检查: 同步状态"

    # 获取状态
    local status
    status=$(api_call "/status" 2>/dev/null || echo "")

    if [[ -n "${status}" ]]; then
        # 检查同步状态
        local syncing
        syncing=$(echo "${status}" | jq -r '.syncing' 2>/dev/null || echo "unknown")

        [[ "${VERBOSE}" == "true" ]] && log_info "同步状态: ${syncing}"

        if [[ "${syncing}" == "false" ]]; then
            record_result "sync_status" "PASS" "节点已同步"
        elif [[ "${syncing}" == "true" ]]; then
            record_result "sync_status" "WARN" "节点正在同步"
        else
            record_result "sync_status" "WARN" "无法确定同步状态"
        fi

        # 获取区块高度
        local block_height
        block_height=$(echo "${status}" | jq -r '.latest_block_height' 2>/dev/null || echo "0")

        [[ "${VERBOSE}" == "true" ]] && log_info "区块高度: ${block_height}"

        if [[ "${block_height}" -gt 0 ]]; then
            record_result "sync_height" "PASS" "当前高度: ${block_height}"
        else
            record_result "sync_height" "FAIL" "无法获取区块高度"
        fi
    else
        record_result "sync_status" "FAIL" "无法获取同步状态"
    fi
}

# ========== 检查 6: 链上活动 ==========
check_chain() {
    log_section "检查: 链上活动"

    # 获取最新区块
    local block
    block=$(api_call "/blocks/latest" 2>/dev/null || echo "")

    if [[ -n "${block}" ]]; then
        local block_height
        block_height=$(echo "${block}" | jq -r '.block.header.height' 2>/dev/null || echo "0")

        local block_time
        block_time=$(echo "${block}" | jq -r '.block.header.timestamp' 2>/dev/null || echo "")

        [[ "${VERBOSE}" == "true" ]] && log_info "最新区块: ${block_height} (${block_time})"

        if [[ "${block_height}" -gt 0 ]]; then
            record_result "chain_block" "PASS" "最新区块: ${block_height}"
        else
            record_result "chain_block" "FAIL" "无法获取区块信息"
        fi

        # 检查交易数量
        local tx_count
        tx_count=$(echo "${block}" | jq -r '.block.data.txs | length' 2>/dev/null || echo "0")

        [[ "${VERBOSE}" == "true" ]] && log_info "交易数: ${tx_count}"

        if [[ "${tx_count}" -gt 0 ]]; then
            record_result "chain_txs" "PASS" "区块包含 ${tx_count} 笔交易"
        else
            record_result "chain_txs" "WARN" "区块无交易"
        fi
    else
        record_result "chain_block" "FAIL" "无法获取链上数据"
    fi

    # 检查验证者集
    local validators
    validators=$(api_call "/validators" 2>/dev/null || echo "")

    if [[ -n "${validators}" ]]; then
        local validator_count
        validator_count=$(echo "${validators}" | jq -r '.validators | length' 2>/dev/null || echo "0")

        [[ "${VERBOSE}" == "true" ]] && log_info "验证者数量: ${validator_count}"

        if [[ "${validator_count}" -gt 0 ]]; then
            record_result "chain_validators" "PASS" "验证者数量: ${validator_count}"
        else
            record_result "chain_validators" "WARN" "未找到验证者"
        fi
    else
        record_result "chain_validators" "FAIL" "无法获取验证者信息"
    fi
}

# ========== 检查服务日志 ==========
check_logs() {
    log_section "检查: 服务日志"

    if command -v journalctl &> /dev/null; then
        # 检查最近错误
        local errors
        errors=$(journalctl -u "${SERVICE_NAME}" --no-pager -n 100 --priority=err 2>/dev/null | wc -l)

        if [[ ${errors} -gt 0 ]]; then
            record_result "log_errors" "WARN" "发现 ${errors} 条错误日志"
            [[ "${VERBOSE}" == "true" ]] && journalctl -u "${SERVICE_NAME}" --no-pager -n 5 --priority=err
        else
            record_result "log_errors" "PASS" "无错误日志"
        fi

        # 检查最近警告
        local warnings
        warnings=$(journalctl -u "${SERVICE_NAME}" --no-pager -n 100 --priority=warning 2>/dev/null | wc -l)

        [[ "${VERBOSE}" == "true" ]] && log_info "警告日志数量: ${warnings}"

        if [[ ${warnings} -gt 10 ]]; then
            record_result "log_warnings" "WARN" "发现 ${warnings} 条警告日志"
        else
            record_result "log_warnings" "PASS" "警告日志数量正常"
        fi
    else
        [[ "${VERBOSE}" == "true" ]] && log_info "跳过日志检查 (journalctl 不可用)"
    fi
}

# ========== 打印摘要 ==========
print_summary() {
    echo
    echo "========================================"
    echo "  验证结果摘要"
    echo "========================================"
    echo

    echo "通过: ${PASS_COUNT}"
    echo "失败: ${FAIL_COUNT}"
    echo

    # 列出失败项
    if [[ ${FAIL_COUNT} -gt 0 ]]; then
        echo -e "${RED}失败检查项:${NC}"
        for key in "${!CHECK_RESULTS[@]}"; do
            if [[ "${CHECK_RESULTS[${key}]}" == "FAIL" ]]; then
                echo "  - ${key}"
            fi
        done
        echo
    fi

    if [[ ${FAIL_COUNT} -eq 0 ]]; then
        echo -e "${GREEN}所有检查通过!${NC}"
        return 0
    else
        echo -e "${RED}部分检查失败，请检查上述错误${NC}"
        return 1
    fi
}

# ========== 主流程 ==========
main() {
    echo
    echo -e "${GREEN}AIB 2.0 升级验证脚本${NC}"
    echo -e "${YELLOW}版本: ${VERSION}${NC}"
    echo

    # 解析参数
    parse_args "$@"

    log_info "API 端口: ${API_PORT}"
    log_info "服务名: ${SERVICE_NAME}"
    log_info "检查项: ${CHECKS}"
    echo

    # 执行检查
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
            # 支持逗号分隔的多个检查
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
                    *) log_warning "未知检查项: ${check}" ;;
                esac
            done
            ;;
    esac

    # 打印摘要
    print_summary

    exit $?
}

# ========== 入口 ==========
main "$@"
