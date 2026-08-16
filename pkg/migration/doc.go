// Package migration implements token migration from AIB1 snapshot and cross-chain bridging.
//
// Overview:
//   - AIB1 Migration: Users who held AIB1 tokens as of the snapshot date (2026-01-01)
//     can claim AIB2 tokens at a 1:1 ratio. Claim deadline is 2028-01-01.
//   - Cross-Chain Migration: Users can migrate BTC, ETH, or SOL to AIB2 tokens during
//     the 3-month window with dynamic incentive rates. Tokens are vested over time.
//
// Package migration provides:
//   - AIB1Migration: Snapshot-based token claiming with Ed25519 signature verification
//   - CrossChainMigration: Cross-chain bridging with vesting schedules
//   - MigrationHub: Central orchestrator for all migration activities
//
// Key Features:
//   - Merkle tree based snapshot proofs
//   - Ed25519 signature verification for AIB1 claims
//   - Dynamic incentive rates (early birds get higher rates)
//   - Time-based vesting with TGE immediate unlock
//   - Multi-signature relayer verification for cross-chain proofs
//
// Usage:
//
//	// Create migration hub
//	cfg := migration.DefaultHubConfig()
//	cfg.Minter = yourTokenMinter
//	hub, _ := migration.NewMigrationHub(cfg)
//
//	// Load AIB1 snapshot
//	hub.LoadAIB1Snapshot(snapshotRecords)
//
//	// Claim AIB1 tokens
//	err := hub.ClaimAIB1(targetAddr, amount, pubKey, signature, nonce)
//
//	// Migrate BTC
//	proof := &migration.CrossChainProof{...}
//	err := hub.MigrateBTC(userAddr, proof)
//
//	// Claim unlocked tokens from vesting
//	claimed, err := hub.ClaimUnlocked(userAddr)
package migration
