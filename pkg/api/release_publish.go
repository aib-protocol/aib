package api

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/aib-protocol/aib/pkg/utxo"
)

func hexIfSetAPI(sha [32]byte) string {
	for _, b := range sha {
		if b != 0 {
			return hex.EncodeToString(sha[:])
		}
	}
	return ""
}

// POST /v1/release/publish
//
// Anchors a release record (name + SHA256) on chain via an ordinary
// fee-paying transaction whose first output is a zero-value anchor
// script output. Body:
//
//	{"private_key":"<64-hex ed25519>","name":"v0.11.24-testnet","sha256":"<64-hex>"}
//
// The tx is signed by the caller's key, pays a fee like any other tx,
// and is gossiped via the normal mempool path.
func (s *Server) handleReleasePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}
	var req struct {
		PrivateKey      string `json:"private_key"`
		Name            string `json:"name"`
		SHA256          string `json:"sha256"`
		InstallerSHA256 string `json:"installer_sha256"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON", err.Error())
		return
	}
	nameRe := regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+[a-zA-Z0-9.\-]*$`)
	if !nameRe.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid release name", "expected e.g. v0.11.24-testnet")
		return
	}
	if len(req.SHA256) != 64 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid sha256", "expected 64 hex chars")
		return
	}
	var sha [32]byte
	if _, err := hex.Decode(sha[:], []byte(req.SHA256)); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid sha256", err.Error())
		return
	}
	var insSHA [32]byte
	if req.InstallerSHA256 != "" {
		if _, err := hex.Decode(insSHA[:], []byte(req.InstallerSHA256)); err != nil || len(req.InstallerSHA256) != 64 {
			writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid installer_sha256", "expected 64 hex chars")
			return
		}
	}
	pkBytes, err := hex.DecodeString(req.PrivateKey)
	if err != nil || len(pkBytes) != ed25519.PrivateKeySize {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid private key", "expected 64-hex ed25519 seed+pub")
		return
	}
	privKey := ed25519.PrivateKey(pkBytes)
	addr := utxo.AddressFromPublicKey(privKey.Public().(ed25519.PublicKey))

	store, ok := s.utxoStore.(stakeCapableStore)
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "UTXO store not available", "")
		return
	}
	fee := uint64(200)
	selected, total, err := store.GetUTXOsForAmount(addr, fee)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInsufficientBalance, "Insufficient balance for anchor tx fee", err.Error())
		return
	}

	inputs := make([]utxo.TXInput, len(selected))
	for i, u := range selected {
		inputs[i] = utxo.TXInput{TxHash: u.TxHash, Index: u.Index}
	}
	outputs := []utxo.TXOutput{
		{
			Value:   0,
			Address: utxo.AnchorAddress,
			Script:  utxo.BuildAnchorScript(req.Name, sha, insSHA),
		},
	}
	if change := total - fee; change > 0 {
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
	txh := tx.Hash()
	writeSuccess(w, map[string]interface{}{
		"published":        true,
		"name":             req.Name,
		"sha256":           req.SHA256,
		"installer_sha256": hexIfSetAPI(insSHA),
		"tx_hash":          hex.EncodeToString(txh[:]),
	})
}
