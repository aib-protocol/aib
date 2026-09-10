# RFC-006: Storage Layer & Git-on-AIB — AI-native permanence, paid in AIB

- **Status**: DRAFT — open for public discussion
- **Depends on**: RFC-002 (fee-burn economy), RFC-004 (ecosystem layering)
- **Version**: 0.1

## 1. Why storage, and why now

AIB settles AI-inference trades (RFC-002). But an AI economy needs **durable state**: models, datasets, agent memories, audit trails. Today these live on rented clouds — custodial, censorable, deletable. RFC-004's rule says applications may centralize business but must decentralize *settlement*; for AI state, **storage is settlement**. A model that can vanish is not settled.

Storage also answers the protocol's harshest question — *"what does the chain actually guarantee long-term?"* — with something concrete: **AIB becomes the economic layer that keeps useful bytes alive.**

## 2. Design constraints (inherited from RFC-002/004)

1. **L1 stays minimal**: signed, paid, staked — nothing else. No file bytes, no chunk addressing, no contract execution enters L1.
2. **Economy must be self-funding**: like staking APR (φ·T/S), storage persistence must be paid by *users of storage*, not by inflation. No premine treasury subsidizing disk.
3. **Anti-Sybil by real cost**: announcing storage you don't hold must be strictly unprofitable (same table logic as RFC-002 §3).

## 3. The storage layer: paid permanence (AIP-Service)

### 3.1 Model — storage contracts as plain UTXO flows

A storage "contract" is three standard primitives glued together, no new consensus rules:

```
Client ── escrow UTXO (streamed rent) ──► Contract
Provider ── stake UTXO (collateral)  ──► Contract
              │
              ▼  periodic proof round
Provider posts proof → rent tranche releases + fee φ burns → score accrues
Provider fails proof → collateral slashes, client refund stream
```

- **Rent, not sale**: clients pay per epoch (e.g. monthly) for a bundle (content-hash list). Continuous payment = continuous verification the bytes still exist. If nobody pays, data gracefully expires — the chain never pretends to store what nobody funds. Honest failure, no false promises.
- **Provider collateral**: storage providers stake AIB per contract. Missing a proof round slashes the stake — renters are insured by collateral, not by goodwill.
- **Fee-burn integration**: each rent payment burns φ (1%) into the epoch pool — storage becomes another flywheel engine feeding staking APR, exactly like inference trades. **AIB's economic model drives long-term storage: more stored bytes → more rent flows → more fees burned → stronger staking APR → more validators → safer chain → more storage. Self-reinforcing.**

### 3.2 Proofs — challenge-response, no new cryptography needed

- Content-addressed chunks (Merkle chunking, standard). Contract records the Merkle root.
- Each proof round the chain (deterministically from VRF output, reusing the existing VRF primitive) picks random chunk indices; provider answers with Merkle branches.
- Verification is **off-chain by clients + light auditors** (AIP-Service); only proof *commitments* and slash/refund settlements touch L1. Fits RFC-004 §3.5: centralize the business (audit services), decentralize the settlement.
- Erasure coding across N providers (k-of-n) for redundancy; rent splits across providers, collateral pooled.

### 3.3 What gets stored first (demand side)

Priority order — same "build what the AI economy already needs" rule:

1. **AI artifacts**: model weights, dataset snapshots, inference receipts (audit trails). The chain already settles inference trades (RFC-002); storing their artifacts is the natural adjacency.
2. **Agent memory & tool registries**: agent state that must outlive any single vendor.
3. **Public datasets** (funded by milestone-bonds, RFC-005): a dataset's upkeep is a recurring milestone — rent paid from bond tranches.
4. **Arbitrary blobs** last, once pricing has proven stable.

## 4. Git-on-AIB: version control as the first killer app

### 4.1 Why Git

- Git repos are **append-mostly, content-addressed, Merkle-structured** — they are *already* shaped like storage-layer objects (`blob`/`tree`/`commit` hashes chain exactly like chunk roots). Mapping is nearly isomorphic; no new data model needed.
- **Every project already has this problem**: GitHub is a single company; a repo ban / DMCA / bankruptcy erases history. "Important code lives on one rented shelf" contradicts everything RFC-004 stands for.
- Version control gives storage a *visible, daily-use consumer app* — the wedge that makes "AIB storage" legible to normal developers, not just AI pipelines.

