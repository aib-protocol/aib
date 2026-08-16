// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./interfaces/IAIBToken.sol";
import "./interfaces/IGovernance.sol";

/**
 * @title Governance
 * @dev AIB 治理合约
 * @dev 支持提案创建、投票和执行
 */
contract Governance {
    // 代币合约
    IAIBToken public immutable aibToken;

    // 提案存储
    Proposal[] public proposals;

    // 投票权重缓存
    uint256 private latestBlockNumber;

    // 投票统计
    struct VoteInfo {
        uint256 support;      // 0=against, 1=for, 2=abstain
        uint256 weight;       // 投票权重
    }

    mapping(uint256 => mapping(address => VoteInfo)) public votes; // proposalId => voter => voteInfo

    // 检查是否已投票
    mapping(uint256 => mapping(address => bool)) public hasVoted;

    // 配置参数
    uint256 public votingPeriod;
    uint256 public votingDelay;
    uint256 public proposalThreshold;
    uint256 public quorumVotes;

    // 参数修改管理器
    address public parametersManager;

    // 事件
    event ProposalCreated(uint256 indexed proposalId, address indexed proposer, bytes32 descriptionHash);
    event VoteCast(address indexed voter, uint256 proposalId, uint8 support, uint256 weight);
    event ProposalExecuted(uint256 indexed proposalId);
    event ProposalCanceled(uint256 indexed proposalId);
    event ParametersUpdated(uint256 votingPeriod, uint256 votingDelay, uint256 proposalThreshold, uint256 quorumVotes);

    // 提案状态名称（用于查询）
    string[8] private proposalStateName;

    constructor(
        address _aibToken,
        uint256 _votingPeriod,
        uint256 _votingDelay,
        uint256 _proposalThreshold,
        uint256 _quorumVotes
    ) {
        require(_aibToken != address(0), "Invalid token address");
        aibToken = IAIBToken(_aibToken);
        votingPeriod = _votingPeriod;
        votingDelay = _votingDelay;
        proposalThreshold = _proposalThreshold;
        quorumVotes = _quorumVotes;
        parametersManager = msg.sender;

        // 初始化状态名称映射
        proposalStateName[uint256(ProposalState.Pending)] = "Pending";
        proposalStateName[uint256(ProposalState.Active)] = "Active";
        proposalStateName[uint256(ProposalState.Canceled)] = "Canceled";
        proposalStateName[uint256(ProposalState.Defeated)] = "Defeated";
        proposalStateName[uint256(ProposalState.Succeeded)] = "Succeeded";
        proposalStateName[uint256(ProposalState.Queued)] = "Queued";
        proposalStateName[uint256(ProposalState.Expired)] = "Expired";
        proposalStateName[uint256(ProposalState.Executed)] = "Executed";
    }

    // ============ 提案创建 ============

    function propose(
        address[] calldata targets,
        uint256[] calldata values,
        bytes[] calldata calldatas,
        string memory description
    ) external override returns (uint256 proposalId) {
        require(targets.length == values.length && targets.length == calldatas.length, "Arrays length mismatch");
        require(targets.length > 0, "No actions provided");
        require(bytes(description).length > 0, "No description");

        // 检查提案者是否有足够投票权重
        uint256 proposerVotes = getPriorVotes(msg.sender, block.number);
        require(proposerVotes >= proposalThreshold, "Insufficient proposal power");

        bytes32 descriptionHash = keccak256(bytes(description));
        require(proposals.length < 2**128, "Proposal capacity exceeded");

        Proposal storage proposal = proposals.push();
        proposal.id = proposals.length - 1;
        proposal.proposer = msg.sender;
        proposal.startBlock = block.number + votingDelay;
        proposal.endBlock = proposal.startBlock + votingPeriod;
        proposal.forVotes = 0;
        proposal.againstVotes = 0;
        proposal.abstainVotes = 0;
        proposal.executed = false;
        proposal.canceled = false;
        proposal.descriptionHash = descriptionHash;
        proposal.targets = targets;
        proposal.values = values;
        proposal.calldatas = calldatas;

        emit ProposalCreated(proposal.id, msg.sender, descriptionHash);

        return proposal.id;
    }

    // ============ 投票 ============

    function vote(uint256 proposalId, uint8 support) external override {
        require(proposalId < proposals.length, "Invalid proposal ID");
        Proposal storage proposal = proposals[proposalId];
        require(block.number >= proposal.startBlock, "Voting not started");
        require(block.number <= proposal.endBlock, "Voting ended");
        require(!proposal.canceled, "Proposal canceled");
        require(!hasVoted[proposalId][msg.sender], "Already voted");

        uint256 voteWeight = getPriorVotes(msg.sender, proposal.startBlock);
        require(voteWeight > 0, "No voting power");

        VoteInfo storage vote = votes[proposalId][msg.sender];
        vote.support = support;
        vote.weight = voteWeight;
        hasVoted[proposalId][msg.sender] = true;

        // 累加到总投票数
        if (support == 1) {
            proposal.forVotes += voteWeight;
        } else if (support == 0) {
            proposal.againstVotes += voteWeight;
        } else {
            proposal.abstainVotes += voteWeight;
        }

        emit VoteCast(msg.sender, proposalId, support, voteWeight);
    }

    // ============ 提案执行 ============

    function execute(address[] calldata targets, uint256[] calldata values, bytes[] calldata calldatas, bytes32 descriptionHash) external payable override returns (bool) {
        uint256 proposalId = getProposalId(targets, values, calldatas, descriptionHash);
        require(proposalId < proposals.length, "Invalid proposal ID");
        Proposal storage proposal = proposals[proposalId];

        require(!proposal.canceled, "Proposal was canceled");
        require(proposal.executed == false, "Already executed");
        require(state(proposalId) == ProposalState.Succeeded, "Not succeeded");

        // 执行所有调用
        for (uint256 i = 0; i < targets.length; i++) {
            (bool success, bytes memory returnData) = targets[i].call{value: values[i]}(calldatas[i]);
            require(success, "Transaction execution failed");
        }

        proposal.executed = true;

        emit ProposalExecuted(proposalId);
        return true;
    }

    // ============ 提案取消 ============

    function cancel(uint256 proposalId) external override {
        require(proposalId < proposals.length, "Invalid proposal ID");
        Proposal storage proposal = proposals[proposalId];

        require(!proposal.executed, "Already executed");
        require(msg.sender == proposal.proposer || msg.sender == parametersManager, "Not authorized");

        proposal.canceled = true;

        emit ProposalCanceled(proposalId);
    }

    // ============ 查询函数 ============

    function state(uint256 proposalId) public view override returns (ProposalState) {
        require(proposalId < proposals.length, "Invalid proposal ID");
        Proposal storage proposal = proposals[proposalId];

        if (proposal.canceled) {
            return ProposalState.Canceled;
        }

        if (block.number < proposal.startBlock) {
            return ProposalState.Pending;
        }

        if (block.number > proposal.endBlock) {
            uint256 votesFor = proposal.forVotes;
            uint256 votesAgainst = proposal.againstVotes;
            uint256 totalVotes = votesFor + votesAgainst;

            if (totalVotes >= quorumVotes && votesFor > votesAgainst) {
                return ProposalState.Succeeded;
            }
            return ProposalState.Defeated;
        }

        return ProposalState.Active;
    }

    function getProposal(uint256 proposalId) external view override returns (Proposal memory) {
        require(proposalId < proposals.length, "Invalid proposal ID");
        return proposals[proposalId];
    }

    function getVotes(address account) external view override returns (uint256) {
        // 获取当前投票权重：余额 + 质押 + 收到的委托 - 已委托出的
        return aibToken.getVotes(account);
    }

    function getProposalState(uint256 proposalId) external view returns (string memory) {
        uint256 s = state(proposalId);
        return proposalStateName[s];
    }

    // 获取某提案的投票详情
    function getProposalVotes(uint256 proposalId) external view returns (
        uint256 totalVotes,
        uint256 forVotes,
        uint256 againstVotes,
        uint256 abstainVotes
    ) {
        require(proposalId < proposals.length, "Invalid proposal ID");
        Proposal memory proposal = proposals[proposalId];

        totalVotes = proposal.forVotes + proposal.againstVotes;
        forVotes = proposal.forVotes;
        againstVotes = proposal.againstVotes;
        abstainVotes = proposal.abstainVotes;
    }

    // ============ 投票权重计算 ============

    function getPriorVotes(address account, uint256 blockNumber) public view returns (uint256) {
        // 获取指定区块的投票权重
        // 包括：余额、质押、委托
        uint256 balance = aibToken.balanceOf(account);
        uint256 staked = aibToken.getStakedBalance(account);
        uint256 delegated = aibToken.getDelegatedBalance(account);

        return balance + staked + delegated;
    }

    function getProposalId(
        address[] calldata targets,
        uint256[] calldata values,
        bytes[] calldata calldatas,
        bytes32 descriptionHash
    ) public pure returns (uint256) {
        bytes32 proposalHash = keccak256(
            abi.encodePacked(targets, values, calldatas, descriptionHash)
        );
        return uint256(proposalHash);
    }

    // ============ 管理函数 ============

    function setVotingPeriod(uint256 _votingPeriod) external {
        require(msg.sender == parametersManager, "Not authorized");
        votingPeriod = _votingPeriod;
    }

    function setVotingDelay(uint256 _votingDelay) external {
        require(msg.sender == parametersManager, "Not authorized");
        votingDelay = _votingDelay;
    }

    function setProposalThreshold(uint256 _proposalThreshold) external {
        require(msg.sender == parametersManager, "Not authorized");
        proposalThreshold = _proposalThreshold;
    }

    function setQuorumVotes(uint256 _quorumVotes) external {
        require(msg.sender == parametersManager, "Not authorized");
        quorumVotes = _quorumVotes;
    }

    function setParametersManager(address _manager) external {
        require(msg.sender == parametersManager, "Not authorized");
        parametersManager = _manager;
    }

    // ============ 视图辅助函数 ============

    function getProposalsLength() external view returns (uint256) {
        return proposals.length;
    }

    function getActiveProposals() external view returns (uint256[]) {
        uint256[] memory active = new uint256[](0);
        for (uint256 i = 0; i < proposals.length; i++) {
            uint256 s = state(i);
            if (s == ProposalState.Active || s == ProposalState.Succeeded || s == ProposalState.Queued) {
                active.push(i);
            }
        }
        return active;
    }

    // 接收 ETH fallback
    receive() external payable {}
    fallback() external payable {}
}
