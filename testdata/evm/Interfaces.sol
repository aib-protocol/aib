// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title ERC20 Token Interface
 * @dev Standard ERC20 token interface
 */
interface IERC20 {
    function totalSupply() external view returns (uint256);
    function balanceOf(address account) external view returns (uint256);
    function transfer(address to, uint256 amount) external returns (bool);
    function allowance(address owner, address spender) external view returns (uint256);
    function approve(address spender, uint256 amount) external returns (bool);
    function transferFrom(address from, address to, uint256 amount) external returns (bool);

    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);
}

/**
 * @title ERC721 Token Interface
 * @dev Standard ERC721 token interface
 */
interface IERC721 {
    function balanceOf(address owner) external view returns (uint256);
    function ownerOf(uint256 tokenId) external view returns (address);
    function safeTransferFrom(address from, address to, uint256 tokenId, bytes calldata data) external;
    function safeTransferFrom(address from, address to, uint256 tokenId) external;
    function transferFrom(address from, address to, uint256 tokenId) external;
    function approve(address to, uint256 tokenId) external;
    function setApprovalForAll(address operator, bool approved) external;
    function getApproved(uint256 tokenId) external view returns (address);
    function isApprovedForAll(address owner, address operator) external view returns (bool);

    event Transfer(address indexed from, address indexed to, uint256 indexed tokenId);
    event Approval(address indexed owner, address indexed approved, uint256 indexed tokenId);
    event ApprovalForAll(address indexed owner, address indexed operator, bool approved);
}

/**
 * @title IUniswapV2Factory
 * @dev Uniswap V2 Factory interface
 */
interface IUniswapV2Factory {
    function createPair(address tokenA, address tokenB) external returns (address pair);
    function getPair(address tokenA, address tokenB) external view returns (address pair);
}

/**
 * @title IUniswapV2Pair
 * @dev Uniswap V2 Pair interface
 */
interface IUniswapV2Pair {
    function getReserves() external view returns (uint112 reserve0, uint112 reserve1, uint32 blockTimestampLast);
    function mint(address to) external returns (uint256 liquidity);
    function burn(address to) external returns (uint256 amount0, uint256 amount1);
    function swap(uint256 amount0Out, uint256 amount1Out, address to, bytes calldata data) external;
    function skim(address to) external;
    function sync() external;
}

/**
 * @title IUniswapV2Router
 * @dev Uniswap V2 Router interface
 */
interface IUniswapV2Router {
    function factory() external pure returns (address);
    function WETH() external pure returns (address);

    function addLiquidity(
        address tokenA,
        address tokenB,
        uint amountADesired,
        uint amountBDesired,
        uint amountAMin,
        uint amountBMin,
        address to,
        uint deadline
    ) external returns (uint amountA, uint amountB, uint liquidity);

    function removeLiquidity(
        address tokenA,
        address tokenB,
        uint liquidity,
        uint amountAMin,
        uint amountBMin,
        address to,
        uint deadline
    ) external returns (uint amountA, uint amountB);

    function swapExactTokensForTokens(
        uint amountIn,
        uint amountOutMin,
        address[] calldata path,
        address to,
        uint deadline
    ) external returns (uint[] memory amounts);

    function swapTokensForExactTokens(
        uint amountOut,
        uint amountInMax,
        address[] calldata path,
        address to,
        uint deadline
    ) external returns (uint[] memory amounts);
}

/**
 * @title IFlashloanReceiver
 * @dev Flashloan receiver interface
 */
interface IFlashloanReceiver {
    function executeOperation(
        address asset,
        uint256 amount,
        uint256 premium,
        address initiator,
        bytes calldata params
    ) external returns (bool);
}

/**
 * @title ILendingPool
 * @dev Aave style lending pool interface
 */
interface ILendingPool {
    function flashLoan(
        address receiverAddress,
        address[] calldata assets,
        uint256[] calldata amounts,
        uint256[] calldata modes,
        address onBehalfOf,
        bytes calldata params,
        uint16 referralCode
    ) external;

    function deposit(
        address asset,
        uint256 amount,
        address onBehalfOf,
        uint16 referralCode
    ) external;

    function withdraw(
        address asset,
        uint256 amount,
        address to
    ) external returns (uint256);
}

/**
 * @title IPriceOracle
 * @dev Price oracle interface
 */
