# EVM Compatibility Test Suite

## Overview

A comprehensive EVM compatibility test suite ensuring that DeFi applications can be deployed and run.

## File Structure

```
./pkg/evm/
├── evm_test.go          # Core functionality tests
├── precompiled_test.go  # Precompiled contract tests
├── defi_test.go         # DeFi scenario tests
├── security_test.go     # Security tests
├── doc.go               # Package documentation
├── executor.go          # EVM executor
├── state.go             # State management
├── gas.go               # Gas calculation
├── precompiled.go       # Precompiled contracts
└── journal.go           # State journal

./testdata/evm/
├── Interfaces.sol       # Solidity interface definitions
└── testvectors.json     # Test vectors

./scripts/
└── test-evm-coverage.sh # Test coverage script
```

## Test Scope

### 1. Core Functionality Tests (evm_test.go)

- Address type conversion
- Hashing (Keccak256)
- State management
- Transaction execution
- Contract deployment
- Gas calculation
- EVM opcode tests
- Edge case handling

### 2. Precompiled Contract Tests (precompiled_test.go)

- **ecrecover (0x01)**: ECDSA signature recovery
- **SHA256 (0x02)**: SHA-256 hash
- **RIPEMD160 (0x03)**: RIPEMD-160 hash
- **Identity (0x04)**: data copy
- **Modexp (0x05)**: modular exponentiation
- **BN128Add (0x06)**: elliptic curve point addition
- **BN128Mul (0x07)**: elliptic curve point multiplication
- **BN128Pairing (0x08)**: elliptic curve pairing
- **Blake2F (0x09)**: Blake2 compression function

### 3. DeFi Scenario Tests (defi_test.go)

#### ERC20 Standard
- `transfer` - token transfer
- `approve` - approval allowance
- `transferFrom` - transfer from approved account
- `allowance` - query approval allowance
- `totalSupply` - query total supply
- `balanceOf` - query balance

#### ERC721 Standard
- `mint` - mint NFT
- `transfer` - transfer NFT
- `approve` - approve NFT
- `ownerOf` - query owner

#### Uniswap V2 Style Trading
- Token swap
- Add/remove liquidity
- Slippage protection

#### Flashloan
- Borrow and repay
- Fee calculation
- Failure rollback

#### Price Oracle
- Price updates
- Stale price detection
- Access control

### 4. Security Tests (security_test.go)

- **Reentrancy protection**: test reentrancy guard pattern
- **Integer overflow protection**: SafeMath operation tests
- **Access control**: role permission tests
- **Gas optimization**: gas cost analysis
- **DoS protection**: external call limits
- **Input validation**: boundary checks
- **State invariants**: state consistency verification

## Running Tests

### Basic Tests
```bash
cd .
go test -v ./pkg/aal/...
```

### Coverage Tests
```bash
go test -v -cover ./pkg/aal/...
go test -v -coverprofile=coverage.out ./pkg/aal/...
go tool cover -html=coverage.out
```

### Benchmark Tests
```bash
go test -bench=. -benchmem ./pkg/aal/...
```

### Using the Coverage Script
```bash
bash ./scripts/test-evm-coverage.sh
```

## Test Coverage Target

- **Target**: 80% or above
- **Current**: see coverage report

## Performance Benchmarks

Performance benchmarks for key operations:

- `BenchmarkKeccak256`: Keccak256 hash computation
- `BenchmarkStateGetBalance`: balance query
- `BenchmarkStateSetBalance`: balance update
- `BenchmarkAddressConversion`: address conversion
- `BenchmarkERC20Transfer`: ERC20 transfer
- `BenchmarkUniswapSwap`: Uniswap swap
- `BenchmarkFlashloan`: flash loan

## Test Data

### Solidity Interfaces (`Interfaces.sol`)

Contains complete DeFi interface definitions:
- ERC20/ERC721 standards
- Uniswap V2 Factory/Pair/Router
- Flashloan receiver
- Lending pool interface
- Price oracle interface
- Governance interface
- Staking interface
- Multisig wallet interface

### Test Vectors (`testvectors.json`)

Standardized test inputs and expected outputs.

## Contributing

1. When adding new tests, ensure coverage stays above 80%
2. Use descriptive test names
3. Add comments for complex tests
4. Run all tests and benchmarks before submitting

## Related Documentation

- [EVM Developer Guide](https://www.aib.one:51200/docs/evm-dev-guide.html)
- [DeFi Deployment Tutorial](https://www.aib.one:51200/docs/defi-deployment.html)
- [Security Documentation](https://www.aib.one:51200/docs/security.html)
