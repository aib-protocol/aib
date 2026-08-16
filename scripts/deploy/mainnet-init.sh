#!/bin/bash
#
# AIB 2.0 主网初始化脚本
# 用法: ./mainnet-init.sh [选项]
# 选项:
#   -n, --network NETWORK    指定网络: mainnet|testnet (默认: mainnet)
#   -i, --node-id NODE_ID    指定节点 ID
#   -p, --port PORT          指定 API 端口 (默认: 51200)
#   -m, --moniker NODE_NAME  指定节点名称
#   -s, --stake AMOUNT       质押金额
#   -d, --data-dir DIR       数据目录
#   -f, --force              强制初始化（覆盖现有数据）
#   -h, --help               显示帮助信息
#
# 功能:
#   1. 创建必要的目录结构
#   2. 生成节点密钥
#   3. 配置 genesis 文件
#   4. 设置初始验证者
#   5. 配置 P2P 网络
#   6. 启动节点服务
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

# ========== 配置变量 ==========
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

# Genesis 配置 (主网)
MAINNET_CHAIN_ID="aib-mainnet-1"
MAINNET_GENESIS_TIME="2026-03-14T00:00:00Z"
MAINNET_TOTAL_SUPPLY="3141592653"
MAINNET_BLOCK_REWARD=50
MAINNET_BLOCK_TIME=30
MAINNET_MAX_VALIDATORS=100
MAINNET_MIN_STAKE="1000000"
MAINNET_UNBONDING_TIME="604800"

# Testnet 配置
TESTNET_CHAIN_ID="aib-testnet-1"
TESTNET_GENESIS_TIME="2026-01-01T00:00:00Z"
TESTNET_TOTAL_SUPPLY="1000000000"
TESTNET_BLOCK_REWARD=10
TESTNET_BLOCK_TIME=15
TESTNET_MAX_VALIDATORS=21
TESTNET_MIN_STAKE="10000"
TESTNET_UNBONDING_TIME="3600"

# ========== 帮助信息 ==========
show_help() {
    cat << EOF
AIB 2.0 主网初始化脚本 v${VERSION}

用法: ${SCRIPT_NAME} [选项]

选项:
  -n, --network NETWORK    指定网络: mainnet|testnet (默认: mainnet)
  -i, --node-id NODE_ID    指定节点 ID (可选，自动生成)
  -p, --port PORT          指定 API 端口 (默认: 51200)
  -m, --moniker NAME       指定节点名称
  -s, --stake AMOUNT       初始质押金额 (默认: 1000000)
  -d, --data-dir DIR       数据目录
  -c, --config-dir DIR     配置目录
  -f, --force              强制初始化（覆盖现有数据）
  -h, --help               显示帮助信息

示例:
  # 初始化主网节点
  ${SCRIPT_NAME} -m "my-validator" -s 1000000

  # 初始化测试网节点
  ${SCRIPT_NAME} -n testnet -m "test-validator"

  # 指定自定义端口
  ${SCRIPT_NAME} -p 51201 -m "secondary-node"

初始化步骤:
  1. 创建目录结构
  2. 生成节点密钥
  3. 配置 genesis 文件
  4. 创建初始验证者
  5. 配置 P2P 网络
  6. 设置 systemd 服务
  7. 启动节点

注意: 首次初始化需要足够的质押代币
更多信息请访问: https://docs.aib.network/mainnet
EOF
    exit 0
}

# ========== 参数解析 ==========
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
                log_error "未知参数: $1"
                show_help
                ;;
        esac
    done
}

# ========== 环境检查 ==========
check_environment() {
    log_info "检查运行环境..."

    # 检查必要命令
    local required_commands=("openssl" "jq" "curl")
    for cmd in "${required_commands[@]}"; do
        if ! command -v "$cmd" &> /dev/null; then
            log_error "缺少必要命令: $cmd"
            return 1
        fi
    done

    # 检查二进制文件
    if [[ ! -f "${BINARY_DIR}/aib-node" ]]; then
        log_error "节点二进制不存在: ${BINARY_DIR}/aib-node"
        log_info "请先构建或下载节点二进制"
        return 1
    fi

    # 检查权限
    if [[ ! -w "${PROJECT_DIR}" ]]; then
        log_error "没有项目目录写权限: ${PROJECT_DIR}"
        return 1
    fi

    log_success "环境检查通过"
    return 0
}

# ========== 获取网络配置 ==========
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

# ========== 创建目录结构 ==========
create_directories() {
    log_step "创建目录结构..."

    local dirs=(
        "${DATA_DIR}"
        "${DATA_DIR}/wallet"
        "${DATA_DIR}/keystore"
        "${CONFIG_DIR}"
        "${PROJECT_DIR}/logs"
    )

    for dir in "${dirs[@]}"; do
        mkdir -p "${dir}"
        log_info "创建目录: ${dir}"
    done

    log_success "目录创建完成"
}

