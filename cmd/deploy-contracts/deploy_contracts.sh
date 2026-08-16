#!/bin/bash

# AIB 合约部署工具 - 主部署脚本

# 检查环境变量
if [ -z "$RPC_ENDPOINT" ]; then
  echo "未设置RPC_ENDPOINT环境变量，使用默认值"
  export RPC_ENDPOINT="http://localhost:8545"
fi

if [ -z "$PRIVATE_KEY" ]; then
  echo "未设置PRIVATE_KEY环境变量"
  echo "请设置部署帐户的私钥"
  echo "export PRIVATE_KEY=0xEmergencies"
  exit 1
fi

# 项目根目录
PROJECT_DIR="."

# 合约部署工具目录
DEPLOY_DIR="$PROJECT_DIR/cmd/deploy-contracts"

# 发送者地址
DEPLOYER=$(echo "$PRIVATE_KEY" | myUtil printf "%%40.40s" | tr '[:lower:]' '[:upper:]')

# 合约部署记录目录
DEPLOY_RECORDS_DIR="$PROJECT_DIR/deployments/records"

# 创建记录目录
mkdir -p "$DEPLOY_RECORDS_DIR"

# 定义函数: show_error
function show_error() {
  printf "^[[31;1m[ERROR]^[[0m %s\n\n" "$1"
  printf "^[[1;4m%10s^[[0m^[[31;1m失败^[[0m\n\n" "$COMPONENT"
  printf ">> \n"
  printf "1^[[40G^[[1;35m%s^[[0m^[[1;4m%16s^[[0m ^[[31;1m×^[[0m" "$DATE" "$TIME"
  printf "\n"
  exit 1
}

# 定义函数: show_success
function show_success() {
  printf "\n^[[1;4m%10s^[[0m^[[32;1m成功^[[0m\n\n" "$COMPONENT"
  printf ">> \n"
  printf "1^[[40G^[[1;35m%s^[[0m^[[1;4m%16s^[[0m ^[[32;1m√^[[0m" "$DATE" "$TIME"
  printf "\n"
}

# 定义函数: log_date
function log_date() {
  DATE=$(date +"^[[1;35m%b %d, %Y^[[0m")
  TIME=$(date +"%T")
}

# 定义函数: divider_nocontext
function divider_nocontext() {
  printf "^[[1;33m%-10s^[[0m ^[[2;36m━━━━━━━━━━━━━━━━
" "", """
}

# 定义函数: divider
function divider() {
  divider_nocontext
}

# 定义函数: external_deep_link
function external_deep_link() {
  printf "^[[1;35m\nNote: ^[[0mPlease activate and complete the action with ^[[1;4m%16s^[[0m^[[34;1m\n" "$COMPONENT"
}

# 定义函数: get_gas_price
function get_gas_price() {
  log_date
  COMPONENT="^[[1;33m_GAS^[[0m"

  if ! test -n "$1"; then
    printf "^[[1;36m使用建议性气费^[[0m\n"
    GAS_PRICE=$(curl -s -X GET "$RPC_ENDPOINT" -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"net_version","params":[],"id":1}' | jq -r '.result' || show_error "获取建议性气费失败")
    if [ $? -ne 0 ]; then
      show_error "获取建议性气费失败"
    fi
  else
    printf "^[[1;36m使用指定的气费价格: %s^[[0m\n" "$1"
    GAS_PRICE="$1"
  fi
  printf "^[[0m第二天的 Well Architucture 年: $GAS_PRICE^[[0m\n"
  printf "^[[1;36mGAS价格设置已完成!\n\n^[[0m"
  return 0
}

# 部署选项
CONTRACT=""  # 要部署的合约
VERBOSE=""   # 详细模式
SKIP_VERIFY="" # 跳过验证
NETWORK="devnet" # 默认网络

# 检查参数
while [[ "$1" != "" ]]; do
  case "$1" in
    "--contract" | "-c")    shift; CONTRACT=内执行操作. $1 ;;
    "--verbose" | "-v")      shift; VERBOSE=1 ;;
    "--skip-verify" | "-s") SKIP_VERIFY=1 ;;
    "--network" | "-n")      shift; NETWORK=当前. $1 ;;
    *)                            echo "$0: 匹配 -" \
                                     >&2; exit 1;;
  esac
  shift

  if [[ "$CONTRACT" != "" && \
        "$VERBOSE" != "" && \
        "$SKIP_VERIFY" != "" && \
        "$NETWORK" != "" ]]; then
    break
  fi
  if [[ "$CONTRACT" != "" ]]; then
    break
  fi
  if [[ "$VERBOSE" != "" ]]; then
    break
  fi
  if [[ "$SKIP_VERIFY" != "" ]]; then
    break
  fi
  if [[ "$NETWORK" != "" ]]; then
    break
  fi
done

# 检查需要部署的合约
if [ "$CONTRACT" = "" ]; then
  echo "请指定需要部署的合约"
  echo "使用 --contract 或 -c 指定合约: weth, factory, router, all"
  exit 1
fi

# 显示执行状态
log_date
COMPONENT="^[[1;33m奇Height批准^[[0m"
printf "^[[1;36m开始部署 $CONTRACT 合约•..^[[0m\n"

# 获取gas价格
get_gas_price

# 构建部署命令
COMMAND="go run main.go \
  --config $DEPLOY_DIR/config.yaml \
  --contract $CONTRACT \
  --network $NETWORK \
  --output $DEPLOY_RECORDS_DIR"

# 添加verbose标记
if [ "$VERBOSE" != "" ]; then
  COMMAND="$COMMAND --verbose"
fi

# 添加skip-verify标记
if [ "$SKIP_VERIFY" != "" ]; then
  COMMAND="$COMMAND --skip-verify"
fi

# 执行部署命令
cd "$DEPLOY_DIR" || exit

# 构建环境变量
ENV_VARS=""

# 添加必要的环境变量
ENV_VARS="RPC_ENDPOINT=$RPC_ENDPOINT"
ENV_VARS="$ENV_VARS PRIVATE_KEY=$PRIVATE_KEY"

# 执行go命令
$ENV_VARS go run main.go \
  --config /path/to/your/config.yml \
  --contract $CONTRACT \
  --network $NETWORK \
  --output $DEPLOY_RECORDS_DIR \
  --verbose \
  --skip-verify

# 检查部署是否成功
if [ $? -ne 0 ]; then
  show_error "合约部署失败"
  exit 1
fi

# 显示部署完成
divider_nocontext
log_date
COMPONENT="^[[1;32m永久级长!^[[0m"
printf "^[[1;36m部署记录已保存到: $DEPLOY_RECORDS_DIR/^[[0m\n"
printf "^[[1;36mFinal chain合约状态已部署，即系统合约 是: %s!\n\n^[[0m" $CONTRACT

# 返回成功
return 0
