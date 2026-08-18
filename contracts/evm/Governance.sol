// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./interfaces/IAIBToken.sol";
import "./interfaces/IGovernance.sol";

/**
 * @title Governance
 * @dev AIB governance contract
 * @dev Supports proposal creation, voting, and execution
 */
contract Governance {
    // token contract
    IAIBToken public immutable aibToken;

    // proposal storage
    Proposal[] public proposals;

    // cast-vote weight cache
    uint256 private latestBlockNumber;

    // cast-vote statistics
    struct VoteInfo {
        uint256 support;      // 0=against, 1=for, 2=abstain
        uint256 weight;       // cast-vote weight
    }

    mapping(uint256 => mapping(address => VoteInfo)) public votes; // proposalId => voter => voteInfo

    // check if already voted
    mapping(uint256 => mapping(address => bool)) public hasVoted;

    // config parameters
    uint256 public votingPeriod;
    uint256 public votingDelay;
    uint256 public proposalThreshold;
    uint256 public quorumVotes;

    // parameter-change manager
    address public parametersManager;

    // events
    event ProposalCreated(uint256 indexed proposalId, address indexed proposer, bytes32 descriptionHash);
    event VoteCast(address indexed voter, uint256 proposalId, uint8 support, uint256 weight);
    event ProposalExecuted(uint256 indexed proposalId);
    event ProposalCanceled(uint256 indexed proposalId);
    event ParametersUpdated(uint256 votingPeriod, uint256 votingDelay, uint256 proposalThreshold, uint256 quorumVotes);

    // proposal status names (for queries)
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

        // initialize status name mapping
        proposalStateName[uint256(ProposalState.Pending)] = "Pending";
        proposalStateName[uint256(ProposalState.Active)] = "Active";
        proposalStateName[uint256(ProposalState.Canceled)] = "Canceled";
        proposalStateName[uint256(ProposalState.Defeated)] = "Defeated";
        proposalStateName[uint256(ProposalState.Succeeded)] = "Succeeded";
        proposalStateName[uint256(ProposalState.Queued)] = "Queued";
        proposalStateName[uint256(ProposalState.Expired)] = "Expired";
        proposalStateName[uint256(ProposalState.Executed)] = "Executed";
    }

    // ============ proposal creation ============

    function propose(
        address[] calldata targets,
        uint256[] calldata values,
        bytes[] calldata calldatas,
        string memory description
    ) external override returns (uint256 proposalId) {
        require(targets.length == values.length && targets.length == calldatas.length, "Arrays length mismatch");
        require(targets.length > 0, "No actions provided");
        require(bytes(description).length > 0, "No description");

        // check proposer has enough voting weight
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

    // ============ voting ============

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

        // add to total votes
        if (support == 1) {
            proposal.forVotes += voteWeight;
        } else if (support == 0) {
            proposal.againstVotes += voteWeight;
        } else {
            proposal.abstainVotes += voteWeight;
        }

        emit VoteCast(msg.sender, proposalId, support, voteWeight);
    }

    // ============ proposal execution ============

    function execute(address[] calldata targets, uint256[] calldata values, bytes[] calldata calldatas, bytes32 descriptionHash) external payable override returns (bool) {
        uint256 proposalId = getProposalId(targets, values, calldatas, descriptionHash);
        require(proposalId < proposals.length, "Invalid proposal ID");
        Proposal storage proposal = proposals[proposalId];

        require(!proposal.canceled, "Proposal was canceled");
        require(proposal.executed == false, "Already executed");
        require(state(proposalId) == ProposalState.Succeeded, "Not succeeded");

        // execute all calls
        for (uint256 i = 0; i < targets.length; i++) {
            (bool success, bytes memory returnData) = targets[i].call{value: values[i]}(calldatas[i]);
            require(success, "Transaction execution failed");
        }

        proposal.executed = true;

        emit ProposalExecuted(proposalId);
        return true;
    }

    // ============ proposal cancellation ============

    function cancel(uint256 proposalId) external override {
        require(proposalId < proposals.length, "Invalid proposal ID");
        Proposal storage proposal = proposals[proposalId];

        require(!proposal.executed, "Already executed");
        require(msg.sender == proposal.proposer || msg.sender == parametersManager, "Not authorized");

        proposal.canceled = true;

        emit ProposalCanceled(proposalId);
    }

    // ============ view functions ============

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
        // current voting weight: balance + stake + delegated-in - delegated-out
        return aibToken.getVotes(account);
    }

    function getProposalState(uint256 proposalId) external view returns (string memory) {
        uint256 s = state(proposalId);
        return proposalStateName[s];
    }

    // get vote details of a proposal
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

    // ============ voting weight ============

    function getPriorVotes(address account, uint256 blockNumber) public view returns (uint256) {
        // get voting weight at a given block
        // includes: balance, stake, delegation
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

    // ============ admin functions ============

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

    // ============ view helpers ============

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

    // receive ETH fallback
    receive() external payable {}
    fallback() external payable {}
}
