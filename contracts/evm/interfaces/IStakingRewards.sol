// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface IStakingRewards {
    // 核心函数
    function stake(uint256 amount) external;
    function unstake(uint256 amount) external;
    function claimRewards() external;
    function getPendingRewards(address account) external view returns (uint256);

    // 管理函数
    function setRewardPerBlock(uint256 _rewardPerBlock) external;
    function slash(address account, uint256 amount) external;

    // 查询函数
    function getStakedBalance(address account) external view returns (uint256);
    function getTotalStaked() external view returns (uint256);

    // 事件
    event Staked(address indexed user, uint256 amount);
    event Unstaked(address indexed user, uint256 amount);
    event RewardsClaimed(address indexed user, uint256 amount);
    event Slashed(address indexed account, uint256 amount);
    event RewardPerBlockUpdated(uint256 newRewardPerBlock);
}
