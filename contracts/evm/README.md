# AIB Smart Contracts

This directory contains the core smart contract implementations of the AIB protocol.

## Contract Overview

### 1. AIBToken.sol
AIB ecosystem token contract, supporting the ERC20 standard with the following features:

- Total supply: 3,141,592,653 AIB (precise to 10^18 decimals)
- **Staking** (stake/unstake): allows users to stake tokens to earn reward weight
- **Delegation** (delegate): allows stakers to delegate voting power to other addresses
- **Voting power calculation**: voting weight = balance + staked balance + received delegations

#### Core Events
- `Staked(address indexed user, uint256 amount)` - staking event
- `Unstaked(address indexed user, uint256 amount)` - unstaking event
- `Delegated(address indexed delegator, address indexed delegatee, uint256 amount)` - delegation event
- `Undelegated(address indexed delegator, address indexed delegatee, uint256 amount)` - undelegation event

### 2. StakingRewards.sol
Staking rewards contract, managing reward distribution and slashing:

- **Block reward distribution**: fixed reward pool per block
- **Reward accumulation**: accrues user rewards over time
- **Slashing mechanism**: admin-triggered slashing enforcement
- **Emergency withdrawal**: users can withdraw staked tokens in emergencies

#### Configuration Parameters
- `rewardPerBlock`: reward amount per block
- `slashRate`: slashing rate (basis 10000, e.g. 500 = 5%)

#### Core Events
- `RewardsClaimed(address indexed user, uint256 amount)` - reward claim
- `Slashed(address indexed account, uint256 amount)` - slashing executed

### 3. Governance.sol
Governance contract, supporting proposal creation, voting, and execution:

- **Proposal system**: multi-target call proposals
- **Voting mechanism**: supports for, against, abstain
- **Voting weight**: based on token balance, staking, and delegations
- **Proposal threshold**: proposer must meet minimum voting power
- **Quorum**: minimum voting power required for a proposal to be valid

#### Configuration Parameters
- `votingPeriod`: voting period (in blocks)
- `votingDelay`: voting delay (timelock, in blocks)
- `proposalThreshold`: proposal threshold (voting power)
- `quorumVotes`: quorum (required voting power)

#### Proposal States
- 0 Pending - voting not started
- 1 Active - voting in progress
- 2 Canceled - canceled
- 3 Defeated - rejected
- 4 Succeeded - passed, pending execution
- 5 Queued - queued
- 6 Expired - expired
- 7 Executed - executed

## Deployment Guide

### Prerequisites
- Solidity compiler >= 0.8.20
- Local or testnet EVM node
- Deployer account with sufficient ETH and AIB tokens (initially all tokens are at the contract creator)

### Deployment Order

```solidity
// 1. Deploy AIBToken (automatically creates total supply)
AIBToken token = new AIBToken();

// 2. Deploy StakingRewards
StakingRewards rewards = new StakingRewards(address(token), rewardPerBlock);

// 3. Deploy Governance
Governance gov = new Governance(
    address(token),
    votingPeriod,
    votingDelay,
    proposalThreshold,
    quorumVotes
);
```

### Initialization Steps

1. Using the deployer account (holding all tokens), call `AIBToken.approve(stakingRewards, amount)` to authorize the reward pool
2. Users must first `AIBToken.transfer` to receive tokens, then:
   - `AIBToken.stake(amount)` to stake for voting power and reward weight
   - `AIBToken.delegate(delegatee)` to delegate voting power to others

3. Access `StakingRewards` to claim rewards:
   - Periodically check `getPendingRewards()`
   - Call `claimRewards()` to claim

4. Governance participation:
   - Ensure current voting power >= `proposalThreshold` to create a proposal
   - Call `vote(proposalId, support)` during the voting period
   - Passed proposals can be executed via `execute()`

## Security Considerations

### Reentrancy Protection
- Uses the "checks-effects-interactions" pattern
- State is updated before critical operations

### Integer Overflow Protection
- Solidity 0.8.x includes built-in overflow checks

### Authorization
- Admin functions use `onlyOwner` for access control
- Proposal creation validates voting power threshold

## Testing Recommendations

1. **AIBToken tests**
   - Transfer, approve, transferFrom
   - Stake, unstake
   - Delegate, undelegate
   - Voting power calculation accuracy

2. **StakingRewards tests**
   - Reward accumulation calculation
   - Stake/unstake reward weight calculation
   - Slashing logic
   - Reward claiming

3. **Governance tests**
   - Proposal creation threshold validation
   - Voting weight calculation
   - Proposal state transitions
   - Proposal execution

## Interface Definitions

All interface definitions are located in the `interfaces/` directory:
- `IAIBToken.sol` - token interface
- `IStakingRewards.sol` - rewards interface
- `IGovernance.sol` - governance interface
