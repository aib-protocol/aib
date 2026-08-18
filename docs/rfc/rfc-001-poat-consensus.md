# RFC-001: Proof of AI Token Transactions (PoAT)

- **Status**: SUPERSEDED in part by [RFC-002](rfc-002-fee-burn-economy.md) (block producer selection & parameters). Mining model in §2–3 still current.
- **Version**: 0.2

## 1. Motivation

The original PoAIW (Proof of AI Work) consensus has fundamental flaws:

1. **Fake computation**: Miners can replay trivial inference jobs just to mine blocks. The chain cannot distinguish meaningful inference from waste.
2. **Frictionless self-dealing**: A miner can send inference requests to itself at zero cost, inflating its measured "AI work" and monopolizing block production.
3. **Unanchored staking rewards**: Staking rewards originally came from a pre-allocated pool with no tie to real economic activity.

## 2. Proposal: PoAT (Proof of AI Token Transactions)

**Mining = real AI token consumption.**

Miners connect to *any* inference backend — cloud APIs, local models (Ollama/vLLM), or custom gateways — and mine by **spending tokens on real inference requests**:

```
Miner ──inference request (spends tokens)──► AI Provider (any backend)
  ▲                                            │
  │                                            ▼
  └────────────signed receipt + result─────────┘
```

Every AI transaction is recorded on-chain with the request digest, token usage, and a provider-signed receipt.

## 3. Anti-Fraud: Mandatory Fee Rotation

Every AI transaction pays an on-chain fee into the staking reward pool:

```
each AI tx: amount × (1 − fee_rate) → remainder goes to staking pool
```

- Staking rewards are funded **exclusively** by real AI economic activity — no longer minted from thin air.
- **Self-dealing has real cost**: a miner trading with itself still pays the fee. Grinding cost = fee_rate × volume, making it profitable only when expected block reward exceeds the burn — a tunable economic equilibrium, exactly like electricity in PoW.

## 4. Block Producer Selection — SUPERSEDED

> **Deprecated by RFC-002 §2.3.** The score-weighted selection described here
> (stake^α × feeVolume^β) allowed wash-trading to buy block-selection
> probability. RFC-002 replaces it with pure stake-weighted VRF selection,
> where transaction volume grants zero block-selection advantage. The
> heaviest-chain rule is redefined as maximum cumulative fee burn.

Original text (kept for the record): proposers were chosen by
`score(i) = stake(i)^0.3 × feeVolume(i)^0.7` with score-weighted VRF; fork
choice by cumulative transaction score.

## 5. Incentives

| Role | Income | Constraint |
|------|--------|-----------|
| AI Miner | block reward + user fees | must really spend tokens (fee burn) |
| Staker | pro-rata share of fee pool | bonded stake secures the chain |
| AI Provider | inference fees from miners | signed receipts, on-chain reputation |

## 6. Migration (testnet)

1. `AITransaction` type + provider receipt verification
2. Fee rotation + staking pool rewrite
3. Proposer selection → pure stake-weighted VRF (per RFC-002)
4. Testnet genesis reset, public beta, parameter calibration
4. Whitepaper / docs update, mainnet decision

## 7. Security Considerations

- **Receipt forgery** (miner-provider collusion): fees still apply, so forgery is economically equivalent to self-dealing — always net-negative.
- **Window manipulation** (burst volume): moot under RFC-002 selection (no volume weight).
- **Long-range attacks**: reuse existing PoS checkpoint tooling.

## 8. Language policy

English is the canonical language of this repository. Translations are community-maintained and non-binding.
