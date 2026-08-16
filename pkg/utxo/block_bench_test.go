// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Performance benchmarks for block operations.
package utxo

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// Benchmark Helpers
// ============================================================================

// benchTxPool is a simple in-memory UTXO provider for benchmarks.
type benchTxPool struct {
	utxos map[string]*UTXO
}

func newBenchTxPool() *benchTxPool {
	return &benchTxPool{
		utxos: make(map[string]*UTXO),
	}
}

func (b *benchTxPool) GetUTXO(txHash [32]byte, index uint32) (*UTXO, error) {
	key := UTXOKey(txHash, index)
	utxo, ok := b.utxos[key]
	if !ok {
		return nil, nil
	}
	return utxo, nil
}

func (b *benchTxPool) addUTXO(txHash [32]byte, index uint32, value uint64, addr interfaces.Address) {
	key := UTXOKey(txHash, index)
	b.utxos[key] = &UTXO{
		TxHash:  txHash,
		Index:   index,
		Value:   value,
		Script:  []byte("script"),
		Address: addr,
	}
}

// generateTestBlock creates a block with specified number of transactions for benchmarking.
func generateTestBlock(txCount int, prevHash [32]byte, height uint64) (*Block, *benchTxPool, ed25519.PrivateKey) {
	// Generate key pair for signing
	_, privKey, _ := ed25519.GenerateKey(nil)
	proposer := sha256.Sum256(privKey.Public().(ed25519.PublicKey))

	// Create UTXO pool
	utxoPool := newBenchTxPool()

	// Pre-generate some UTXOs
	for i := 0; i < txCount*2; i++ {
		txHash := sha256.Sum256([]byte{byte(i >> 8), byte(i)})
		utxoPool.addUTXO(txHash, 0, 1000000, interfaces.Address(proposer))
	}

	// Create transactions
	transactions := make([]*Transaction, txCount)
	for i := 0; i < txCount; i++ {
		// Create inputs (2 inputs per transaction)
		inputs := []TXInput{
			{
				TxHash:    sha256.Sum256([]byte{byte(i >> 8), byte(i)}),
				Index:     0,
				Signature: make([]byte, 64),
				PublicKey: privKey.Public().(ed25519.PublicKey),
			},
			{
				TxHash:    sha256.Sum256([]byte{byte((i + txCount) >> 8), byte(i + txCount)}),
				Index:     0,
				Signature: make([]byte, 64),
				PublicKey: privKey.Public().(ed25519.PublicKey),
			},
		}

		// Create outputs (2 outputs per transaction)
		outputs := []TXOutput{
			{
				Value:   500000,
				Script:  []byte("output"),
				Address: interfaces.Address(proposer),
			},
			{
				Value:   400000,
				Script:  []byte("output"),
				Address: interfaces.Address(proposer),
			},
		}

		tx := NewTransaction(inputs, outputs)
		transactions[i] = tx
	}

	// Create block
	block := NewBlock(transactions, prevHash, height, proposer)

	// Sign the block
	_ = block.SignBlock(privKey)

	return block, utxoPool, privKey
}

// generateSignedTransactions creates pre-signed transactions for mempool benchmarks.
func generateSignedTransactions(count int) ([]*Transaction, *benchTxPool, ed25519.PrivateKey) {
	_, privKey, _ := ed25519.GenerateKey(nil)
	proposer := sha256.Sum256(privKey.Public().(ed25519.PublicKey))

	utxoPool := newBenchTxPool()

	// Pre-generate UTXOs
	for i := 0; i < count*3; i++ {
		txHash := sha256.Sum256([]byte{byte(i >> 16), byte(i >> 8), byte(i)})
		utxoPool.addUTXO(txHash, 0, 10000000, interfaces.Address(proposer))
	}

	transactions := make([]*Transaction, count)
	for i := 0; i < count; i++ {
		txHash := sha256.Sum256([]byte{byte(i >> 16), byte(i >> 8), byte(i)})

		inputs := []TXInput{
			{
				TxHash:    txHash,
				Index:     0,
				Signature: make([]byte, 64),
				PublicKey: privKey.Public().(ed25519.PublicKey),
			},
		}

		outputs := []TXOutput{
			{
				Value:   5000000,
				Script:  []byte("output"),
				Address: interfaces.Address(proposer),
			},
		}

		tx := NewTransaction(inputs, outputs)
		transactions[i] = tx
	}

	return transactions, utxoPool, privKey
}