# ========== 生成节点密钥 ==========
generate_keys() {
    log_step "生成节点密钥..."

    local priv_key_file="${DATA_DIR}/priv_validator_key.json"
    local node_key_file="${DATA_DIR}/node_key.json"

    if [[ -f "${priv_key_file}" && "${FORCE_INIT}" != "true" ]]; then
        log_warning "节点密钥已存在，跳过生成"
    else
        # 生成验证者私钥
        if [[ ! -f "${priv_key_file}" ]]; then
            # 使用 openssl 生成随机私钥
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
            log_info "生成验证者密钥: ${priv_key_file}"
        fi

        # 生成节点密钥
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
            log_info "生成节点密钥: ${node_key_file}"
        fi
    fi

    # 生成节点 ID
    if [[ -z "${NODE_ID}" ]]; then
        NODE_ID=$(cat "${node_key_file}" 2>/dev/null | jq -r '.priv_key.value' | sha256sum | cut -d' ' -f1 | head -c 40)
    fi

    log_success "节点 ID: ${NODE_ID}"
    return 0
}

# ========== 生成 genesis 文件 ==========
generate_genesis() {
    log_step "生成 Genesis 文件..."

    local genesis_file="${CONFIG_DIR}/genesis.json"

    # 检查是否已有 genesis
    if [[ -f "${genesis_file}" && "${FORCE_INIT}" != "true" ]]; then
        log_warning "Genesis 文件已存在: ${genesis_file}"
        read -p "是否覆盖? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "使用现有 genesis 文件"
            return 0
        fi
    fi

    # 从模板复制或创建新的 genesis
    if [[ -f "${GENESIS_DIR}/genesis.json" ]]; then
        log_info "使用 genesis 模板..."
        cp "${GENESIS_DIR}/genesis.json" "${genesis_file}"
    else
        log_info "创建新的 genesis 文件..."
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

    # 设置 genesis 哈希
    local genesis_hash=$(sha256sum "${genesis_file}" | awk '{print $1}')
    log_info "Genesis hash: ${genesis_hash}"

    log_success "Genesis 文件生成完成"
    return 0
}

# ========== 创建配置文件 ==========
create_config() {
    log_step "创建配置文件..."

    local config_file="${CONFIG_DIR}/config.toml"

    cat > "${config_file}" << EOF
# AIB 2.0 Node Configuration
# Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)

# ========== 节点配置 ==========
[node]
  # 节点名称
  moniker = "${MONIKER}"
  # 节点类型: validator, full, light
  node_type = "validator"
  # 数据目录
  data_dir = "${DATA_DIR}"
  # 日志级别
  log_level = "info"

# ========== API 配置 ==========
[api]
  # API 监听地址
  listen_address = "0.0.0.0:${API_PORT}"
  # 启用 API
  enabled = true
  # 允许跨域
  enable_cors = true
  # API 限流
  rate_limit = 100

# ========== P2P 配置 ==========
[p2p]
  # P2P 监听地址
  listen_address = "0.0.0.0:${P2P_PORT}"
  # 外部地址
  external_address = ""
  # 种子节点
  seeds = []
  # 持久节点
  persistent_peers = []
  # 最大连接数
  max_connections = 100

# ========== 共识配置 ==========
[consensus]
  # 区块时间（秒）
  block_time = ${BLOCK_TIME}
  # 验证者数量
  validators = []
  # 最小质押
  min_stake = "${MIN_STAKE}"
  # 解绑时间（秒）
  unbonding_time = ${UNBONDING_TIME}

# ========== 创世配置 ==========
[genesis]
  # 链 ID
  chain_id = "${CHAIN_ID}"
  # Genesis 文件路径
  genesis_file = "${CONFIG_DIR}/genesis.json"

# ========== 监控配置 ==========
[telemetry]
  # 启用遥测
  enabled = true
  # Prometheus 端口
  prometheus_port = 26660
EOF

    log_success "配置文件创建完成: ${config_file}"
    return 0
}

# ========== 创建 systemd 服务 ==========
create_systemd_service() {
    log_step "创建 systemd 服务..."

    local service_file="/etc/systemd/system/${SERVICE_NAME}.service"

    if [[ -f "${service_file}" ]]; then
        log_warning "服务文件已存在: ${service_file}"
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

# 环境变量
Environment="HOME=${HOME}"
Environment="PORT=${API_PORT}"

# 日志配置
StandardOutput=append:${PROJECT_DIR}/logs/mainnet.log
StandardError=append:${PROJECT_DIR}/logs/mainnet.error.log

[Install]
WantedBy=multi-user.target
EOF
        log_success "服务文件创建完成: ${service_file}"
    fi

    # 重新加载 systemd
    if command -v systemctl &> /dev/null; then
        systemctl daemon-reload
        log_info "systemd 配置已重载"
    fi

    return 0
}

