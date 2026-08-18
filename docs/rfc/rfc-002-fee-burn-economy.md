# RFC-002: The Fee-Burn Economy — Pure Fee-Funded Staking & Self-Sustaining Consensus

- **Status**: DRAFT — open for public discussion
- **Discussion**: [Issue #1](https://github.com/aib-protocol/aib/issues/1)
- **Supersedes**: RFC-001 §4 (block producer selection) and §6 (parameters)
- **Version**: 0.2

## 1. One line

**The AIB chain is a settlement layer for AI-inference trades plus a fee-burn engine. No premine, no fixed emission: the staking APR is set entirely by fees from real inference throughput — the market prices it, not a whitepaper.**

## 2. Mechanism

1. Every on-chain settled inference transaction pays a fee φ (proposed 1%) into the current epoch reward pool.
2. Staking APR: `APR = φ·T / S` (T = epoch transaction volume, S = total stake — floating, market-discovered).
3. Block production: pure stake-weighted VRF. **The transaction-volume weight from RFC-001 §4 is removed** — washing volume grants zero block-selection advantage.
4. Founder privilege: **veto only**. No mint key, no allocation key, no unilateral upgrade power.

## 3. Why cheating is strictly unprofitable

| Attack | Outcome |
|--------|---------|
| Wash trading | Net loss = φ·v·(1−s) > 0 — strictly dominated strategy |
| Proposer stuffing fake txs | Fake txs still require real payment + fee burn — always negative |
| Miner/provider receipt collusion | Forgery cost = real cost; no arbitrage exists |
| Buy 51% of stake | Self-immolating: chain dies → fee pool dies → attacker's own stake → 0 |
| Suppress APR then attack | Negative feedback restores S automatically (S↓ → APR↑ → new stakers enter) |

The security budget is not electricity (PoW) verification or fixed inflation (PoS) — it is **the real fee cash-flow of the AI economy the chain settles**. Attacks destroy their own funding source.

## 3.1 No verification computation

None of this requires re-running the inference (no verification computation). The chain never verifies the AI output itself; it verifies that **signed parties paid real money and burned fees**. This keeps the protocol service-agnostic for a century: AI today, HI or whatever comes next tomorrow — the settlement logic is identical.

## 4. Long-term self-evolution (founder-decentralization roadmap)

- **Phase 0 (now)**: founder holds veto; community discussion via issues/PRs; RFC governance process established.
- **Phase 1**: veto scope narrowed in writing — only proposals altering monetary policy or breaking the no-premine/no-fixed-emission invariant can be vetoed.
- **Phase 1.5**: veto use is rate-limited and publicly logged; every veto must cite which invariant it protects.

- **Phase 2**: veto becomes *delayed veto* (delay + arbitration instead of kill); routine upgrades move to on-chain voting.
- **Phase 3**: veto sunsets automatically (e.g., after N consecutive quarters of voter participation above threshold). Protocol fully self-governing.
- **Adaptive parameters**: φ, epoch length, etc. adjust only via fixed governance formulas (e.g., an APR target band) — *rules amend rules; humans do not hand-pick numbers*.

## 5. Century / interplanetary scale

The protocol layer does not know what "AI" is. It knows exactly three things: **signed transactions, fees, stake**. Whatever the service becomes (AI → HI → whatever follows), settlement logic needs zero changes.

Latency-tolerant design:

- Block production and settlement are local (per-planet); only checkpoints (light finality proofs) cross the interplanetary gap.
- Challenge windows are parameterized by physical latency (Mars: 4–24 min one-way → windows auto-scaled by measured transit time).

## 6. Open questions

1. Should the fee rate φ be fixed at 1%, or dynamic (an EIP-1559-style fee market)?
2. Reward distribution: per-2block (daily) or per-block (continuous)?
3. Veto sunset condition: participation threshold, time-based, or both?
4. Interplanetary checkpoints: light-client proofs or a high-stake committee?
5. What belongs in the untouchable invariant set? (no-premine, fixed supply π×10⁸, fee-only rewards … what else?)

## 7. Language policy

English is the canonical language of this repository: code, comments, issues, RFCs, and discussions. Translations are community-maintained and non-binding.
