// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./AIBToken.sol";
import "./StakingRewards.sol";
import "./Governance.sol";

/**
 * @title AIBTokenFactory
 * @dev AIB 合约部署辅助工厂
 */
contract AIBTokenFactory {
    event TokenDeployed(address indexed tokenAddress, address indexed owner);
    event SystemDeployed(address indexed token, address indexed staking, address indexed governance);

    function deployToken() external returns (address) {
        AIBToken token = new AIBToken();
        emit TokenDeployed(address(token), msg.sender);
        return address(token);
    }

    // 批量部署所有合约
    function deploySystem() external returns (
        address aibTokenAddr,
        address stakingRewardsAddr,
        address governanceAddr
    ) {
        // 1. 部署 AIBToken
        AIBToken token = new AIBToken();
        aibTokenAddr = address(token);

        // 2. 部署 StakingRewards
        uint256 rewardPerBlock = 10000000000000000; // 每区块 0.01 AIB (以 12秒一个区块计算)
        StakingRewards rewards = new StakingRewards(aibTokenAddr, rewardPerBlock);
        stakingRewardsAddr = address(rewards);

        // 3. 部署 Governance
        Governance gov = new Governance(
            aibTokenAddr,
            7200,              // 投票期 7200 个区块 (约 24 小时)
            65,                // 投票延迟 65 个块 (约 13 分钟)
            10000 * 10**18,    // 提案门槛 10000 AIB
            100000 * 10**18    // 法定票数 100000 AIB
        );
        governanceAddr = address(gov);

        emit SystemDeployed(aibTokenAddr, stakingRewardsAddr, governanceAddr);

        return (aibTokenAddr, stakingRewardsAddr, governanceAddr);
    }
}