// ============================================================================
// Block Validation Benchmarks
// ============================================================================

// BenchmarkValidateBlockSmall benchmarks validation of a block with 10 transactions.
func BenchmarkValidateBlockSmall(b *testing.B) {
	block, _, _ := generateTestBlock(10, [32]byte{}, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = block.ValidateBlock()
	}
}

// BenchmarkValidateBlockMedium benchmarks validation of a block with 100 transactions.
func BenchmarkValidateBlockMedium(b *testing.B) {
	block, _, _ := generateTestBlock(100, [32]byte{}, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = block.ValidateBlock()
	}
}

// BenchmarkValidateBlockLarge benchmarks validation of a block with 1000 transactions.
func BenchmarkValidateBlockLarge(b *testing.B) {
	block, _, _ := generateTestBlock(1000, [32]byte{}, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = block.ValidateBlock()
	}
}

// BenchmarkValidateBlockChain benchmarks validation of a block in chain context.
func BenchmarkValidateBlockChain(b *testing.B) {
	// Create a chain of blocks
	parentHash := [32]byte{}
	var blocks []*Block

	for i := 0; i < 10; i++ {
		block, _, _ := generateTestBlock(50, parentHash, uint64(i))
		parentHash = block.Hash
		blocks = append(blocks, block)
	}

	// Benchmark validation of last block against parent
	lastBlock := blocks[len(blocks)-1]
	parentBlock := blocks[len(blocks)-2]

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = lastBlock.ValidateBlockChain(parentBlock)
	}
}

// ============================================================================
// Block Serialization Benchmarks
// ============================================================================

// BenchmarkSerializeBlockSmall benchmarks serialization of a block with 10 transactions.
func BenchmarkSerializeBlockSmall(b *testing.B) {
	block, _, _ := generateTestBlock(10, [32]byte{}, 1)

	var serialized []byte

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		serialized = block.SerializeBlock()
	}

	b.SetBytes(int64(len(serialized)))
}

// BenchmarkSerializeBlockMedium benchmarks serialization of a block with 100 transactions.
func BenchmarkSerializeBlockMedium(b *testing.B) {
	block, _, _ := generateTestBlock(100, [32]byte{}, 1)

	var serialized []byte

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		serialized = block.SerializeBlock()
	}

	b.SetBytes(int64(len(serialized)))
}

// BenchmarkSerializeBlockLarge benchmarks serialization of a block with 1000 transactions.
func BenchmarkSerializeBlockLarge(b *testing.B) {
	block, _, _ := generateTestBlock(1000, [32]byte{}, 1)

	var serialized []byte

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		serialized = block.SerializeBlock()
	}

	b.SetBytes(int64(len(serialized)))
}

// BenchmarkDeserializeBlockSmall benchmarks deserialization of a small block.
func BenchmarkDeserializeBlockSmall(b *testing.B) {
	block, _, _ := generateTestBlock(10, [32]byte{}, 1)
	serialized := block.SerializeBlock()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = DeserializeBlock(serialized)
	}

	b.SetBytes(int64(len(serialized)))
}

// BenchmarkDeserializeBlockMedium benchmarks deserialization of a medium block.
func BenchmarkDeserializeBlockMedium(b *testing.B) {
	block, _, _ := generateTestBlock(100, [32]byte{}, 1)
	serialized := block.SerializeBlock()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = DeserializeBlock(serialized)
	}

	b.SetBytes(int64(len(serialized)))
}

// BenchmarkDeserializeBlockLarge benchmarks deserialization of a large block.
func BenchmarkDeserializeBlockLarge(b *testing.B) {
	block, _, _ := generateTestBlock(1000, [32]byte{}, 1)
	serialized := block.SerializeBlock()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = DeserializeBlock(serialized)
	}

	b.SetBytes(int64(len(serialized)))
}

// ============================================================================
// Block Header Serialization Benchmarks
// ============================================================================

// BenchmarkSerializeHeader benchmarks block header serialization.
func BenchmarkSerializeHeader(b *testing.B) {
	block, _, _ := generateTestBlock(10, [32]byte{}, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = block.Header.Serialize()
	}
}