# ========== 启动节点 ==========
start_node() {
    log_step "启动节点服务..."

    if command -v systemctl &> /dev/null; then
        # 启用服务
        systemctl enable "${SERVICE_NAME}"

        # 启动服务
        systemctl start "${SERVICE_NAME}"

        # 等待启动
        sleep 3

        # 检查状态
        if systemctl is-active --quiet "${SERVICE_NAME}"; then
            log_success "节点服务已启动"
        else
            log_error "节点服务启动失败"
            journalctl -u "${SERVICE_NAME}" --no-pager -n 50
            return 1
        fi
    else
        log_warning "systemd 未安装，使用前台模式启动"
        log_info "运行: ${BINARY_DIR}/aib-node --config ${CONFIG_DIR}/config.toml"
        # 不在这里启动，交给用户
    fi

    return 0
}

# ========== 验证初始化 ==========
verify_init() {
    log_step "验证初始化结果..."

    sleep 5

    # 检查服务状态
    if command -v systemctl &> /dev/null; then
        if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
            log_error "服务未运行"
            return 1
        fi
    fi

    # 检查 API
    local api_url="http://localhost:${API_PORT}/health"
    if command -v curl &> /dev/null; then
        if curl -sf "${api_url}" > /dev/null 2>&1; then
            log_success "API 健康检查通过"
        else
            log_warning "API 健康检查失败"
        fi
    fi

    # 获取节点信息
    local node_info_url="http://localhost:${API_PORT}/node_info"
    if command -v curl &> /dev/null; then
        local node_info=$(curl -s "${node_info_url}" 2>/dev/null)
        if [[ -n "${node_info}" ]]; then
            log_info "节点信息: ${node_info}"
        fi
    fi

    log_success "初始化验证完成"
    return 0
}

# ========== 打印摘要 ==========
print_summary() {
    echo
    echo "========================================"
    echo "  AIB 2.0 主网初始化完成"
    echo "========================================"
    echo
    echo "网络: ${NETWORK}"
    echo "链 ID: ${CHAIN_ID}"
    echo "节点 ID: ${NODE_ID}"
    echo "节点名称: ${MONIKER}"
    echo "API 端口: ${API_PORT}"
    echo "P2P 端口: ${P2P_PORT}"
    echo
    echo "数据目录: ${DATA_DIR}"
    echo "配置目录: ${CONFIG_DIR}"
    echo
    echo "服务名称: ${SERVICE_NAME}"
    echo
    echo "常用命令:"
    echo "  查看状态: systemctl status ${SERVICE_NAME}"
    echo "  查看日志: journalctl -u ${SERVICE_NAME} -f"
    echo "  重启服务: systemctl restart ${SERVICE_NAME}"
    echo "  停止服务: systemctl stop ${SERVICE_NAME}"
    echo
    echo "API 地址: http://localhost:${API_PORT}"
    echo "链 ID: ${CHAIN_ID}"
    echo
    echo "========================================"
}

# ========== 主流程 ==========
main() {
    echo
    echo -e "${GREEN}AIB 2.0 主网初始化脚本${NC}"
    echo -e "${YELLOW}版本: ${VERSION}${NC}"
    echo

    # 解析参数
    parse_args "$@"

    # 检查必需参数
    if [[ -z "${MONIKER}" ]]; then
        log_error "必须指定节点名称 (-m, --moniker)"
        show_help
    fi

    # 环境检查
    check_environment || exit 1

    # 获取网络配置
    get_network_config

    log_info "网络: ${NETWORK}"
    log_info "链 ID: ${CHAIN_ID}"

    # 确认初始化
    if [[ "${FORCE_INIT}" != "true" ]]; then
        echo
        echo -e "${YELLOW}即将初始化 ${NETWORK} 节点${NC}"
        echo -e "节点名称: ${MONIKER}"
        echo -e "API 端口: ${API_PORT}"
        echo
        read -p "确认继续? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "初始化已取消"
            exit 0
        fi
    fi

    # 执行初始化步骤
    create_directories || exit 1
    generate_keys || exit 1
    generate_genesis || exit 1
    create_config || exit 1
    create_systemd_service || exit 1

    # 询问是否启动
    if [[ "${FORCE_INIT}" == "true" || -t 1 ]]; then
        read -p "是否启动节点服务? (Y/n) " -n 1 -r
        echo
        if [[ -z $REPLY || $REPLY =~ ^[Yy]$ ]]; then
            start_node || exit 1
            verify_init || exit 1
        fi
    fi

    # 打印摘要
    print_summary

    log_success "初始化完成!"
    exit 0
}

# ========== 入口 ==========
main "$@"
