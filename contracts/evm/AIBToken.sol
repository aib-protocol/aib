// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title AIBToken
 * @dev AIB 生态代币合约 - 支持质押和委托功能
 * @dev 发行量: 3,141,592,653 AIB
 */
contract AIBToken {
    string public constant name = "AIB Token";
    string public constant symbol = "AIB";
    uint8 public constant decimals = 18;

    // 发行量 3,141,592,653 * 10^18
    uint256 public constant TOTAL_SUPPLY = 3141592653 * 10**uint256(decimals);

    mapping(address => uint256) private _balances;
    mapping(address => mapping(address => uint256)) private _allowances;

    // 质押相关
    mapping(address => uint256) private _stakedBalances;
    uint256 private _totalStaked;

    // 委托相关
    mapping(address => mapping(address => uint256)) private _delegations; // delegator => delegatee => amount
    mapping(address => uint256) private _delegatedToMe; // delegatee => total delegated to them
    mapping(address => address) private _delegation; // address => current delegatee
    mapping(address => uint256) private _totalDelegatedOut; // total amount delegated out by each account

    // 合约部署者
    address public _initialOwner;

    // 修改器
    modifier onlyOwner() {
        require(msg.sender == _initialOwner, "Not the owner");
        _;
    }

    modifier nonZeroAddress(address addr) {
        require(addr != address(0), "Zero address not allowed");
        _;
    }

    constructor() {
        _initialOwner = msg.sender;
        _balances[_initialOwner] = TOTAL_SUPPLY;
        emit Transfer(address(0), _initialOwner, TOTAL_SUPPLY);
    }

    // ============ ERC20 基础函数 ============

    function totalSupply() external view returns (uint256) {
        return TOTAL_SUPPLY;
    }

    function balanceOf(address account) external view returns (uint256) {
        return _balances[account];
    }

    function transfer(address to, uint256 amount) external nonZeroAddress(to) returns (bool) {
        require(_balances[msg.sender] >= amount, "Insufficient balance");

        _balances[msg.sender] -= amount;
        _balances[to] += amount;

        emit Transfer(msg.sender, to, amount);
        return true;
    }

    function allowance(address owner, address spender) external view returns (uint256) {
        return _allowances[owner][spender];
    }

    function approve(address spender, uint256 amount) external nonZeroAddress(spender) returns (bool) {
        _allowances[msg.sender][spender] = amount;
        emit Approval(msg.sender, spender, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        require(_balances[from] >= amount, "Insufficient balance");
        require(_allowances[from][msg.sender] >= amount, "Allowance exceeded");

        _balances[from] -= amount;
        _balances[to] += amount;
        _allowances[from][msg.sender] -= amount;

        emit Transfer(from, to, amount);
        return true;
    }

    // ============ 质押功能 ============

    function stake(uint256 amount) external {
        require(amount > 0, "Cannot stake 0");
        require(_balances[msg.sender] >= amount, "Insufficient balance");

        // 解除可能存在的委托
        address currentDelegate = _delegation[msg.sender];
        if (currentDelegate != address(0)) {
            uint256 delegatedAmount = _delegations[msg.sender][currentDelegate];
            if (delegatedAmount > 0) {
                _delegations[msg.sender][currentDelegate] = 0;
                _delegatedToMe[currentDelegate] -= delegatedAmount;
                emit Undelegated(msg.sender, currentDelegate, delegatedAmount);
            }
        }

        _balances[msg.sender] -= amount;
        _stakedBalances[msg.sender] += amount;
        _totalStaked += amount;

        emit Staked(msg.sender, amount);
    }

    function unstake(uint256 amount) external {
        require(amount > 0, "Cannot unstake 0");

        // 检查不能解除已委托的代币
        uint256 available = _stakedBalances[msg.sender] - _totalDelegatedOut[msg.sender];
        require(available >= amount, "Cannot unstake delegated tokens");

        _stakedBalances[msg.sender] -= amount;
        _totalStaked -= amount;
        _balances[msg.sender] += amount;

        emit Unstaked(msg.sender, amount);
    }

    function getStakedBalance(address account) external view returns (uint256) {
        return _stakedBalances[account];
    }

    function getTotalStaked() external view returns (uint256) {
        return _totalStaked;
    }

    // ============ 委托功能 ============

    function delegate(address delegatee) external nonZeroAddress(delegatee) {
        require(_stakedBalances[msg.sender] > 0, "No staked balance to delegate");

        // 先解除之前的委托
        address previousDelegate = _delegation[msg.sender];
        if (previousDelegate != address(0)) {
            uint256 prevAmount = _delegations[msg.sender][previousDelegate];
            if (prevAmount > 0) {
                _delegations[msg.sender][previousDelegate] = 0;
                _delegatedToMe[previousDelegate] -= prevAmount;
                _totalDelegatedOut[msg.sender] -= prevAmount;
                emit Undelegated(msg.sender, previousDelegate, prevAmount);
            }
        }

        // 计算可委托金额 (质押余额 - 已委托出)
        uint256 available = _stakedBalances[msg.sender] - _totalDelegatedOut[msg.sender];
        require(available > 0, "No available balance to delegate");

        _delegation[msg.sender] = delegatee;
        _delegations[msg.sender][delegatee] = available;
        _delegatedToMe[delegatee] += available;
        _totalDelegatedOut[msg.sender] += available;

        emit Delegated(msg.sender, delegatee, available);
    }

    function undelegate(address delegatee, uint256 amount) external {
        require(amount > 0, "Cannot undelegate 0");
        require(_delegations[msg.sender][delegatee] >= amount, "Insufficient delegated amount");

        _delegations[msg.sender][delegatee] -= amount;
        _delegatedToMe[delegatee] -= amount;
        _totalDelegatedOut[msg.sender] -= amount;

        if (_delegations[msg.sender][delegatee] == 0 && _delegation[msg.sender] == delegatee) {
            _delegation[msg.sender] = address(0);
        }

        emit Undelegated(msg.sender, delegatee, amount);
    }

    function getDelegatedBalance(address delegatee) external view returns (uint256) {
        return _delegatedToMe[delegatee];
    }

    function getDelegationAmount(address delegator, address delegatee) external view returns (uint256) {
        return _delegations[delegator][delegatee];
    }

    function getDelegate(address account) external view returns (address) {
        return _delegation[account];
    }

    // ============ 投票权重计算 ============

    function getVotes(address account) external view returns (uint256) {
        uint256 balance = _balances[account];
        uint256 staked = _stakedBalances[account];
        uint256 delegatedOut = _totalDelegatedOut[account];
        uint256 delegatedIn = _delegatedToMe[account];

        require(staked >= delegatedOut, "Invalid delegation data");
        return balance + (staked - delegatedOut) + delegatedIn;
    }

    // ============ 事件 ============

    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);
    event Staked(address indexed user, uint256 amount);
    event Unstaked(address indexed user, uint256 amount);
    event Delegated(address indexed delegator, address indexed delegatee, uint256 amount);
    event Undelegated(address indexed delegator, address indexed delegatee, uint256 amount);
}