// BenchmarkDeserializeHeader benchmarks block header deserialization.
func BenchmarkDeserializeHeader(b *testing.B) {
	block, _, _ := generateTestBlock(10, [32]byte{}, 1)
	serialized := block.Header.Serialize()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = DeserializeBlockHeader(serialized)
	}
}

// ============================================================================
// Merkle Tree Benchmarks
// ============================================================================

// BenchmarkMerkleTreeSmall benchmarks Merkle tree calculation with 10 transactions.
func BenchmarkMerkleTreeSmall(b *testing.B) {
	block, _, _ := generateTestBlock(10, [32]byte{}, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = block.CalculateMerkleRoot()
	}
}

// BenchmarkMerkleTreeMedium benchmarks Merkle tree calculation with 100 transactions.
func BenchmarkMerkleTreeMedium(b *testing.B) {
	block, _, _ := generateTestBlock(100, [32]byte{}, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = block.CalculateMerkleRoot()
	}
}

// BenchmarkMerkleTreeLarge benchmarks Merkle tree calculation with 1000 transactions.
func BenchmarkMerkleTreeLarge(b *testing.B) {
	block, _, _ := generateTestBlock(1000, [32]byte{}, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = block.CalculateMerkleRoot()
	}
}

// BenchmarkMerkleTreeVeryLarge benchmarks Merkle tree calculation with 5000 transactions.
func BenchmarkMerkleTreeVeryLarge(b *testing.B) {
	block, _, _ := generateTestBlock(5000, [32]byte{}, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = block.CalculateMerkleRoot()
	}
}

// BenchmarkMerkleTreeIncremental benchmarks incremental Merkle tree calculation.
func BenchmarkMerkleTreeIncremental(b *testing.B) {
	// Create block with transactions
	_, privKey, _ := ed25519.GenerateKey(nil)
	_ = privKey // Used for proposer generation
	proposer := sha256.Sum256(privKey.Public().(ed25519.PublicKey))

	txCount := 100
	transactions := make([]*Transaction, txCount)
	for i := 0; i < txCount; i++ {
		outputs := []TXOutput{
			{Value: 500000, Script: []byte("out"), Address: interfaces.Address(proposer)},
		}
		tx := NewTransaction([]TXInput{}, outputs)
		transactions[i] = tx
	}

	// Pre-calculate transaction hashes
	hashes := make([][32]byte, len(transactions))
	for i, tx := range transactions {
		hashes[i] = tx.Hash()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reconstruct Merkle tree from pre-hashed transactions
		level := make([][32]byte, len(hashes))
		copy(level, hashes)

		for len(level) > 1 {
			if len(level)%2 != 0 {
				level = append(level, level[len(level)-1])
			}
			nextLevel := make([][32]byte, len(level)/2)
			for j := 0; j < len(level); j += 2 {
				concat := append(level[j][:], level[j+1][:]...)
				hash := sha256.Sum256(concat)
				nextLevel[j/2] = sha256.Sum256(hash[:])
			}
			level = nextLevel
		}
		_ = level[0]
	}
}

// ============================================================================
// Transaction Pool (Mempool) Benchmarks
// ============================================================================

// BenchmarkTxPoolAdd benchmarks adding transactions to the mempool.
func BenchmarkTxPoolAdd(b *testing.B) {
	mp := NewMempool(10000, 100)
	txs, utxoPool, _ := generateSignedTransactions(1000)

	// Add transactions to pool one by one
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N && i < len(txs); i++ {
		_ = mp.AddTransaction(txs[i], utxoPool)
	}
}

// BenchmarkTxPoolGet benchmarks getting a transaction from the mempool.
func BenchmarkTxPoolGet(b *testing.B) {
	mp := NewMempool(10000, 100)
	txs, utxoPool, _ := generateSignedTransactions(1000)

	// Add all transactions
	for _, tx := range txs {
		_ = mp.AddTransaction(tx, utxoPool)
	}

	// Benchmark get operations
	txHashes := make([][32]byte, len(txs))
	for i, tx := range txs {
		txHashes[i] = tx.Hash()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = mp.GetTransaction(txHashes[i%len(txHashes)])
	}
}

