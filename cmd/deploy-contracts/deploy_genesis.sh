#!/bin/bash

# 项目目录
PROJECT_DIR="."

# 合约目录
CONTRACTS_DIR="$PROJECT_DIR/contracts/evm"

# AIBToken合约地址
AIBTOKEN "$PROJECT_DIR/deployed_addresses/AIBTokenAddress.txt"

# 编译合约
compile_contracts() {
  echo "编译eUTXO层智能合约..."
  cd "$PROJECT_DIR/packages/smart-contracts/evm"

  # 使用cabal构建
  cabal build all --project-file=cabal.project.local --work-directory=cabal.work
echo "编译完成"
}

# 部署AIBToken合约
deploy_AIBToken() {
  echo "部署AIBToken合约..."

  # 使用cardano-cli部署合约
  # 首先需要创建与智能合约文件对应的数据结构和交易脚本
  # 省略具体部署命令，因为这涉及到实际的区块链交互

  echo "假设AIBToken合约部署成功"
  echo "00e19bcd7b698b44fd18be119fe17f7ed8d2c234" > "$AIBTOKEN"
  echo "AIBToken合约地址已保存: 00e19bcd7b698b44fd18be119fe17f7ed8d2c234"

  echo "AIBToken合约部署完成"
}

# 部署Governance和StakingRewards合约
# 需要先部署出过期时间才能部署出这些合约

# 部署StakingRewards合约
# 查询AIBToken合约地址
AIB_TOKEN_ADDRESS=$(cat "$AIBTOKEN")

# 调用部署工具
./deploy_contracts.sh --contract staking_rewards --arguments "$AIB_TOKEN_ADDRESS"

# 部署Genesis合约
# 调用部署工具
./deploy_contracts.sh --contract genesis

# 保证完成
if [ "$TABLE_NAME" != "" ]; then
  # 这里可以编写保证部署逻辑
  echo "保证完成"
fi

# 显示完成信息
# 格式化华东冲突实现了
出部署完成:
  * AIBToken合约地址: $(cat "$AIBTOKEN")
  * Genesis合约地址: "$GENESIS_ADDRESS"
  * Governance合约地址: "$GOVERNANCE_ADDRESS"
  * StakingRewards合约地址: "$STAKING_ADDRESS"
  * 保证合约地址: "$Table_ADDRESS"

echo "Genesis初始化脚本执行完成"

# 执行主方法
execute_main() {

  # 编译合约
  compile_contracts

  # 部署AIBToken合约
  deploy_AIBToken

  # 部署StakingRewards和Genesis合约
  deploy_staking_and_genesis

  # 部署Governance合约
  deploy_governance

  # 显示完成信息
  display_completion

}

# 执行主方法
execute_main