### 4.2 Mechanism — Git remote protocol over the storage layer

```
git push aib::myrepo
   │
   ▼
aib-remote (adapter, AIP-Service standard)
   │  translates git packfiles ↔ storage chunks
   ▼
Storage contract: "repo bundle @ merkle-root, rent paid to epoch N"
```

- **Push = rent escrow**: `git push` computes the new packfile chunks, negotiates a rent contract (or extends the repo's standing one), pays from wallet. Code persistence becomes a *gas-like microservice*: one command, one payment, permanent until unpaid.
- **Clone/fetch = plain read**: storage layer serves chunks; adapter rebuilds packfiles. Reads are free at protocol level (providers may charge for bandwidth as their business, per §3.5 centralize-business rule).
- **No chain-side understanding of Git**: L1 sees only storage contracts (hash lists + rent). The git adapter is an AIP-Service standard — anyone can implement a remote, anyone can run a mirror. The chain guarantees the *bytes and the payment flow*; the tooling ecosystem competes on UX.
- **Fork = insurance**: mirrors are cheap (reads are free); GitHub/GitLab mirrors can coexist — AIB is the neutral floor under all of them, not a GitHub replacement.

### 4.3 Project management on top

Once repos are permanent + paid-for, project management is composable on the same primitives:

- **Issues/PRs as append-only logs**: same storage contracts, same rent model; identity = wallet keys (fee-score-weighted governance extends naturally — RFC-004 §4).
- **Milestone-bond repos (RFC-005 tie-in)**: a venture's repo itself can be milestone-locked — bond tranches release *and* prepay the repo's storage rent. Shipping code and funding code live under one audit trail.
- **CI/run environments**: recorded artifacts (logs, binaries, SBOMs) are just more storage bundles with their own rent payers — supply-chain audit trails that cannot be quietly rewritten.

## 5. AI-native protocol: the unified picture

AIB's claim sharpens from "settle AI trades" to: **the protocol where an AI economy keeps its state, its money, and its provenance on one neutral ledger.**

| Layer | Primitive | Payment |
|---|---|---|
| Money/settlement | L1 UTXO + fee-burn | φ per tx |
| Compute (inference) | signed receipts (RFC-002) | fee per trade |
| **State (this RFC)** | **storage contracts + rent** | **per-epoch rent, φ burned** |
| Collaboration | git remotes, issue logs | storage rent + tx fees |
| Ventures | milestone bonds (RFC-005) | tranche unlocks |
| Governance | fee-score weighting | burned work |

An AI agent born on this stack natively holds: a wallet (L1), pays inference (RFC-002), persists its memory (§3), versions its own source (§4), and raises milestone funding (RFC-005) — **without ever depending on a custodial intermediary**. That is the complete autonomy loop, and each link pays fees into the same engine.

## 6. Honest risks / open questions (attacked before built)

1. **Rent price discovery**: who sets AIB/GB/epoch? Proposal: free market per contract, chain only burns φ — no oracle on L1. Cold-start pricing bootstrap is an open question.
2. **Proof-gaming**: provider could store only challenged chunks (probing the VRF). Mitigation: high-frequency random spot checks + full-replica audits by paid auditor services (business layer). Residual risk accepted and disclosed.
3. **Data expiry vs. permanence marketing**: graceful expiry is *honest* but could orphan unmaintained history. Mitigation: public-good rent pools (donation-funded, AIP-Service) for culturally critical repos; the chain does not socialize disk costs by inflation.
4. **Chunk-size/latency tradeoffs for interactive git use**: clone of a huge monorepo needs bandwidth business (paid CDN mirrors) — protocol stays neutral.
5. **Priority risk**: storage is a second flywheel; shipping it before the trading L2 (RFC-004 §2 order) could split focus. Counter-argument: git-storage has clearer immediate demand from developers; sequencing is a genuine open question — which comes first, the exchange or the repository?
6. **Naming**: "Git-on-AIB" tool name, AIP-Service standard number, and whether repos count toward fee-score (proposal: yes — rent burns are fees) — all open.