// BenchmarkTxPoolRemove benchmarks removing transactions from the mempool.
func BenchmarkTxPoolRemove(b *testing.B) {
	mp := NewMempool(10000, 100)
	txs, utxoPool, _ := generateSignedTransactions(1000)

	// Add all transactions
	for _, tx := range txs {
		_ = mp.AddTransaction(tx, utxoPool)
	}

	txHashes := make([][32]byte, len(txs))
	for i, tx := range txs {
		txHashes[i] = tx.Hash()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N && i < len(txHashes); i++ {
		mp.RemoveTransaction(txHashes[i%len(txHashes)])
		// Re-add for next iteration
		_ = mp.AddTransaction(txs[i%len(txs)], utxoPool)
	}
}

// BenchmarkTxPoolGetTransactionsForBlock benchmarks selecting transactions for block inclusion.
func BenchmarkTxPoolGetTransactionsForBlock(b *testing.B) {
	mp := NewMempool(10000, 100)
	txs, utxoPool, _ := generateSignedTransactions(500)

	// Add all transactions
	for _, tx := range txs {
		_ = mp.AddTransaction(tx, utxoPool)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = mp.GetTransactionsForBlock(1000000)
	}
}

// BenchmarkTxPoolSize benchmarks getting the mempool size.
func BenchmarkTxPoolSize(b *testing.B) {
	mp := NewMempool(10000, 100)
	txs, utxoPool, _ := generateSignedTransactions(1000)

	// Add all transactions
	for _, tx := range txs {
		_ = mp.AddTransaction(tx, utxoPool)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = mp.Size()
	}
}

// BenchmarkTxPoolPrune benchmarks pruning expired transactions from mempool.
func BenchmarkTxPoolPrune(b *testing.B) {
	mp := NewMempool(10000, 100)
	txs, utxoPool, _ := generateSignedTransactions(1000)

	// Add all transactions
	for _, tx := range txs {
		_ = mp.AddTransaction(tx, utxoPool)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mp.Prune(0) // Prune all with 0 age
		// Re-add transactions
		for _, tx := range txs {
			_ = mp.AddTransaction(tx, utxoPool)
		}
	}
}

// ============================================================================
// Block Hash Benchmarks
// ============================================================================

// BenchmarkBlockHash benchmarks block hash calculation.
func BenchmarkBlockHash(b *testing.B) {
	block, _, _ := generateTestBlock(10, [32]byte{}, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = block.CalculateHash()
	}
}

// ============================================================================
// Transaction Benchmarks
// ============================================================================

// BenchmarkTransactionSerialize benchmarks transaction serialization.
func BenchmarkTransactionSerialize(b *testing.B) {
	_, privKey, _ := ed25519.GenerateKey(nil)
	proposer := sha256.Sum256(privKey.Public().(ed25519.PublicKey))

	tx := NewTransaction(
		[]TXInput{
			{
				TxHash:    [32]byte{1},
				Index:     0,
				Signature: make([]byte, 64),
				PublicKey: privKey.Public().(ed25519.PublicKey),
			},
		},
		[]TXOutput{
			{Value: 500000, Script: []byte("output"), Address: interfaces.Address(proposer)},
			{Value: 400000, Script: []byte("output"), Address: interfaces.Address(proposer)},
		},
	)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = tx.Serialize()
	}
}

// BenchmarkTransactionDeserialize benchmarks transaction deserialization.
func BenchmarkTransactionDeserialize(b *testing.B) {
	_, privKey, _ := ed25519.GenerateKey(nil)
	proposer := sha256.Sum256(privKey.Public().(ed25519.PublicKey))

	tx := NewTransaction(
		[]TXInput{
			{
				TxHash:    [32]byte{1},
				Index:     0,
				Signature: make([]byte, 64),
				PublicKey: privKey.Public().(ed25519.PublicKey),
			},
		},
		[]TXOutput{
			{Value: 500000, Script: []byte("output"), Address: interfaces.Address(proposer)},
			{Value: 400000, Script: []byte("output"), Address: interfaces.Address(proposer)},
		},
	)

	serialized := tx.Serialize()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = DeserializeTransaction(serialized)
	}
}

