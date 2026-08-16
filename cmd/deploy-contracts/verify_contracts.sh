#!/bin/bash

# AIB DeFi 合约验证脚本

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
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

# 配置文件路径
CONFIG_FILE="${1:-./cmd/deploy-contracts/config.yaml}"
CONTRACTS_DIR="./cmd/deploy-contracts/contracts"

# 检查参数
if [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    echo "用法: $0 [配置文件路径]"
    echo "默认配置文件路径: ./cmd/deploy-contracts/config.yaml"
    exit 0
fi

log_info "开始验证 DeFi 合约..."

# 检查配置文件
if [ ! -f "$CONFIG_FILE" ]; then
    log_error "配置文件不存在: $CONFIG_FILE"
    exit 1
fi

# 提取RPC端点
RPC_ENDPOINT=$(grep "rpc_endpoint:" "$CONFIG_FILE" | awk '{print $2}')
if [ -z "$RPC_ENDPOINT" ]; then
    RPC_ENDPOINT="http://localhost:8545"
fi

log_info "RPC 端点: $RPC_ENDPOINT"

# 检查网络连接
log_info "检查网络连接..."
if ! curl -s -X POST "$RPC_ENDPOINT" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
    > /dev/null 2>&1; then
    log_error "无法连接到 RPC 节点: $RPC_ENDPOINT"
    log_info "请确保节点正在运行"
    exit 1
fi

# 获取链ID
CHAIN_ID=$(curl -s -X POST "$RPC_ENDPOINT" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' \
    | jq -r '.result')

log_info "链ID: $CHAIN_ID"

# 验证函数
verify_contract() {
    local contract_name=$1
    local contract_address=$2

    log_info "验证 $contract_name 合约..."

    # 检查合约地址格式
    if ! [[ "$contract_address" =~ ^0x[a-fA-F0-9]{40}$ ]]; then
        log_error "$contract_name: 无效的地址格式"
        return 1
    fi

    # 检查合约代码是否存在
    local code=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_getCode\",\"params\":[\"$contract_address\",\"latest\"],\"id\":1}" \
        | jq -r '.result')

    if [ "$code" = "0x" ] || [ -z "$code" ]; then
        log_error "$contract_name: 合约代码为空，部署可能失败"
        return 1
    fi

    local code_length=$(( ${#code} - 2 ))  # 减去 "0x" 前缀
    log_info "$contract_name: 代码长度 = $code_length 字节"

    if [ "$code_length" -lt 100 ]; then
        log_warning "$contract_name: 代码长度异常短"
    fi

    # 检查合约的nonce
    local nonce=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_getTransactionCount\",\"params\":[\"$contract_address\",\"latest\"],\"id\":1}" \
        | jq -r '.result')

    log_info "$contract_name: Nonce = $nonce"

    log_success "$contract_name 验证通过"
    return 0
}

# 测试WETH功能
test_weth() {
    local weth_address=$1

    log_info "测试 WETH 功能..."

    # 存款功能
    local balance=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$weth_address\",\"data\":\"0x70a082310000000000000000000000000000000000000000000000000000000000000000\"},\"latest\"],\"id\":1}" \
        | jq -r '.result')

    if [ "$balance" != "0x" ]; then
        log_info "WETH 总供应量: $balance"
    fi

    log_success "WETH 功能测试完成"
    return 0
}

# 测试UniswapV2Factory功能
test_factory() {
    local factory_address=$1

    log_info "测试 UniswapV2Factory 功能..."

    # 获取交易对数量
    local pair_count=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$factory_address\",\"data\":\"0x1e3dd18d\"},\"latest\"],\"id\":1}" \
        | jq -r '.result')

    if [ "$pair_count" != "0x" ]; then
        log_info "Factory 交易对数量: $pair_count"
    fi

    log_success "UniswapV2Factory 功能测试完成"
    return 0
}

# 测试UniswapV2Router功能
test_router() {
    local router_address=$1
    local weth_address=$2
    local factory_address=$3

    log_info "测试 UniswapV2Router 功能..."

    # 检查WETH地址
    local router_weth=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$router_address\",\"data\":\"0x4e2f6d76\"},\"latest\"],\"id\":1}" \
        | jq -r '.result')

    if [ "$router_weth" != "0x" ]; then
        # 提取WETH地址 (取最后40个字符)
        local extracted_weth="0x${router_weth: -40}"
        if [ "${extracted_weth,,}" = "${weth_address,,}" ]; then
            log_info "Router WETH 配置正确"
        else
            log_warning "Router WETH 地址不匹配"
        fi
    fi

    # 检查Factory地址
    local router_factory=$(curl -s -X POST "$RPC_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$router_address\",\"data\":\"0xac4c2f2f\"},\"latest\"],\"id\":1}" \
        | jq -r '.result')

    if [ "$router_factory" != "0x" ]; then
        local extracted_factory="0x${router_factory: -40}"
        if [ "${extracted_factory,,}" = "${factory_address,,}" ]; then
            log_info "Router Factory 配置正确"
        else
            log_warning "Router Factory 地址不匹配"
        fi
    fi

    log_success "UniswapV2Router 功能测试完成"
    return 0
}

# 性能测试
performance_test() {
    log_info "执行性能测试..."

    # 测试区块时间
    local start_time=$(date +%s%N)

    for i in {1..10}; do
        curl -s -X POST "$RPC_ENDPOINT" \
            -H "Content-Type: application/json" \
            -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
            > /dev/null
    done

    local end_time=$(date +%s%N)
    local duration=$(( (end_time - start_time) / 1000000 ))

    log_info "10次请求耗时: ${duration}ms"
    log_info "平均响应时间: $(( duration / 10 ))ms"

    log_success "性能测试完成"
}

# 主验证流程
echo "============================================"
echo "  AIB DeFi 合约验证工具"
echo "============================================"
echo ""

# 检查部署记录
DEPLOY_RECORDS_DIR="./deployments/records"

if [ -d "$DEPLOY_RECORDS_DIR" ]; then
    log_info "找到部署记录目录: $DEPLOY_RECORDS_DIR"

    # 查找最新的部署记录
    LATEST_RECORD=$(ls -t "$DEPLOY_RECORDS_DIR"/deployment_*.json 2>/dev/null | head -1)

    if [ -n "$LATEST_RECORD" ]; then
        log_info "使用部署记录: $LATEST_RECORD"

        # 解析合约地址
        WETH_ADDRESS=$(cat "$LATEST_RECORD" | jq -r '.contracts[] | select(.name=="WETH") | .address')
        FACTORY_ADDRESS=$(cat "$LATEST_RECORD" | jq -r '.contracts[] | select(.name=="UniswapV2Factory") | .address')
        ROUTER_ADDRESS=$(cat "$LATEST_RECORD" | jq -r '.contracts[] | select(.name=="UniswapV2Router") | .address')

        # 验证每个合约
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
        log_warning "未找到部署记录，请先运行部署脚本"
    fi
else
    log_warning "未找到部署记录目录: $DEPLOY_RECORDS_DIR"
    log_info "请先运行部署脚本: ./deploy_contracts.sh"
fi

echo ""
echo "============================================"
echo "  验证完成"
echo "============================================"
