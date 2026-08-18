# AIB DeFi Contract Deployment Tool

## Overview

The AIB DeFi contract deployment tool is a complete deployment and verification solution for deploying and verifying DeFi contracts on the AIB blockchain.

## Features

- ✅ **Complete DeFi contract suite**: WETH, Uniswap V2 Factory, Router
- ✅ **Automated deployment tool**: deployment main program written in Go
- ✅ **Multi-environment support**: devnet, testnet, and mainnet configs
- ✅ **Verification script**: automated contract verification and testing
- ✅ **Detailed docs**: deployment guide and user manual

## Directory Structure

```
./cmd/deploy-contracts/
├── main.go              # Deployment main program (Go)
├── config.yaml          # Config file
├── go.mod              # Go module file
├── networks.json        # Network configs
├── contracts/          # Contract sources
│   ├── WETH.sol        # WETH wrapper contract
│   ├── UniswapV2.sol   # Factory and Pair contracts
│   ├── Router.sol      # Router contract
│   └── AIBTestToken.sol # Test token
├── deploy_genesis.sh   # Genesis initialization script
├── deploy_contracts.sh # Contract deployment script
└── verify_contracts.sh # Verification script
```

## Quick Start

### 1. Prepare Environment

```bash
# Install Go (1.22+)
go version

# Set environment variables
export RPC_ENDPOINT="http://localhost:8545"
export PRIVATE_KEY="0xYOUR_PRIVATE_KEY"
```

### 2. Configure

Edit the `config.yaml` file:

```yaml
rpc_endpoint: "http://localhost:8545"
chain_id: 314159
private_key: "0xYOUR_PRIVATE_KEY"
gas_limit: 8000000
timeout_sec: 300
```

### 3. Deploy

```bash
cd ./cmd/deploy-contracts

# Deploy all contracts
./deploy_contracts.sh --contract all

# Deploy individually
./deploy_contracts.sh --contract weth
./deploy_contracts.sh --contract factory
./deploy_contracts.sh --contract router
```

### 4. Verify

```bash
# Verify deployed contracts
./verify_contracts.sh
```

## Contract Overview

### WETH (Wrapped Ether)
- Wraps ETH as an ERC20 token
- Supports deposit and withdrawal
- Used for interacting with DeFi protocols

### UniswapV2Factory
- Creates and manages token pairs
- Core DEX factory contract
- Supports creation of arbitrary token pairs

### UniswapV2Router
- Main entry point for user interaction
- Provides swaps and liquidity management
- Supports multiple swap paths

## Documentation

- [Deployment verification report](https://www.aib.one:51200/docs/deployment/verification-report.html)
- [User deployment guide](https://www.aib.one:51200/docs/developers/defi-deploy-guide.html)
- [Plan document](https://www.aib.one:51200/plans/deploy-defi-verification.md)

## Tech Stack

- **Deployment tool**: Go 1.22+
- **Contract language**: Solidity 0.8.20+
- **Network protocol**: JSON-RPC
- **Script language**: Bash

## Testing

The tool includes a complete test suite:

- Contract deployment tests
- Functionality verification tests
- Performance tests
- User experience tests

## License

MIT License

## Contributing

Issues and Pull Requests are welcome to improve this tool.