// BenchmarkTransactionHash benchmarks transaction hash calculation.
func BenchmarkTransactionHash(b *testing.B) {
	_, privKey, _ := ed25519.GenerateKey(nil)
	proposer := sha256.Sum256(privKey.Public().(ed25519.PublicKey))

	tx := NewTransaction(
		[]TXInput{
			{
				TxHash:    [32]byte{1},
				Index:     0,
				Signature: make([]byte, 64),
				PublicKey: privKey.Public().(ed25519.PublicKey),
			},
		},
		[]TXOutput{
			{Value: 500000, Script: []byte("output"), Address: interfaces.Address(proposer)},
		},
	)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = tx.Hash()
	}
}

// ============================================================================
// Block Signature Benchmarks
// ============================================================================

// BenchmarkBlockSign benchmarks block signing.
func BenchmarkBlockSign(b *testing.B) {
	block, _, privKey := generateTestBlock(10, [32]byte{}, 1)

	// Remove signature for benchmark
	block.Header.Signature = nil

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		block.Header.Signature = nil
		_ = block.SignBlock(privKey)
	}
}

// BenchmarkBlockVerifySignature benchmarks block signature verification.
func BenchmarkBlockVerifySignature(b *testing.B) {
	block, _, _ := generateTestBlock(10, [32]byte{}, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = block.VerifyBlockSignature()
	}
}

// ============================================================================
// UTXO Store Benchmarks
// ============================================================================

// BenchmarkUTXOStoreGet benchmarks getting a UTXO from the store.
func BenchmarkUTXOStoreGet(b *testing.B) {
	store := NewUTXOStore()

	// Add many UTXOs
	for i := 0; i < 10000; i++ {
		txHash := sha256.Sum256([]byte{byte(i >> 16), byte(i >> 8), byte(i)})
		store.AddUTXO(&UTXO{
			TxHash:  txHash,
			Index:   0,
			Value:   1000000,
			Address: interfaces.Address{byte(i)},
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		txHash := sha256.Sum256([]byte{byte(i >> 16), byte(i >> 8), byte(i)})
		_, _ = store.GetUTXO(txHash, 0)
	}
}

// BenchmarkUTXOStoreAdd benchmarks adding a UTXO to the store.
func BenchmarkUTXOStoreAdd(b *testing.B) {
	store := NewUTXOStore()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		txHash := sha256.Sum256([]byte{byte(i >> 16), byte(i >> 8), byte(i)})
		store.AddUTXO(&UTXO{
			TxHash:  txHash,
			Index:   0,
			Value:   1000000,
			Address: interfaces.Address{byte(i)},
		})
	}
}

// BenchmarkUTXOStoreSpend benchmarks spending a UTXO.
func BenchmarkUTXOStoreSpend(b *testing.B) {
	store := NewUTXOStore()

	// Pre-add UTXOs
	for i := 0; i < 1000; i++ {
		txHash := sha256.Sum256([]byte{byte(i >> 16), byte(i >> 8), byte(i)})
		store.AddUTXO(&UTXO{
			TxHash:  txHash,
			Index:   0,
			Value:   1000000,
			Address: interfaces.Address{byte(i)},
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		txHash := sha256.Sum256([]byte{byte((i % 1000) >> 16), byte((i % 1000) >> 8), byte(i % 1000)})
		_ = store.SpendUTXO(txHash, 0)
		// Re-add for next iteration
		store.AddUTXO(&UTXO{
			TxHash:  txHash,
			Index:   0,
			Value:   1000000,
			Address: interfaces.Address{byte(i % 1000)},
		})
	}
}

// BenchmarkUTXOStoreGetBalance benchmarks getting balance for an address.
func BenchmarkUTXOStoreGetBalance(b *testing.B) {
	store := NewUTXOStore()

	targetAddr := interfaces.Address{1, 2, 3}

	// Add many UTXOs for the same address
	for i := 0; i < 1000; i++ {
		txHash := sha256.Sum256([]byte{byte(i >> 8), byte(i)})
		store.AddUTXO(&UTXO{
			TxHash:  txHash,
			Index:   0,
			Value:   1000000,
			Address: targetAddr,
		})
		// Add some for other addresses
		store.AddUTXO(&UTXO{
			TxHash:  txHash,
			Index:   1,
			Value:   1000000,
			Address: interfaces.Address{byte(i + 10)},
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = store.GetBalance(targetAddr)
	}
}
