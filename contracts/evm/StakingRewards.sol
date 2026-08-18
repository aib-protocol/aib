// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./interfaces/IStakingRewards.sol";

/**
 * @title StakingRewards
 * @dev AIB staking rewards contract
 * @dev Supports per-block reward distribution, slashing, and reward accumulation
 */
contract StakingRewards is IStakingRewards {
    // token contract
    IAIBToken public immutable token;

    // reward per block
    uint256 public rewardPerBlock;
    uint256 private constant REWARD_PRECISION = 1e18;

    // globally accumulated rewards
    uint256 public rewardPerTokenStored;
    uint256 public lastUpdateTime;

    // user-related
    struct UserInfo {
        uint256 amount;          // staked amount
        uint256 rewardDebt;      // reward debt
        uint256 pendingRewards;  // pending rewards
        uint256 lastStakeTime;   // last stake time
    }

    mapping(address => UserInfo) public userInfo;

    // total staked
    uint256 public totalStaked;

    // contract owner
    address public owner;

    // penalty factor (basis 10000, e.g. 500 = 5%)
    uint256 public slashRate = 500;

    // modifiers
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

    // ============ core view functions ============

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

    // ============ staking ============

    function stake(uint256 amount) external override updateReward(msg.sender) {
        require(amount > 0, "Amount must be > 0");

        // transfer from user to contract
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

        // return tokens
        token.transfer(msg.sender, amount);

        emit Unstaked(msg.sender, amount);
    }

    // ============ rewards ============

    function claimRewards() external override updateReward(msg.sender) {
        UserInfo storage user = userInfo[msg.sender];
        uint256 rewards = user.pendingRewards;

        require(rewards > 0, "No rewards to claim");

        user.pendingRewards = 0;

        // assumes the token contract can mint or the contract is pre-funded
        // adjust in actual deployment
        require(token.balanceOf(address(this)) >= rewards, "Insufficient reward pool");

        token.transfer(msg.sender, rewards);

        emit RewardsClaimed(msg.sender, rewards);
    }

    function getPendingRewards(address account) external view override returns (uint256) {
        return earned(account);
    }

    // ============ admin ============

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

        // compute penalty amount
        uint256 slashAmount = (amount * slashRate) / 10000;
        uint256 remaining = amount - slashAmount;

        // reduce stake
        user.amount -= amount;
        totalStaked -= amount;

        // return the unpenalized part
        if (remaining > 0) {
            token.transfer(account, remaining);
        }

        // penalized tokens can be burned or returned to owner
        // here returned to owner as reward-pool replenishment
        if (slashAmount > 0) {
            token.transfer(owner, slashAmount);
        }

        emit Slashed(account, slashAmount);
    }

    // ============ views ============

    function getStakedBalance(address account) external view override returns (uint256) {
        return userInfo[account].amount;
    }

    function getTotalStaked() external view override returns (uint256) {
        return totalStaked;
    }

    // ============ emergency ============

    function emergencyWithdraw() external {
        UserInfo storage user = userInfo[msg.sender];
        uint256 amount = user.amount;

        require(amount > 0, "No staked amount");

        user.amount = 0;
        totalStaked -= amount;

        // forfeit unclaimed rewards
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

// simplified IAIBToken interface for internal use
interface IAIBToken {
    function transferFrom(address from, address to, uint256 amount) external returns (bool);
    function transfer(address to, uint256 amount) external returns (bool);
    function balanceOf(address account) external view returns (uint256);
}
