# RFC-003: Bootstrap Window & Liquidity Flywheel

- **Status**: DRAFT — open for public discussion
- **Discussion**: to be opened as a GitHub issue
- **Depends on**: RFC-002 (Fee-Burn Economy)
- **Version**: 0.1

## 1. One line

**The first K blocks are minted by presence, not by stake — then the chain switches itself to pure Fee-Burn mode, and external liquidity (BTC/ETH/USDT) flows in by yield, never by force.**

## 2. The cold-start deadlock, and why only randomness solves it

Pure stake-weighted VRF has a chicken-and-egg problem at genesis:

```
block rights ∝ stake → nobody has coins (absolute no-premine)
→ nobody can stake → nobody can produce blocks → deadlock
```

Every historical fix is centralizing: premine, ICO, sale. We reject all of them. The only fair alternative: **a bounded window where block production rights are assigned by pure VRF lottery among online nodes — one ticket per node, zero weight, zero stake, zero permission**.

## 3. Bootstrap window mechanics

1. Blocks 1..K: every registered online node participates in a VRF lottery with **equal weight** (no stake, no hashrate, no permission to ask for).
2. The lottery winner produces the block and receives the coinbase reward.
3. Coinbase during the window is deliberately **low** (proposed 1 AIB/block) — the coins are not yet worth attacking for; the window's purpose is distribution, not enrichment.
4. **Auto-stake**: the coinbase is automatically recorded as stake for the producer (no action needed, no lock, no expiry — the holder can unstake anytime after the standard unbonding period).
5. No lockups, no vesting cliffs, no expiring coins, no dual-token classes. Idle coins earn nothing and dilute relative to active stakers — time itself rewards activity, no rule needed.

| Parameter | Proposal | Rationale |
|-----------|----------|-----------|
| K (window length) | 10,000 blocks (~7 days at 60s/block) | long enough for fair distribution; short enough that stake-weighted security arrives quickly |
| Window coinbase | 1 AIB/block | total ≈ 0.0003% of supply — too small to farm with sybils, too visible to hide |
| Post-window | automatic switch to RFC-002 stake-weighted VRF | no human decision involved in the transition |
| Unbonding | 7 days (global constant, applies to all staking) | flash dumps get a 7-day on-chain warning; industry standard |

## 4. Why this cannot be gamed

- **Sybil nodes**: a million fake nodes split the same 1 AIB/block pie; each still must actually produce valid blocks (real work) or forfeit. Expected value per fake identity ≈ 0.
- **Missed lottery (offline)**: no penalty — you simply weren't in the draw. Come back online.
- **Founder advantage**: none. The founder runs the same binary under the same rules. Miner #1 is a role, not a privilege — exactly the Satoshi position, but with *equal* per-node odds instead of hashrate dominance.

## 5. Liquidity flywheel (why external value flows in voluntarily)

The Fee-Burn economy makes **liquidity itself the mining energy**:

```
trade volume T ↑ → fee pool φ·T ↑ → staking APR ↑
→ demand for AIB (to stake) ↑ → liquidity arrives
→ more trades settled → T ↑  (loop)
```

Design commitments that keep this honest:

1. **Yield comes only from real fees** — never from inflation. If nobody trades, APR is honestly zero; no fake yield to attract mercenary capital.
2. **Asset anchoring via burn-and-mint with light-client verification** (BTC/ETH/USDT, planned): anchoring is trustless (no multisig custodian), 1:1 redeemable, and the anchored asset's trading pays φ into the same fee pool. BTC holders don't "migrate" — their BTC *visits* AIB to earn, and can always leave. Trapped value is a bug; flowing value is the feature.
3. **φ is dynamic** (RFC-002 §6.1): steers APR = φ·T/S into a target band — usage is the difficulty knob; the market sets the rate, not a committee.

## 6. Simple core, complex edges

The L1 will never gain a Turing-complete VM. Smart contracts, AI agents, DeFi, ICOs — all live **outside** the chain (payment channels, L2s, external protocols) and settle onto AIB. The chain's verdict is always the same three words: signed, paid, staked. An exploit destroys one channel; it can never reach the consensus layer. (This is the explicit lesson from Ethereum's on-chain-complexity hacks: The DAO, Parity freeze, etc.)

On-chain ICOs of third-party projects are possible as plain UTXO swaps — the chain settles the trade, passes no judgment, and assumes no liability. Entrepreneurship happens *on* AIB, not *inside* AIB.

## 7. Century / interplanetary

Unchanged from RFC-002 §5: the protocol is service-agnostic (settles any signed fee-paying trade, AI today, anything tomorrow) and latency-tolerant (local block production + checkpoints). A chain this simple can run on Mars.

## 8. Open questions

1. K = 10,000 blocks — right size? Should the window instead end when unique-staker count exceeds a threshold?
2. Window coinbase 1 AIB/block — too high/low? (Total window payout = 0.0003% of supply.)
3. Should the window have a minimal liveness proof (node must have been online N of the last M blocks) to strengthen the "presence" claim?
4. Anchoring: light-client only, or allow a phased rollout with rate-limited caps?
5. Unbonding 7 days — standard, but should large stakes have longer notice?

## 9. Language policy

English is canonical; translations are community-maintained and non-binding.
