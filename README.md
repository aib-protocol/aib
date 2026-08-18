<div align="center">

# AIB Protocol  （Drafting and Refactoring Peroid, all not working yet)

### AI-Native Blockchain Protocol

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![GoReportCard](https://goreportcard.com/badge/github.com/aib-protocol/aib)](https://goreportcard.com/report/github.com/aib-protocol/aib)

```
ASCII-Art for AIB here

            Where AI meets Blockchain
```

[Website](https://www.aib.one) | [Documentation](https://docs.aib.one) | [Community](https://community.aib.one)

</div>

---

## Quick Install

```bash
curl -sSfL https://www.aib.one/install.sh | bash
```

## Overview

AIB is a novel blockchain protocol designed from the ground up for the AI era. It combines:

- **PoAIW Consensus**: Proof of AI Work - miners contribute AI compute to earn block rewards
- **UTXO Model**: Extended UTXO with support for smart contracts and AI inference requests
- **Payment Channels**: Lightning-style channels for instant, low-fee AI service payments
- **ZK Rollups**: Scalable layer 2 solutions with Merkle proof-based batch verification
- **AI Inference**: Built-in protocol-level support for distributed AI inference

## Features

### PoAIW (Proof of AI Work)

AIB replaces traditional proof-of-work with AI inference work. Miners run AI models and submit ZK proofs of correct inference:

```
Traditional PoW:    SHA256hash(nonce) < target
AIB PoAIW:          VerifyZKProof(AIModel(input)) == expected_output
```

This turns wasted energy into useful AI computation while maintaining cryptographic security.

### Extended UTXO Model

- **Traditional UTXO**: Bitcoin-style transaction model
- **Extended UTXO**: Supports smart contracts via predicate scripts
- **AI Requests**: Native transaction type for AI inference jobs
- **State Channels**: Off-chain computation with on-chain settlement

### Payment Channels

- **Lightning-style**: Bidirectional payment channels
- **HTLC Support**: Hashed Time-Locked Contracts for atomic cross-chain swaps
- **AI Service Payments**: Micro-payments for AI inference services
- **Dispute Resolution**: On-chain dispute resolution with penalty bonds

### ZK Rollups

- **Merkle Proof Batching**: Batch thousands of transactions into one proof
- **Data Availability**: On-chain data storage with fraud proofs
- **Fast Withdrawals**: One-click withdrawal from rollup to main chain
- **Cross-Rollup**: Bridges between multiple rollup instances

### AI Inference Protocol

```
┌─────────┐    Request    ┌─────────┐    Proof     ┌─────────┐
│ Client  │ ───────────► │  Node   │ ───────────► │  Block  │
└─────────┘              └─────────┘              └─────────┘
     │                        │                         │
     │                        │    Result               │
     └──────────────────────── ◄────────────────────────┘
```

- **Inference Requests**: Native transaction type for AI inference
- **Staking Nodes**: Operators stake AIB to provide inference services
- **Reputation System**: Quality-based reputation scoring
- **Fair Pricing**: Weighted round-robin provider selection

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Application Layer                       │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐  │
│  │  aib-cli│ │aib-miner│ │aib-node │ │aibd CLI│ │  Portal │  │
│  └─────────┘ └─────────┘ └─────────┘ └─────────┘ └─────────┘  │
├─────────────────────────────────────────────────────────────────┤
│                          Service Layer                          │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐  │
│  │Agentic  │ │Inference│ │ Oracle  │ │Channel  │ │  EVM    │  │
│  │ Service │ │  Nodes  │ │         │ │ Manager │ │  Bridge │  │
│  └─────────┘ └─────────┘ └─────────┘ └─────────┘ └─────────┘  │
├─────────────────────────────────────────────────────────────────┤
│                          Core Layer                             │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐  │
│  │  UTXO   │ │  P2P    │ │Consensus│ │  ZK     │ │Crypto   │  │
│  │  Model  │ │ Network │ │  PoAIW  │ │ Rollup  │ │ Primitives│  │
│  └─────────┘ └─────────┘ └─────────┘ └─────────┘ └─────────┘  │
├─────────────────────────────────────────────────────────────────┤
│                       Storage Layer                             │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐                             │
│  │Merkle DB│ │ BoltDB  │ │  Badger │                             │
│  └─────────┘ └─────────┘ └─────────┘                             │
└─────────────────────────────────────────────────────────────────┘
```

## Module Structure

| Module | Description |
|--------|-------------|
| `pkg/utxo` | Extended UTXO transaction model with smart contract support |
| `pkg/channel` | Lightning-style payment channels with HTLC |
| `pkg/p2p` | Pure Go P2P networking (no libp2p dependency) |
| `pkg/agentic` | AI service layer with provider registry |
| `pkg/inference` | AI inference node implementation |
| `pkg/zkrollup` | ZK rollup with Merkle proof batching |
| `pkg/oracle` | On-chain oracle for external data |
| `pkg/evm` | EVM bridge for cross-chain operations |
| `core/crypto` | Cryptographic primitives (Ed25519, Secp256k1, VRF, ZK) |
| `zkml` | Zero-Knowledge Machine Learning proof system |

## Build from Source

### Prerequisites

- **Go 1.24+** - [Install Go](https://go.dev/dl/)
- **Git** - [Install Git](https://git-scm.com/)

### Build Commands

```bash
# Clone the repository
git clone https://github.com/aib-protocol/aib.git
cd aib

# Build all binaries
go build ./cmd/aib-node
go build ./cmd/aib-miner
go build ./cmd/aib-cli
go build ./cmd/aibd

# Or build everything at once
go build ./...

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...
```

### Generated Binaries

| Binary | Description |
|--------|-------------|
| `aib-node` | Full node implementation |
| `aib-miner` | PoAIW miner with AI inference |
| `aib-cli` | Command-line wallet/tool |
| `aibd` | Daemon process for background operation |

## Network Information

### Mainnet

| Parameter | Value |
|-----------|-------|
| Chain ID | `aib-mainnet-1` |
| P2P Port | 31415 |
| RPC Port | 51413 (default) |
| Explorer | https://explorer.aib.one  (not ready yet)

### Testnet

| Parameter | Value |
|-----------|-------|
| Chain ID | `aib-testnet-1` |
| P2P Port | 31414
| Faucet | https://faucet.aib.one |  (not ready yet)

## Tokenomics

### Supply

- **Total Supply**: 3,141,592,653 AIB (π × 10^8)
- **Decimal Places**: 8
- **Ticker**: AIB

### Distribution

| Category | Amount | Percentage |
|----------|--------|------------|
| Mined (PoAIW → PoAT, see RFC-001) | 3,141,582,653 | 100% of circulating supply |
| Founder Bootstrap Grant | 10,000 | 0.0003% |

> **No-premine policy:** the ONLY pre-allocated AIB is a 10,000-unit bootstrap
> grant to the founder for early testnet bootstrapping. There is no team
> allocation, no treasury, no ecosystem fund, no airdrop pool. Every other
> AIB is minted exclusively by mining. Community programs, if ever funded,
> must come from voluntarily donated mined coins — never from genesis.

### Block Reward Schedule (TODO: re-calculated again!)

```
Year 1-4:   3.14 AIB per block (halving at year 4)
Year 5-8:   1.57 AIB per block
Year 9-12:  0.785 AIB per block
...continues with 4-year halving until ~2140
```

## Usage Examples

### Start a Node

```bash
# Initialize the node
aib-node init --moniker "my-validator"

# Start the node
aib-node start

# Check status
aib-cli status
```

### Create an Inference Request

```bash
# Request AI inference
aib-cli inference request \
  --model "llama2" \
  --input "What is the meaning of life?" \
  --payment 1000
```

### Open a Payment Channel

```bash
# Open channel with another node
aib-cli channel open \
  --peer "12D3KooW..." \
  --capacity 100000 \
  --push-amount 50000
```

## Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details.

### Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Code Style

- Follow standard Go formatting (`gofmt`)
- Write tests for new functionality
- Update documentation as needed
- Keep PRs focused and well-described

## Documentation

- [Architecture Overview](https://docs.aib.one/architecture)
- [API Reference](https://docs.aib.one/api)
- [PoAIW Whitepaper](https://docs.aib.one/poaiw)
- [Developer Guide](https://docs.aib.one/developers)

## Community

- **Website**: https://www.aib.one
- **Discord**: https://discord.gg/aib-protocol
- **Twitter**: https://twitter.com/aib_protocol
- **GitHub**: https://github.com/aib-protocol/aib

## Security

For security issues, please email security@aib.one instead of using public issues.

## License

```
MIT License

Copyright (c) 2026 aib-protocol

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

<div align="center">

**Built with passion for the AI + Blockchain future**

[![Stars](https://img.shields.io/github/stars/aib-protocol/aib?style=social)](https://github.com/aib-protocol/aib/stargazers)
[![Forks](https://img.shields.io/github/forks/aib-protocol/aib?style=social)](https://github.com/aib-protocol/aib/network/members)
[![Issues](https://img.shields.io/github/issues/aib-protocol/aib)](https://github.com/aib-protocol/aib/issues)

</div>
