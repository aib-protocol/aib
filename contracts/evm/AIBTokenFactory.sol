// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./AIBToken.sol";
import "./StakingRewards.sol";
import "./Governance.sol";

/**
 * @title AIBTokenFactory
 * @dev AIB contract deployment helper factory
 */
contract AIBTokenFactory {
    event TokenDeployed(address indexed tokenAddress, address indexed owner);
    event SystemDeployed(address indexed token, address indexed staking, address indexed governance);

    function deployToken() external returns (address) {
        AIBToken token = new AIBToken();
        emit TokenDeployed(address(token), msg.sender);
        return address(token);
    }

    // deploy all contracts in batch
    function deploySystem() external returns (
        address aibTokenAddr,
        address stakingRewardsAddr,
        address governanceAddr
    ) {
        // 1. deploy AIBToken
        AIBToken token = new AIBToken();
        aibTokenAddr = address(token);

        // 2. deploy StakingRewards
        uint256 rewardPerBlock = 10000000000000000; // 0.01 AIB per block (12s blocks)
        StakingRewards rewards = new StakingRewards(aibTokenAddr, rewardPerBlock);
        stakingRewardsAddr = address(rewards);

        // 3. deploy Governance
        Governance gov = new Governance(
            aibTokenAddr,
            7200,              // voting period: 7200 blocks (~24h)
            65,                // voting delay: 65 blocks (~13min)
            10000 * 10**18,    // proposal threshold: 10000 AIB
            100000 * 10**18    // quorum: 100000 AIB
        );
        governanceAddr = address(gov);

        emit SystemDeployed(aibTokenAddr, stakingRewardsAddr, governanceAddr);

        return (aibTokenAddr, stakingRewardsAddr, governanceAddr);
    }
}
