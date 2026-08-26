# Changelog

All notable changes to the AIB Protocol node software. Format: Keep a Changelog / semver-ish, testnet suffix until mainnet.


## [v0.11.5-testnet] — 2026-08-26

### Fixed — chain catch-up no longer deadlocks on historical timestamps
- Merge of PR #4 (issue #3). `Block.ValidateBlockChain` and `ChainState.validateBlockTimestamp` used to enforce `MaxBlockTimeDrift = 5m` against the **wall clock** unconditionally, even while catching up. A node that fell behind (clock skew, downtime, slow sync) would reject the first historical block and stall forever at the catch-up boundary. Tip freshness is still enforced; only the unconditional future-bound during sync was lifted. New tests in `security_audit_test.go` cover the boundary. **Resolution of the "stuck at height 100" / "block time exceeds drift" symptom observed on remote test nodes.**

## [v0.11.3-testnet] — 2026-08-25

### Added — Smart setup wizard (`cmd/aib-node/setup.go`)
- **Existing-wallet detection**: if `node_key.pem` already exists in the data dir, the wizard keeps it, skips wallet creation, and prints the current address instead of offering to create a new wallet (previously a careless `Y` overwrote into a fresh, empty wallet).
- **PoW-era mining check**: wizard queries `/v1/block/latest`; if height > 1000 (PoW era over) it auto-skips the "Start CPU mining now?" question and points the user to staking (`POST /v1/stake`) instead. The mining prompt only appears for genuinely new networks still inside the PoW era.

## [v0.11.2-testnet] — 2026-08-25

### Security
- **API binds 127.0.0.1 by default** (`-api-bind`, default loopback). Signing endpoints (`/v1/stake`, `/v1/unstake`, `/v1/wallet/send`) accept private keys in the request body — they must never be exposed to the internet. Verified: direct external connection to the seed's API port now fails; `aib.one` continues to serve via the on-host reverse proxy. `0.0.0.0` still available via explicit `-api-bind 0.0.0.0`.
- **CORS whitelist**: the API no longer sends `Access-Control-Allow-Origin: *`; only `https://aib.one` is whitelisted. Requests without an Origin header (curl, agents) are unaffected.
- **Bootstrap exception narrowed**: the empty-validator-set proposer exception now applies **only to block 1001** (the PoW→PoS transition block). Previously any empty-set block was accepted, which the security test suite correctly flagged (TestAttack_InvalidProposer green again).

### Added — Live-network APIs
- `GET /v1/distribution` — every holder's liquid/staked/total AIB (pie-chart data source).
- `GET /v1/stake/validators` — current validator set: address, staked amount, weight %, blocks produced over the last 50 blocks.
- `GET /v1/stake/info/{addr}` (from v0.11.2 line) — liquid vs staked split per address.

### Fixed — consensus & wallet
- **Coinbase uniqueness**: PoW coinbase transactions now embed the block height in their data field. Previously all 1000 coinbase txs were byte-identical → identical tx hashes → UTXO set entries overwrote each other, leaving only the LAST block's 31.415 AIB spendable out of 31,415 total. Verified post-fix: UTXO count == block height exactly.
- **API server mempool wiring**: `SetMempool` was never called on the API server, so `/v1/stake` (and any tx submission) failed with "mempool not available".
- **PoS blocks set `Header.Version = 3`**: they were being minted as V2 and then rejected by V3 rules ("first transaction is not coinbase"). PoS-era blocks carry stake/transfer transactions with no coinbase (fixed supply).
- **PoS `Header.Proposer` = wallet address** (SHA256 of pubkey), matching what validator-set verification compares. The pubkey itself moved to `Header.ProposerKey` for signature verification.
- **wallet/info reads the wallet address** (not the raw pubkey) — balances showed 0 for wallets that held funds.

### Consensus model (locked 2026-08-25, user decision)
- **True-stake PoS**: validator weight comes exclusively from live on-chain STAKE UTXOs (script tag `0xA1`, UNSTAKE with 500-block cooldown). No PoW-derived weight, no self-registration (`-validator` self-mint of 10000 AIB weight removed — that was a privilege bug), no MinStake special-casing.
- **PoW era**: exactly 1000 blocks × 31.415 AIB = 31,415 total supply, then zero coinbase forever. PoS blocks may legitimately be empty.
- **Transition bootstrap**: block 1001 is accepted from any proposer when the validator set is empty (it carries the first STAKE transaction that activates the set); from 1002 onward strict VRF sortition applies.

## [v0.11.1-testnet] — 2026-08-24

- Unique genesis: `ChainID = aib-testnet-3` + new genesis message, so stale/ghost chains (external miners on the old genesis) are rejected on connect instead of polluting sync.
- `-validator` self-registration weight bug removed (see v0.11.2 note above).

## [v0.11.0-testnet] — 2026-08-24

- Consensus determinism fixes: proposer selection uses the verified block's own height (not the local tip), validator-set iteration deterministically ordered by address.
- PoW hash excludes `Signature` (Bitcoin rule); `ProposerKey` pre-filled before mining.
- Address model V3: wallet/coinbase/validator identity = SHA256(pubkey); `Header.ProposerKey` carries the pubkey for signature checks.
- TESTNET RESET (chain data).

## Earlier

See git history (`git log --oneline`) — v0.10.x and below cover the setup wizard, drift/AutoSync/P2P height fixes, coinbase anti-inflation, and the original PoW distribution launch.

---

### Operational notes (not in code)

- **Seed node**: 212.56.43.128, systemd `aib-seed` (API 31999 loopback-only, P2P 51413 public). `aib.one/v1/*` is the public read surface via Caddy.
- **Known flaky test**: `TestState_Transition_ChainReorg` passed intermittently pre-V3; now deterministic (V3 address model applied to the test). `zkml/testnet` suite has an unrelated failure — legacy module, not consensus.
- **Migration tip for nodes stuck on an old chain** (e.g. height 100, old genesis): archive the data dir (`mv ~/.aib/data ~/aib-old/`) and restart with `-bootstrap aib.one:51413`. The setup wizard (v0.11.3+) keeps existing wallets and skips PoW mining automatically.
