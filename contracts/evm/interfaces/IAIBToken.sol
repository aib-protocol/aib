// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface IAIBToken {
    // ERC20 base functions
    function totalSupply() external view returns (uint256);
    function balanceOf(address account) external view returns (uint256);
    function transfer(address to, uint256 amount) external returns (bool);
    function allowance(address owner, address spender) external view returns (uint256);
    function approve(address spender, uint256 amount) external returns (bool);
    function transferFrom(address from, address to, uint256 amount) external returns (bool);

    // staking functions
    function stake(uint256 amount) external;
    function unstake(uint256 amount) external;
    function getStakedBalance(address account) external view returns (uint256);

    // delegation functions
    function delegate(address delegatee) external;
    function undelegate(address delegatee, uint256 amount) external;
    function getDelegatedBalance(address delegatee) external view returns (uint256);
    function getDelegationAmount(address delegator, address delegatee) external view returns (uint256);

    function getVotes(address account) external view returns (uint256);

    // events
    event Staked(address indexed user, uint256 amount);
    event Unstaked(address indexed user, uint256 amount);
    event Delegated(address indexed delegator, address indexed delegatee, uint256 amount);
    event Undelegated(address indexed delegator, address indexed delegatee, uint256 amount);
}
