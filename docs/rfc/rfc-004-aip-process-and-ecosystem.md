# RFC-004: AIP Process & Ecosystem Vision

- **Status**: DRAFT — open for public discussion
- **Version**: 0.1

## 1. Improvement proposal naming: AIP

From now on, protocol improvements use the **AIP** (AIB Improvement Proposal) naming, following the BIP/EIP tradition:

| Stage | Name | Meaning |
|-------|------|---------|
| Early design discussion | RFC-00x | Request for Comments — ideas under open attack; anything can change |
| Formal proposal (from mainnet onward) | AIP-NNN | Format: number + status machine (Draft → Review → Active → Final) |

RFC-001..003 are hereby grandfathered as the historical design documents. **AIP-1** (to be published near mainnet) will define the AIP process itself: editors, status machine, and fee-score-weighted voting — governance by contribution, not by committee.

Categories (simple, two only):
- **AIP-Core**: consensus rules, economics, protocol wire format
- **AIP-Service**: off-chain services, tooling, ecosystem standards (DEX, channels, agent APIs)

## 2. Ecosystem vision: real technology runs on AIB

AIB's L1 stays minimal forever (RFC-003 §6: signed, paid, staked — nothing else). Everything valuable grows **on top of it**, settling down to the chain. The intended ecosystem, in build order:

1. **High-speed trading L2** (Hyperliquid-class): on-chain orderbook settlement with off-chain matching; every trade pays φ — the flywheel's biggest engine.
2. **Payment network**: Lightning-style channels for instant micro-payments; the everyday-money layer.
3. **Native AIB embeddability**: wallets, agents, and robots speak the AIB wire protocol natively — no intermediary, no gatekeeper, no human discretion. Anything can hold and move value: a droid on an asteroid included.
4. **Smart-contract extension**: contracts live off-chain (channels, L2, external protocols) and settle on AIB. Turing completeness never enters L1.

## 3. Incubation: real projects, not paper projects

The long-term purpose of this protocol is to host **hard-technology ventures that genuinely run on it** — robotics, deep-space exploration, deep-tech biology, frontier AI, advanced materials, and new energy. Not token launches; **technology launches that happen to settle on AIB**.

Principles for incubated projects:

1. **Build first, tokenize never-or-last**: a project earns ecosystem attention by shipping working technology, not by emitting a token. If it ever mints a token, it is a plain UTXO swap under AIP-Service rules — the chain stays neutral.
2. **Settlement is the only contract**: AIB provides payment, staking, and fee-burn settlement. It never judges a project's merits — communities, users, and markets do, off-chain.
3. **No treasury, no gatekeeper**: there is no foundation fund deciding who gets incubated. Anyone may build. Resources come from participants' own mined coins and voluntary contribution — the same absolute-decentralization rule as genesis.
4. **Milestone-settled funding**: raised funds (aBTC/aETH/aUSDT) can be locked as plain UTXO with time/module locks and released by verifiable milestones — a primitive AIB already has, enough for honest teams, useless for frauds who can't ship.

## 4. Founder role: gardener, not king

The founder holds **zero protocol privilege** — no premine, no veto, no admin key (RFC-002 §2.4). The founder's legitimate role is exactly one thing: **keep the project alive through its fragile early years** — keep building, keep the vision coherent, keep it from being abandoned.

- Early on, that means the founder is miner #1 and lead developer — influence through work, identical rules to everyone.
- As the ecosystem grows, founder influence is naturally diluted by fee-score: anyone who contributes more work earns more voice. That is by design.
- The project's ultimate success is when it no longer needs its founder. Like Bitcoin.

## 5. Open questions

1. Should AIP numbering start at AIP-1 = process spec, or continue from RFC numbering (AIP-004)?
2. Milestone-locked UTXO: is the existing lock-time primitive sufficient, or does an AIP-Service standard for "milestone oracles" make sense (off-chain, of course)?
3. L2 trading engine: reference Hyperliquid's on-chain-liquidity/ off-chain-matching split directly, or design from first principles?
