# AIB 智能合约

本目录包含 AIB 协议的核心智能合约实现。

## 合约说明

### 1. AIBToken.sol
AIB 生态代币合约，支持 ERC20 标准和以下功能：

- 总供应量：3,141,592,653 AIB (精确到 10^18 位小数)
- **质押功能** (stake/unstake)：允许用户质押代币获取奖励权重
- **委托功能** (delegate)：允许质押者将投票权委托给其他地址
- **投票权计算**：投票权重 = 余额 + 质押余额 + 收到的委托

#### 核心事件
- `Staked(address indexed user, uint256 amount)` - 质押事件
- `Unstaked(address indexed user, uint256 amount)` - 解除质押事件
- `Delegated(address indexed delegator, address indexed delegatee, uint256 amount)` - 委托事件
- `Undelegated(address indexed delegator, address indexed delegatee, uint256 amount)` - 取消委托事件

### 2. StakingRewards.sol
质押奖励合约，管理奖励分配和惩罚机制：

- **区块奖励分配**：每区块固定量奖励池
- **奖励累积**：按时间累积用户应得奖励
- **惩罚机制**：支持管理员的惩罚执行功能
- **紧急提取**：用户在紧急情况下可提取质押代币

#### 配置参数
- `rewardPerBlock`：每区块奖励数量
- `slashRate`：惩罚系数 (基数 10000，如 500 = 5%)

#### 核心事件
- `RewardsClaimed(address indexed user, uint256 amount)` - 奖励领取
- `Slashed(address indexed account, uint256 amount)` - 惩罚执行

### 3. Governance.sol
治理合约，支持提案创建、投票和执行：

- **提案系统**：多目标调用提案
- **投票机制**：支持对于、反对、弃权
- **投票权重**：基于代币余额、质押和委托
- **提案门槛**：提案者需达到最低投票权
- **法定票数**：提案有效需达到的至少投票权

#### 配置参数
- `votingPeriod`：投票周期（区块数）
- `votingDelay`：投票延迟（悬赏期，区块数）
- `proposalThreshold`：提案门槛（投票权）
- `quorumVotes`：法定票数（有效投票权）

#### 提案状态
- 0 Pending - 投票未开始
- 1 Active - 投票进行中
- 2 Canceled - 已取消
- 3 Defeated - 未通过
- 4 Succeeded - 通过待执行
- 5 Queued - 排队中
- 6 Expired - 已过期
- 7 Executed - 已执行

## 部署说明

### 前置条件
- Solidity 编译器 >= 0.8.20
- 本地或测试网 EVM 节点
- 部署者账户有充足 ETH 和 AIB 代币（初始全部在合约创建者）

### 部署顺序

```solidity
// 1. 部署 AIBToken (自动创建总供应量)
AIBToken token = new AIBToken();

// 2. 部署 StakingRewards
StakingRewards rewards = new StakingRewards(address(token), rewardPerBlock);

// 3. 部署 Governance
Governance gov = new Governance(
    address(token),
    votingPeriod,
    votingDelay,
    proposalThreshold,
    quorumVotes
);
```

### 初始化步骤

1. 使用部署者账户（持有全部代币）调用 `AIBToken.approve(stakingRewards, amount)` 授权奖励池
2. 用户需要先 `AIBToken.transfer` 获得代币，然后：
   - `AIBToken.stake(amount)` 质押以获取投票权和奖励权重
   - `AIBToken.delegate(delegatee)` 将投票权委托给他人

3. 访问 `StakingRewards` 领取奖励：
   - 定期检查 `getPendingRewards()`
   - 调用 `claimRewards()` 领取

4. 治理参与：
   - 确保当前投票权 >= `proposalThreshold` 才能创建提案
   - 在投票期内调用 `vote(proposalId, support)` 投票
   - 通过的提案可调用 `execute()` 执行

## 安全考虑

### 重入攻击防护
- 使用 "checks-effects-interactions" 模式
- 关键操作前更新状态

### 整数溢出防护
- Solidity 0.8.x 自带溢出检查

### 授权验证
- 管理函数使用 `onlyOwner` 限制权限
- 提案创建验证投票权门槛

## 测试建议

1. **AIBToken 测试**
   - 转账、授权、从授权转账
   - 质押、解除质押
   - 委托、取消委托
   - 投票权计算准确性

2. **StakingRewards 测试**
   - 奖励累积计算
   - 质押/解除质押奖励权益计算
   - 惩罚逻辑
   - 奖励领取

3. **Governance 测试**
   - 提案创建门槛验证
   - 投票权重计算
   - 提案状态转换
   - 提案执行

## 接口定义

所有接口定义位于 `interfaces/` 目录：
- `IAIBToken.sol` - 代币接口
- `IStakingRewards.sol` - 奖励接口
- `IGovernance.sol` - 治理接口
