package utxo

import (
	"sync/atomic"
	"testing"
)

func BenchmarkMempoolAddTransaction(b *testing.B) {
	const targetIterations = 1000
	mempool := NewMempool(targetIterations+1024, 10)
	provider := newMockUTXOProvider()

	txs := make([]*Transaction, targetIterations)
	for i := 0; i < targetIterations; i++ {
		privKey, _, addr := generateKeyPair()
		_, fundingUTXOs := createFundingTransaction(addr, 1000)
		for _, utxo := range fundingUTXOs {
			provider.addUTXO(utxo)
		}
		txs[i] = createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
	}

	txSize := txs[0].SerializeSize()
	b.SetBytes(int64(txSize))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tx := txs[i%targetIterations]
		if err := mempool.AddTransaction(tx, provider); err != nil {
			b.Fatalf("add transaction failed at %d: %v", i, err)
		}
	}
}

func BenchmarkMempoolGetTransaction(b *testing.B) {
	const txCount = 4096
	mempool := NewMempool(txCount+1024, 10)
	provider := newMockUTXOProvider()
	txHashes := make([][32]byte, txCount)

	for i := 0; i < txCount; i++ {
		privKey, _, addr := generateKeyPair()
		_, fundingUTXOs := createFundingTransaction(addr, 1000)
		for _, utxo := range fundingUTXOs {
			provider.addUTXO(utxo)
		}
		tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
		if err := mempool.AddTransaction(tx, provider); err != nil {
			b.Fatalf("prepare add transaction failed at %d: %v", i, err)
		}
		txHashes[i] = tx.Hash()
	}

	txSize := mempool.GetTransaction(txHashes[0]).SerializeSize()
	b.SetBytes(int64(txSize))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tx := mempool.GetTransaction(txHashes[i%txCount])
		if tx == nil {
			b.Fatalf("transaction not found at iteration %d", i)
		}
	}
}

func BenchmarkMempoolSelectTransactions(b *testing.B) {
	const txCount = 3000
	mempool := NewMempool(txCount+1024, 10)
	provider := newMockUTXOProvider()

	for i := 0; i < txCount; i++ {
		privKey, _, addr := generateKeyPair()
		_, fundingUTXOs := createFundingTransaction(addr, 1000+uint64(i))
		for _, utxo := range fundingUTXOs {
			provider.addUTXO(utxo)
		}
		fee := uint64(20 + (i % 200))
		tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, fee)
		if err := mempool.AddTransaction(tx, provider); err != nil {
			b.Fatalf("prepare add transaction failed at %d: %v", i, err)
		}
	}

	blockLimit := 128 * 1024
	selected := mempool.GetTransactionsForBlock(blockLimit)
	if len(selected) == 0 {
		b.Fatal("no transactions selected during preparation")
	}

	b.SetBytes(int64(blockLimit))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		txs := mempool.GetTransactionsForBlock(blockLimit)
		if len(txs) == 0 {
			b.Fatalf("no transactions selected at iteration %d", i)
		}
	}
}

func BenchmarkMempoolConcurrentAccess(b *testing.B) {
	const preloadCount = 2048
	mempool := NewMempool(preloadCount+10000, 10)
	provider := newMockUTXOProvider()
	lookupHashes := make([][32]byte, preloadCount)

	for i := 0; i < preloadCount; i++ {
		privKey, _, addr := generateKeyPair()
		_, fundingUTXOs := createFundingTransaction(addr, 1000)
		for _, utxo := range fundingUTXOs {
			provider.addUTXO(utxo)
		}
		tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
		if err := mempool.AddTransaction(tx, provider); err != nil {
			b.Fatalf("preload add transaction failed at %d: %v", i, err)
		}
		lookupHashes[i] = tx.Hash()
	}

	baseTxSize := mempool.GetTransaction(lookupHashes[0]).SerializeSize()
	b.SetBytes(int64(baseTxSize))
	b.ReportAllocs()

	var seq uint64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := int(atomic.AddUint64(&seq, 1) - 1)
			if idx%3 == 0 {
				hash := lookupHashes[idx%preloadCount]
				if mempool.GetTransaction(hash) == nil {
					b.Fatalf("transaction not found at parallel idx %d", idx)
				}
			} else if idx%3 == 1 {
				selected := mempool.GetTransactionsForBlock(64 * 1024)
				if len(selected) == 0 {
					b.Fatalf("no transactions selected at parallel idx %d", idx)
				}
			} else {
				privKey, _, addr := generateKeyPair()
				_, fundingUTXOs := createFundingTransaction(addr, 1000)
				for _, utxo := range fundingUTXOs {
					provider.addUTXO(utxo)
				}
				tx := createTestTransaction(privKey, addr, fundingUTXOs, 500, 50)
				if err := mempool.AddTransaction(tx, provider); err != nil {
					b.Fatalf("parallel add transaction failed at idx %d: %v", idx, err)
				}
			}
		}
	})
}
