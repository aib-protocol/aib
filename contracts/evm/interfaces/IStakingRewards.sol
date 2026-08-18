// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface IStakingRewards {
    // core functions
    function stake(uint256 amount) external;
    function unstake(uint256 amount) external;
    function claimRewards() external;
    function getPendingRewards(address account) external view returns (uint256);

    // admin functions
    function setRewardPerBlock(uint256 _rewardPerBlock) external;
    function slash(address account, uint256 amount) external;

    // view functions
    function getStakedBalance(address account) external view returns (uint256);
    function getTotalStaked() external view returns (uint256);

    // events
    event Staked(address indexed user, uint256 amount);
    event Unstaked(address indexed user, uint256 amount);
    event RewardsClaimed(address indexed user, uint256 amount);
    event Slashed(address indexed account, uint256 amount);
    event RewardPerBlockUpdated(uint256 newRewardPerBlock);
}
