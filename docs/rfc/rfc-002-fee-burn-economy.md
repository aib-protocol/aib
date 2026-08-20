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
4. **No founder privilege. Zero.** There is no veto, no admin key, no special role of any kind. Governance weight is earned on-chain: the voice that matters belongs to the highest cumulative **fee-score** — the address that has contributed the most burned-fee work over the longest time. Contribution-weighted, time-accrued, fully decentralized.

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

## 4. Long-term self-evolution (no-founder governance)

There is no founder role and no sunset roadmap for one — centralization was never introduced in the first place.

- **Governance weight = cumulative fee-score.** Every address accrues score from burned fees it has caused over time (work × duration). The largest contributors — those who have walked the farthest and paid the most real cost on-chain — hold the strongest voice. Buying it late costs real burned fees, not a token grant.
- **Rules amend rules.** φ, epoch length, and all tunable parameters adjust only via fixed governance formulas (e.g., an APR target band) executed by the fee-score-weighted on-chain vote — *humans do not hand-pick numbers; the chain's own work-weight decides*.
- **Attack-resistant by construction**: to capture governance you must out-contribute every honest participant cumulatively — an attack that pays real fees funds the very reward pool it seeks to drain (strictly negative EV, same table as §3).
- Community discussion via issues/PRs; RFC governance process is the only off-chain artifact, and it is advisory, never authoritative.

## 5. Century / interplanetary scale

The protocol layer does not know what "AI" is. It knows exactly three things: **signed transactions, fees, stake**. Whatever the service becomes (AI → HI → whatever follows), settlement logic needs zero changes.

Latency-tolerant design:

- Block production and settlement are local (per-planet); only checkpoints (light finality proofs) cross the interplanetary gap.
- Challenge windows are parameterized by physical latency (Mars: 4–24 min one-way → windows auto-scaled by measured transit time).

## 6. Open questions

1. Fee rate φ: fixed at 1%, or **dynamic**? A PoW chain adjusts difficulty to hashpower; a fee-burn chain has no hashpower — but it has an equivalent: **fee throughput vs. stake (T/S)**. Proposal on the table: φ adapts like a difficulty adjustment, steering `APR = φ·T/S` toward a target band (e.g., 4–8%): if real usage pushes APR above the band, φ eases down; below the band, φ rises. Usage is the mining energy of this chain; φ becomes its difficulty knob — market-set, not human-set.
2. Reward distribution: per-epoch (daily) or per-block (continuous)? *(epoch = 1 day is the working proposal)*
3. Interplanetary checkpoints: light-client proofs or a high-stake committee?
4. What belongs in the untouchable invariant set? (no-premine, fixed supply π×10⁸, fee-only rewards, **no founder/admin keys** … what else?)

## 7. Language policy

English is the canonical language of this repository: code, comments, issues, RFCs, and discussions. Translations are community-maintained and non-binding.
