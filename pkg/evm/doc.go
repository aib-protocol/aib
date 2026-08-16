// Package evm provides EVM compatibility testing for the AIB blockchain.
//
// This package contains comprehensive tests for:
// - Core EVM functionality (address, hash, state management)
// - Precompiled contracts (ecrecover, SHA256, RIPEMD160, etc.)
// - DeFi scenarios (ERC20, ERC721, Uniswap V2, Flashloans, Oracles)
// - Security (reentrancy, overflow, access control, gas optimization)
//
// Test Data
//
// Test contracts and vectors are stored in /testdata/evm/:
//   - Interfaces.sol: Solidity interfaces for DeFi contracts
//   - testvectors.json: JSON test vectors for various operations
//
// Running Tests
//
//   go test -v ./pkg/evm/...
//
// With coverage:
//   go test -v -cover ./pkg/evm/...
//   go test -v -coverprofile=coverage.out ./pkg/evm/...
//   go tool cover -html=coverage.out
//
// Benchmarks:
//   go test -bench=. ./pkg/evm/...
//
// Test Organization
//
// Tests are organized by functionality:
//   - evm_test.go: Core EVM functionality tests
//   - precompiled_test.go: Precompiled contract tests
//   - defi_test.go: DeFi scenario tests
//   - security_test.go: Security vulnerability tests
package evm
