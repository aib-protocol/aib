// Package api provides REST API handlers for AIB 2.0 staking operations.
// This file implements tests for staking handlers.
package api

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aib-protocol/aib/pkg/utxo"
	"github.com/aib-protocol/aib/pkg/wallet"
)

// ============================================================================
// helpers
// ============================================================================

// createTestWallet creates a test wallet
func createTestWallet(t *testing.T) (*wallet.WalletSDK, []byte) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	walletSDK, err := wallet.NewWalletSDK(&wallet.SDKConfig{
		PrivateKey: privKey,
	})
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	t.Logf("Created test wallet with address: %x", pubKey)

	return walletSDK, privKey
}

// createTestServer createtestserver
func createTestServer(t *testing.T) *Server {
	return &Server{
		mux:            http.NewServeMux(),
		utxoStore:      &mockUTXOStore{},
		mempool:        &mockMempool{},
		chain:          newMockChainReader(),
		consensusState: &mockConsensusConfig{},
		apiKeys:        []string{},
	}
}

// ============================================================================
// Mock implements
// ============================================================================

type mockUTXOStore struct {
	utxos map[string][]*utxo.UTXO
}

func (m *mockUTXOStore) GetAllUTXOs(addr [32]byte) []*utxo.UTXO {
	if m.utxos == nil {
		return []*utxo.UTXO{}
	}
	key := hex.EncodeToString(addr[:])
	return m.utxos[key]
}

func (m *mockUTXOStore) GetTransactionIndex(txHash [32]byte) (uint64, error) {
	return 0, nil
}

func (m *mockUTXOStore) GetUTXOsForAmount(addr [32]byte, amount uint64) ([]*utxo.UTXO, uint64, error) {
	utxos := m.GetAllUTXOs(addr)
	var selected []*utxo.UTXO
	var total uint64

	for _, u := range utxos {
		selected = append(selected, u)
		total += u.Value
		if total >= amount {
			break
		}
	}

	if total < amount {
		return nil, 0, errors.New("insufficient balance")
	}

	return selected, total, nil
}

func (m *mockUTXOStore) AddUTXO(addr [32]byte, u *utxo.UTXO) {
	if m.utxos == nil {
		m.utxos = make(map[string][]*utxo.UTXO)
	}
	key := hex.EncodeToString(addr[:])
	m.utxos[key] = append(m.utxos[key], u)
}

type mockMempool struct {
	transactions map[[32]byte]*utxo.Transaction
}

func (m *mockMempool) GetAllEntries() []*utxo.MempoolEntry {
	var entries []*utxo.MempoolEntry
	for _, tx := range m.transactions {
		entries = append(entries, &utxo.MempoolEntry{
			Tx: tx,
		})
	}
	return entries
}

func (m *mockMempool) GetTransaction(txHash [32]byte) *utxo.Transaction {
	return m.transactions[txHash]
}

func (m *mockMempool) AddTransaction(tx *utxo.Transaction, provider utxo.UTXOProvider) error {
	if m.transactions == nil {
		m.transactions = make(map[[32]byte]*utxo.Transaction)
	}
	txHash := tx.Hash()
	m.transactions[txHash] = tx
	return nil
}

type mockConsensusConfig struct{}

func (m *mockConsensusConfig) GetConfig() *utxo.PoSConfig {
	return &utxo.PoSConfig{}
}

func (m *mockConsensusConfig) GetActiveValidators() []*utxo.Validator {
	return nil
}

func (m *mockConsensusConfig) GetTotalStake() uint64 {
	return 0
}

func (m *mockConsensusConfig) GetValidatorCount() int {
	return 0
}

func (m *mockConsensusConfig) GetCurrentEpoch() uint64 {
	return 0
}

// ============================================================================
// test cases
// ============================================================================

