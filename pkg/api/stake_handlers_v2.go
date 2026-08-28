package api

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// ---- request/response types ----

// (request types StakeRequest/UnstakeRequest are declared in stake_handlers.go)

// ---- helpers ----

func aibToRaw(v float64) uint64 { return uint64(v * 1e8) }
func rawToAib(v uint64) float64 { return float64(v) / 1e8 }

type stakeCapableStore interface {
	GetUTXOsForAmount(addr [32]byte, amount uint64) ([]*utxo.UTXO, uint64, error)
	GetAllUTXOs(addr [32]byte) []*utxo.UTXO
}

// handleStake locks AIB into a stake output.
// POST /v1/stake {private_key, amount_aib}
func (s *Server) handleStakeCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "POST only", "")
		return
	}
	var req StakeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body", "")
		return
	}
	pkBytes, err := hex.DecodeString(req.PrivateKey)
	if err != nil || len(pkBytes) != 64 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "private_key must be 64-byte hex (seed+pub)", "")
		return
	}
	// amount_aib (preferred, float) takes precedence over amount (raw)
	amtStr := req.AmountAIB
	if amtStr == "" {
		amtStr = req.Amount
	}
	amount := parseRawOrAIB(amtStr)
	if amount == 0 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "amount_aib must be > 0", "")
		return
	}

	privKey := ed25519.PrivateKey(pkBytes)
	pub := privKey.Public().(ed25519.PublicKey)
	addr := walletAddrFromPub(pub)

	store, ok := s.utxoStore.(stakeCapableStore)
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}

	// select liquid UTXOs (GetUTXOsForAmount already skips stake outputs)
	fee := uint64(200)
	selected, total, err := store.GetUTXOsForAmount(addr, amount+fee)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInsufficientBalance, "Insufficient liquid balance", err.Error())
		return
	}

	inputs := make([]utxo.TXInput, len(selected))
	for i, u := range selected {
		inputs[i] = utxo.TXInput{TxHash: u.TxHash, Index: u.Index}
	}

	outputs := []utxo.TXOutput{
		{
			Value:   amount,
			Address: addr,
			Script:  []byte{utxo.StakeScriptTag}, // mark as stake
		},
	}
	if change := total - amount - fee; change > 0 {
		outputs = append(outputs, utxo.TXOutput{Value: change, Address: addr})
	}

	tx := utxo.NewTransaction(inputs, outputs)
	for i := range inputs {
		if err := tx.SignInput(i, privKey); err != nil {
			writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Failed to sign", err.Error())
			return
		}
	}

	if err := s.submitToMempool(tx); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to submit", err.Error())
		return
	}

	h := tx.Hash()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"tx_hash":    hex.EncodeToString(h[:]),
			"staked_aib": req.Amount,
			"message":    "stake submitted; weight activates when included in a block",
		},
	})
}

// handleStakeRelease unlocks stake (after cooldown check).
// POST /v1/unstake {private_key, amount_aib}
func (s *Server) handleStakeRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "POST only", "")
		return
	}
	var req UnstakeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body", "")
		return
	}
	pkBytes, err := hex.DecodeString(req.PrivateKey)
	if err != nil || len(pkBytes) != 64 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "private_key must be 64-byte hex", "")
		return
	}
	privKey := ed25519.PrivateKey(pkBytes)
	pub := privKey.Public().(ed25519.PublicKey)
	addr := walletAddrFromPub(pub)

	store, ok := s.utxoStore.(interface {
		GetAllUTXOs(addr [32]byte) []*utxo.UTXO
		GetTransactionIndex(txHash [32]byte) (uint64, error)
		GetBestHeight() uint64
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}

	// amount_aib (preferred, float) takes precedence over amount (raw)
	amtStr := req.AmountAIB
	if amtStr == "" {
		amtStr = req.Amount
	}
	amount := parseRawOrAIB(amtStr)
	all := store.GetAllUTXOs(addr)
	var stakeUTXOs []*utxo.UTXO
	for _, u := range all {
		if utxo.IsStakeOutput(u) {
			stakeUTXOs = append(stakeUTXOs, u)
		}
	}
	if len(stakeUTXOs) == 0 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "No stake outputs found", "")
		return
	}

	best := store.GetBestHeight()
	fee := uint64(200)
	var selected []*utxo.UTXO
	var total uint64
	for _, u := range stakeUTXOs {
		created, err := store.GetTransactionIndex(u.TxHash)
		if err != nil {
			continue
		}
		if best < created+utxo.UnstakeCooldownBlocks {
			continue // still cooling down
		}
		selected = append(selected, u)
		total += u.Value
		if amount > 0 && total >= amount+fee {
			break
		}
	}
	if len(selected) == 0 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "No stake past cooldown", "")
		return
	}

	inputs := make([]utxo.TXInput, len(selected))
	for i, u := range selected {
		inputs[i] = utxo.TXInput{TxHash: u.TxHash, Index: u.Index}
	}
	var outVal uint64
	if amount > 0 && total > amount+fee {
		outVal = amount
	} else {
		outVal = total - fee
	}
	outputs := []utxo.TXOutput{{Value: outVal, Address: addr}}

	tx := utxo.NewTransaction(inputs, outputs)
	for i := range inputs {
		if err := tx.SignInput(i, privKey); err != nil {
			writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Failed to sign", err.Error())
			return
		}
	}
	if err := s.submitToMempool(tx); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Failed to submit", err.Error())
		return
	}
	h := tx.Hash()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"tx_hash":      hex.EncodeToString(h[:]),
			"unstaked_aib": rawToAib(outVal),
		},
	})
}

// handleStakeInfo GET /v1/stake/info/{address}
func (s *Server) handleStakeInfo(w http.ResponseWriter, r *http.Request) {
	addrHex := r.URL.Path[len("/v1/stake/info/"):]
	addrBytes, err := hex.DecodeString(addrHex)
	if err != nil || len(addrBytes) != 32 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid address", "")
		return
	}
	var addr [32]byte
	copy(addr[:], addrBytes)

	store, ok := s.utxoStore.(stakeCapableStore)
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}
	all := store.GetAllUTXOs(addr)
	var staked, liquid uint64
	var cnt int
	for _, u := range all {
		if utxo.IsStakeOutput(u) {
			staked += u.Value
			cnt++
		} else {
			liquid += u.Value
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"address":      addrHex,
			"staked_aib":   rawToAib(staked),
			"stake_outputs": cnt,
			"liquid_aib":   rawToAib(liquid),
		},
	})
}

// walletAddrFromPub: SHA256 of the public key = wallet address.
func walletAddrFromPub(pub ed25519.PublicKey) [32]byte {
	h := sha256.Sum256(pub)
	return h
}

// parseRawOrAIB accepts raw units (integer string) or AIB (float).
func parseRawOrAIB(s string) uint64 {
	if s == "" {
		return 0
	}
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return v
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return aibToRaw(f)
	}
	return 0
}

// submitToMempool pushes a signed tx into the mempool.
func (s *Server) submitToMempool(tx *utxo.Transaction) error {
	if s.mempool == nil {
		return errMempoolUnavailable
	}
	utxoProvider, ok := s.utxoStore.(utxo.UTXOProvider)
	if !ok {
		return errMempoolUnavailable
	}
	return s.mempool.AddTransaction(tx, utxoProvider)
}

var errMempoolUnavailable = fmtError("mempool not available")

func fmtError(s string) error { return &simpleError{s} }

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }
