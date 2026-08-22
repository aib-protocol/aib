# RFC-005: Milestone-Bond ICO — decentralized seedbed for real ventures

- **Status**: DRAFT — open for public discussion
- **Depends on**: RFC-002 (Fee-Burn), RFC-003 (VRF bootstrap), RFC-004 (incubation principles)
- **Version**: 0.1

## 1. Problem

2017-style ICOs let teams raise on whitepapers and walk away. Platforms that "fix" this become gatekeepers. AIB needs the opposite: **capital that can only be unlocked by shipping**, supervised by randomness instead of institutions.

## 2. Mechanism (three on-chain primitives)

### 2.1 Escrow-by-milestone

100% of raised funds (aBTC/aETH/aUSDT/AIB) are locked in plain time+condition locked UTXOs at contribution time. The team never holds the treasury. Funds release in tranches tied to verifiable milestones M1..Mn:

```
M1 shipped  → 20% unlocks
M2 shipped  → 30% unlocks
M3 shipped  → 50% unlocks   (later tranches may grow — big milestones, big money)
```

If a milestone fails or the team vanishes, remaining funds automatically pro-rate back to contributors on-chain. No arbiter can intercept them.

### 2.2 Team tokens are bonds, not gifts

Team allocation unlocks on the *same* milestone schedule as investor funds. Delivery is the only path to liquidity. A fraudster's cost basis becomes "actually building the thing" — negative expected value for scams.

### 2.3 VRF stake-jury supervision

"Was Mi actually delivered?" is decided by a **randomly drawn jury of 21 stakers**, selected per milestone via the chain's existing VRF sortition (same code path as block production, weighted by stake). Jurors stake a bond:

- honest adjudication → share of the project's fees
- provably wrong verdicts → slashing

No committee, no foundation, no platform. Supervision is a lottery duty, like block production itself.

## 3. Why this kills the failure modes

| Attack | Outcome under Milestone-Bond ICO |
|---|---|
| Raise-and-run | Funds never left escrow; run yields nothing |
| Dump on listing | Team liquidity drips at delivery pace; dump pressure ∝ delivery (i.e., earned) |
| Whitepaper scam | Unlock requires shipped, verifiable milestones — scam cost = real dev cost |
| Gatekeeper capture | Jury is re-drawn per milestone by VRF; capturing it = capturing the whole stake |
| Big projects under-funded | Tranches scale with verified delivery; fundraising capacity = f(tract record) |

## 4. L1 cost: two primitives, zero new consensus

1. Conditional locked UTXO (timelock exists today; add condition-hash unlock)
2. VRF jury draw (reuse block-lottery sortition with a different seed domain)

The chain still only ever sees *signed, paid, staked*. An ICO is just a pattern of UTXO locks and a jury signature — no ICO semantics in consensus. Fully consistent with RFC-004 §6 (simple core, complex edges).

## 5. Open questions

1. Jury size 21 — right number? Should it scale with tranche value?
2. Milestone verification: purely juror judgment, or require machine-checkable evidence hashes where possible (CI runs, demo URLs, on-chain metrics)?
3. Tranche proportions (20/30/50) — sensible default or configurable per project?
4. Should contributors be able to exit early (sell their escrow claim) — a secondary market on locked claims?
5. Anti-collusion: juror anonymity vs stake-weighted draw — can a team pre-bribe a future jury?

## 6. Language policy

English canonical; translations community-maintained, non-binding.