// TestHandleStake teststake / stakinghandler
func TestHandleStake(t *testing.T) {
	t.Run("InvalidMethod", func(t *testing.T) {
		s := createTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/v1/stake", nil)
		w := httptest.NewRecorder()

		s.handleStake(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("MissingPrivateKey", func(t *testing.T) {
		s := createTestServer(t)

		reqBody := StakeRequest{
			Amount: "1000000000000",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/v1/stake", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		s.handleStake(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}

		var resp APIResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Success {
			t.Error("Expected failure response")
		}
	})

	t.Run("InvalidPrivateKeyFormat", func(t *testing.T) {
		s := createTestServer(t)

		reqBody := StakeRequest{
			PrivateKey: "invalid",
			Amount:     "1000000000000",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/v1/stake", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		s.handleStake(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("AmountBelowMinimum", func(t *testing.T) {
		s := createTestServer(t)
		_, privKey := createTestWallet(t)

		reqBody := StakeRequest{
			PrivateKey: hex.EncodeToString(privKey),
			Amount:     "100000000", // 1 AIB, below minimum
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/v1/stake", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		s.handleStake(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}

		var resp APIResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Error == nil {
			t.Error("Expected error response")
		} else {
			t.Logf("Got expected error: %s", resp.Error.Message)
		}
	})

	t.Run("InvalidAmountFormat", func(t *testing.T) {
		s := createTestServer(t)
		_, privKey := createTestWallet(t)

		reqBody := StakeRequest{
			PrivateKey: hex.EncodeToString(privKey),
			Amount:     "invalid",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/v1/stake", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		s.handleStake(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})
}

// TestHandleUnstake tests the unstake handler
func TestHandleUnstake(t *testing.T) {
	t.Run("InvalidMethod", func(t *testing.T) {
		s := createTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/v1/unstake", nil)
		w := httptest.NewRecorder()

		s.handleUnstake(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("MissingPrivateKey", func(t *testing.T) {
		s := createTestServer(t)

		reqBody := UnstakeRequest{
			Amount: "1000000000000",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/v1/unstake", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		s.handleUnstake(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("MissingAmount", func(t *testing.T) {
		s := createTestServer(t)
		_, privKey := createTestWallet(t)

		reqBody := UnstakeRequest{
			PrivateKey: hex.EncodeToString(privKey),
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/v1/unstake", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		s.handleUnstake(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})
}

// TestHandleGetStake teststake / stakingstatusqueryhandler
func TestHandleGetStake(t *testing.T) {
	t.Run("InvalidMethod", func(t *testing.T) {
		s := createTestServer(t)

		req := httptest.NewRequest(http.MethodPost, "/v1/wallet/stake", nil)
		w := httptest.NewRecorder()

		s.handleGetStake(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("MissingAddress", func(t *testing.T) {
		s := createTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/v1/wallet/stake", nil)
		w := httptest.NewRecorder()

		s.handleGetStake(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("InvalidAddressFormat", func(t *testing.T) {
		s := createTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/v1/wallet/stake?address=invalid", nil)
		w := httptest.NewRecorder()

		s.handleGetStake(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("InvalidAddressLength", func(t *testing.T) {
		s := createTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/v1/wallet/stake?address=abc123", nil)
		w := httptest.NewRecorder()

		s.handleGetStake(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("ValidRequest", func(t *testing.T) {
		s := createTestServer(t)
		walletSDK, _ := createTestWallet(t)

		// add some test UTXOs
		store := s.utxoStore.(*mockUTXOStore)
		addr := walletSDK.GetAddress()

		// addstake UTXO
		store.AddUTXO(addr, &utxo.UTXO{
			TxHash:  [32]byte{1},
			Index:   0,
			Value:   1000000000000,
			Script:  []byte(StakingScriptType),
			Address: addr,
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/wallet/stake?address="+hex.EncodeToString(addr[:]), nil)
		w := httptest.NewRecorder()

		s.handleGetStake(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp APIResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if !resp.Success {
			t.Errorf("Expected success response, got error: %v", resp.Error)
		}

		data, ok := resp.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Expected data to be a map")
		}

		totalStaked, ok := data["total_staked"].(string)
		if !ok {
			t.Error("Expected total_staked to be a string")
		}
		if totalStaked != "1000000000000" {
			t.Errorf("Expected total_staked to be 1000000000000, got %s", totalStaked)
		}

		t.Logf("Response: %+v", resp)
	})
}