interface IPriceOracle {
    function getAssetPrice(address asset) external view returns (uint256);
    function setAssetPrice(address asset, uint256 price) external;
    function updatePrice(address asset, uint256 price) external;
}

/**
 * @title IGovernance
 * @dev Governance interface
 */
interface IGovernance {
    function propose(address[] memory targets, uint256[] memory values, bytes[] memory calldatas, string memory description) external returns (uint256);
    function vote(uint256 proposalId, uint8 support) external;
    function execute(uint256 proposalId) external;
    function cancel(uint256 proposalId) external;
}

/**
 * @title IStaking
 * @dev Staking interface
 */
interface IStaking {
    function stake(uint256 amount) external;
    function unstake(uint256 amount) external;
    function claimRewards() external;
    function getTotalStaked(address user) external view returns (uint256);
    function getPendingRewards(address user) external view returns (uint256);
}

/**
 * @title IMultisigWallet
 * @dev Multisig wallet interface
 */
interface IMultisigWallet {
    function submitTransaction(address destination, uint256 value, bytes calldata data) external returns (uint256);
    function confirmTransaction(uint256 transactionId) external;
    function executeTransaction(uint256 transactionId) external;
    function revokeConfirmation(uint256 transactionId) external;
    function getConfirmationCount(uint256 transactionId) external view returns (uint256);
    function isConfirmed(uint256 transactionId) external view returns (bool);
}

/**
 * @title ReentrancyGuard
 * @dev Contract module that helps prevent reentrant calls to functions.
 */
abstract contract ReentrancyGuard {
    uint256 private constant _NOT_ENTERED = 1;
    uint256 private constant _ENTERED = 2;
    uint256 private _status;

    constructor() {
        _status = _NOT_ENTERED;
    }

    modifier nonReentrant() {
        require(_status != _ENTERED, "ReentrancyGuard: reentrant call");
        _status = _ENTERED;
        _;
        _status = _NOT_ENTERED;
    }
}

/**
 * @title SafeMath
 * @dev Safe arithmetic operations (Solidity 0.8+ has built-in overflow checks)
 */
library SafeMath {
    function add(uint256 a, uint256 b) internal pure returns (uint256) {
        uint256 c = a + b;
        require(c >= a, "SafeMath: addition overflow");
        return c;
    }

    function sub(uint256 a, uint256 b) internal pure returns (uint256) {
        require(b <= a, "SafeMath: subtraction overflow");
        return a - b;
    }

    function mul(uint256 a, uint256 b) internal pure returns (uint256) {
        if (a == 0) return 0;
        uint256 c = a * b;
        require(c / a == b, "SafeMath: multiplication overflow");
        return c;
    }

    function div(uint256 a, uint256 b) internal pure returns (uint256) {
        require(b > 0, "SafeMath: division by zero");
        return a / b;
    }
}

/**
 * @title Ownable
 * @dev Contract module which provides a basic access control mechanism
 */
abstract contract Ownable {
    address private _owner;

    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);

    constructor() {
        _owner = msg.sender;
        emit OwnershipTransferred(address(0), _owner);
    }

    function owner() public view returns (address) {
        return _owner;
    }

    modifier onlyOwner() {
        require(_owner == msg.sender, "Ownable: caller is not the owner");
        _;
    }

    function renounceOwnership() public virtual onlyOwner {
        emit OwnershipTransferred(_owner, address(0));
        _owner = address(0);
    }

    function transferOwnership(address newOwner) public virtual onlyOwner {
        require(newOwner != address(0), "Ownable: new owner is the zero address");
        emit OwnershipTransferred(_owner, newOwner);
        _owner = newOwner;
    }
}

/**
 * @title Pausable
 * @dev Contract module which allows children to implement an emergency stop mechanism.
 */
abstract contract Pausable {
    event Paused(address account);
    event Unpaused(address account);

    bool private _paused;

    constructor() {
        _paused = false;
    }

    function paused() public view returns (bool) {
        return _paused;
    }

    modifier whenNotPaused() {
        require(!_paused, "Pausable: paused");
        _;
    }

    modifier whenPaused() {
        require(_paused, "Pausable: not paused");
        _;
    }

    function pause() public virtual {
        _paused = true;
        emit Paused(msg.sender);
    }

    function unpause() public virtual {
        _paused = false;
        emit Unpaused(msg.sender);
    }
}
