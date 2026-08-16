// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface IGovernance {
    // 提案状态
    enum ProposalState {
        Pending,
        Active,
        Canceled,
        Defeated,
        Succeeded,
        Queued,
        Expired,
        Executed
    }

    // 提案类型
    enum VoteType {
        Against,
        For,
        Abstain
    }

    // 提案结构
    struct Proposal {
        uint256 id;
        address proposer;
        uint256 startBlock;
        uint256 endBlock;
        uint256 forVotes;
        uint256 againstVotes;
        uint256 abstainVotes;
        bool executed;
        bool canceled;
        bytes32 descriptionHash;
        address[] targets;
        uint256[] values;
        bytes[] calldatas;
    }

    // 核心函数
    function propose(
        address[] memory targets,
        uint256[] memory values,
        bytes[] memory calldatas,
        string memory description
    ) external returns (uint256);

    function vote(uint256 proposalId, uint8 support) external;
    function execute(
        address[] memory targets,
        uint256[] memory values,
        bytes[] memory calldatas,
        bytes32 descriptionHash
    ) external payable;

    function cancel(uint256 proposalId) external;

    // 查询函数
    function state(uint256 proposalId) external view returns (ProposalState);
    function getProposal(uint256 proposalId) external view returns (Proposal memory);
    function getVotes(address account) external view returns (uint256);

    // 配置参数
    function votingPeriod() external view returns (uint256);
    function votingDelay() external view returns (uint256);
    function proposalThreshold() external view returns (uint256);
    function quorumVotes() external view returns (uint256);

    // 事件
    event ProposalCreated(uint256 indexed proposalId, address indexed proposer, bytes32 descriptionHash);
    event VoteCast(address indexed voter, uint256 proposalId, uint8 support, uint256 weight);
    event ProposalExecuted(uint256 indexed proposalId);
    event ProposalCanceled(uint256 indexed proposalId);
}
