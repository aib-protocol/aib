<div align="center">

# AIB Protocol  （Drafting and Refactoring Peroid, all not working yet)

### AI-Native Blockchain Protocol

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![GoReportCard](https://goreportcard.com/badge/github.com/aib-protocol/aib)](https://goreportcard.com/report/github.com/aib-protocol/aib)

```
 █████╗ ██╗██████╗       ██████╗ ██████╗  ██████╗ ████████╗ ██████╗  ██████╗ ██████╗ ██╗
██╔══██╗██║██╔══██╗      ██╔══██╗██╔══██╗██╔═══██╗╚══██╔══╝██╔═══██╗██╔════╝██╔═══██╗██║
███████║██║██████╔╝█████╗██████╔╝██████╔╝██║   ██║   ██║   ██║   ██║██║     ██║   ██║██║
██╔══██║██║██╔══██╗╚════╝██╔═══╝ ██╔══██╗██║   ██║   ██║   ██║   ██║██║     ██║   ██║██║
██║  ██║██║██████╔╝      ██║     ██║  ██║╚██████╔╝   ██║   ╚██████╔╝╚██████╗╚██████╔╝███████╗
╚═╝  ╚═╝╚═╝╚═════╝       ╚═╝     ╚═╝  ╚═╝ ╚═════╝    ╚═╝    ╚═════╝  ╚═════╝ ╚═════╝ ╚══════╝

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

**AIB is a settlement layer that charges a transparent fee for the flow of value — decentralization removes the middleman, and the protocol collects the middleman's fee as public security budget.**

Bitcoin settles value at rest. AIB settles value **in motion**: every on-chain trade pays a protocol fee φ that funds the network's security, so liquidity itself is the mining energy. The chain knows exactly three things — **signed transactions, fees, stake** — and nothing else:

- **Fee-Burn Economy (RFC-002)**: no premine, absolute. Staking APR = φ·T/S — set by real transaction flow, market-discovered, never by whitepaper promise
- **Simple core, complex edges**: like Bitcoin, the consensus layer stays minimal forever (UTXO + VRF + fee). All complexity — AI inference, contracts, DeFi — lives *outside* the chain and settles onto it. An exploit burns one channel, never the chain
- **UTXO Model**: extended UTXO with payment channels and AI-inference settlement as first-class citizens
- **Trustless asset anchoring (planned)**: BTC/ETH/USDT anchor in via light-client burn-and-mint (no multisig custodian), trade at AIB speed, redeem 1:1 anytime — value flows in for the yield, never trapped
- **Native AI inference settlement**: providers and users settle inference trades on-chain; the chain verifies signatures and fees, never the AI output itself — service-agnostic for a century

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
| Mined (fee-burn economy, see RFC-002) | 3,141,592,653 | 100% |

> **No-premine policy: absolute.** The genesis block allocates **zero** AIB to
> anyone — no founder grant, no team, no treasury, no ecosystem fund, no
> airdrop pool. Every AIB in existence is minted exclusively by mining. The
> founder is simply miner #1: like Bitcoin's Satoshi, any early coins are
> earned by running a node and producing blocks from block 1 — the same
> opportunity every participant has at every moment. Community programs, if
> ever funded, must come from voluntarily donated mined coins — never from
> genesis.

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
