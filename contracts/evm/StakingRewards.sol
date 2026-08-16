// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./interfaces/IStakingRewards.sol";

/**
 * @title StakingRewards
 * @dev AIB 质押奖励合约
 * @dev 支持区块奖励分配、惩罚机制和奖励累积
 */
contract StakingRewards is IStakingRewards {
    // 代币合约
    IAIBToken public immutable token;

    // 每区块奖励
    uint256 public rewardPerBlock;
    uint256 private constant REWARD_PRECISION = 1e18;

    // 全局累积奖励
    uint256 public rewardPerTokenStored;
    uint256 public lastUpdateTime;

    // 用户相关
    struct UserInfo {
        uint256 amount;          // 质押数量
        uint256 rewardDebt;      // 奖励债务
        uint256 pendingRewards;  // 待领取奖励
        uint256 lastStakeTime;   // 最后质押时间
    }

    mapping(address => UserInfo) public userInfo;

    // 总质押量
    uint256 public totalStaked;

    // 合约所有者
    address public owner;

    // 惩罚系数 (基数为 10000，如 500 = 5%)
    uint256 public slashRate = 500;

    // 修改器
    modifier onlyOwner() {
        require(msg.sender == owner, "Not the owner");
        _;
    }

    modifier updateReward(address account) {
        rewardPerTokenStored = rewardPerToken();
        lastUpdateTime = block.timestamp;

        if (account != address(0)) {
            UserInfo storage user = userInfo[account];
            user.pendingRewards += earned(account);
            user.rewardDebt = (user.amount * rewardPerTokenStored) / REWARD_PRECISION;
        }
        _;
    }

    constructor(address _token, uint256 _rewardPerBlock) {
        require(_token != address(0), "Invalid token address");
        token = IAIBToken(_token);
        rewardPerBlock = _rewardPerBlock;
        owner = msg.sender;
        lastUpdateTime = block.timestamp;
    }

    // ============ 核心视图函数 ============

    function rewardPerToken() public view returns (uint256) {
        if (totalStaked == 0) {
            return rewardPerTokenStored;
        }
        uint256 blocksPassed = (block.timestamp - lastUpdateTime);
        uint256 rewards = blocksPassed * rewardPerBlock;
        return rewardPerTokenStored + (rewards * REWARD_PRECISION / totalStaked);
    }

    function earned(address account) public view returns (uint256) {
        UserInfo memory user = userInfo[account];
        uint256 currentRewardPerToken = rewardPerToken();
        uint256 userReward = (user.amount * currentRewardPerToken) / REWARD_PRECISION;
        return userReward - user.rewardDebt + user.pendingRewards;
    }

    // ============ 质押功能 ============

    function stake(uint256 amount) external override updateReward(msg.sender) {
        require(amount > 0, "Amount must be > 0");

        // 从用户转账到合约
        token.transferFrom(msg.sender, address(this), amount);

        UserInfo storage user = userInfo[msg.sender];
        user.amount += amount;
        user.lastStakeTime = block.timestamp;
        totalStaked += amount;

        emit Staked(msg.sender, amount);
    }

    function unstake(uint256 amount) external override updateReward(msg.sender) {
        require(amount > 0, "Amount must be > 0");
        UserInfo storage user = userInfo[msg.sender];
        require(user.amount >= amount, "Insufficient staked amount");

        user.amount -= amount;
        totalStaked -= amount;

        // 转回代币
        token.transfer(msg.sender, amount);

        emit Unstaked(msg.sender, amount);
    }

    // ============ 奖励功能 ============

    function claimRewards() external override updateReward(msg.sender) {
        UserInfo storage user = userInfo[msg.sender];
        uint256 rewards = user.pendingRewards;

        require(rewards > 0, "No rewards to claim");

        user.pendingRewards = 0;

        // 这里假设 token 合约有 mint 功能或合约预先充值了奖励
        // 实际部署时需要根据具体情况调整
        require(token.balanceOf(address(this)) >= rewards, "Insufficient reward pool");

        token.transfer(msg.sender, rewards);

        emit RewardsClaimed(msg.sender, rewards);
    }

    function getPendingRewards(address account) external view override returns (uint256) {
        return earned(account);
    }

    // ============ 管理功能 ============

    function setRewardPerBlock(uint256 _rewardPerBlock) external override onlyOwner {
        rewardPerBlock = _rewardPerBlock;
        emit RewardPerBlockUpdated(_rewardPerBlock);
    }

    function setSlashRate(uint256 _slashRate) external onlyOwner {
        require(_slashRate <= 10000, "Slash rate too high");
        slashRate = _slashRate;
    }

    function slash(address account, uint256 amount) external override onlyOwner {
        require(account != address(0), "Invalid address");
        UserInfo storage user = userInfo[account];
        require(user.amount >= amount, "Amount exceeds staked balance");

        // 计算惩罚量
        uint256 slashAmount = (amount * slashRate) / 10000;
        uint256 remaining = amount - slashAmount;

        // 减少质押
        user.amount -= amount;
        totalStaked -= amount;

        // 返还未惩罚部分
        if (remaining > 0) {
            token.transfer(account, remaining);
        }

        // 惩罚的代币可以销毁或转回所有者
        // 这里选择转回所有者作为奖励池补充
        if (slashAmount > 0) {
            token.transfer(owner, slashAmount);
        }

        emit Slashed(account, slashAmount);
    }

    // ============ 查询功能 ============

    function getStakedBalance(address account) external view override returns (uint256) {
        return userInfo[account].amount;
    }

    function getTotalStaked() external view override returns (uint256) {
        return totalStaked;
    }

    // ============ 紧急功能 ============

    function emergencyWithdraw() external {
        UserInfo storage user = userInfo[msg.sender];
        uint256 amount = user.amount;

        require(amount > 0, "No staked amount");

        user.amount = 0;
        totalStaked -= amount;

        // 放弃未领取的奖励
        user.pendingRewards = 0;

        token.transfer(msg.sender, amount);

        emit Unstaked(msg.sender, amount);
    }

    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "Invalid address");
        owner = newOwner;
    }

    function recoverTokens(address _token, uint256 amount) external onlyOwner {
        require(_token != address(token), "Cannot recover staking token");
        IAIBToken(_token).transfer(owner, amount);
    }
}

// 简化的 IAIBToken 接口，用于内部使用
interface IAIBToken {
    function transferFrom(address from, address to, uint256 amount) external returns (bool);
    function transfer(address to, uint256 amount) external returns (bool);
    function balanceOf(address account) external view returns (uint256);
}
