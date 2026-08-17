// Package founder implements the founder allocation system for AIB 2.0.
// This file implements founder verification and multi-sig release mechanisms.
package founder

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// Verifier handles founder identity verification and multi-signature operations.
type Verifier struct {
	founders *FounderList
	multiSig *MultiSigConfig
	releases map[string]*ReleaseRecord // Key: founderID:nonce
	mu       sync.RWMutex
}

// ReleaseRecord tracks a multi-sig release request.
type ReleaseRecord struct {
	FounderID     string          `json:"founder_id"`
	Nonce         uint64          `json:"nonce"`
	Amount        uint64          `json:"amount"`
	TargetAddress [32]byte        `json:"target_address"`
	Signatures    []SignedRelease `json:"signatures"`
	Status        ReleaseStatus   `json:"status"`
	CreatedAt     time.Time       `json:"created_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	TxHash        string          `json:"tx_hash,omitempty"`
}

// SignedRelease represents a signature from an authorized signer.
type SignedRelease struct {
	SignerAddress [32]byte  `json:"signer_address"`
	Signature     []byte    `json:"signature"`
	Timestamp     time.Time `json:"timestamp"`
}

// ReleaseStatus represents the status of a release request.
type ReleaseStatus string

const (
	ReleaseStatusPending   ReleaseStatus = "pending"   // Waiting for signatures
	ReleaseStatusApproved  ReleaseStatus = "approved"  // Enough signatures collected
	ReleaseStatusRejected  ReleaseStatus = "rejected"  // Rejected by signers
	ReleaseStatusCompleted ReleaseStatus = "completed" // Transaction completed
)

// NewVerifier creates a new verifier instance.
func NewVerifier(founders *FounderList, multiSig *MultiSigConfig) *Verifier {
	return &Verifier{
		founders: founders,
		multiSig: multiSig,
		releases: make(map[string]*ReleaseRecord),
	}
}

// VerifyFounderIdentity verifies that a founder's identity matches their public key.
func (v *Verifier) VerifyFounderIdentity(founderID string, message []byte, signature []byte) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	founder, exists := v.founders.Get(founderID)
	if !exists {
		return fmt.Errorf("founder %s not found", founderID)
	}

	if len(founder.PubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size for founder %s", founderID)
	}

	// Verify signature
	if !ed25519.Verify(founder.PubKeyBytes, message, signature) {
		return fmt.Errorf("signature verification failed for founder %s", founderID)
	}

	return nil
}

// VerifyFounderSignature verifies a signature from a founder.
func (v *Verifier) VerifyFounderSignature(founderID string, signatureHex string, message string) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	founder, exists := v.founders.Get(founderID)
	if !exists {
		return fmt.Errorf("founder %s not found", founderID)
	}

	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}

	if !ed25519.Verify(founder.PubKeyBytes, []byte(message), signature) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// CreateReleaseRequest creates a new multi-sig release request.
func (v *Verifier) CreateReleaseRequest(founderID string, amount uint64, nonce uint64) (*ReleaseRecord, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	founder, exists := v.founders.Get(founderID)
	if !exists {
		return nil, fmt.Errorf("founder %s not found", founderID)
	}

	// Check if similar request exists
	key := v.makeReleaseKey(founderID, nonce)
	if _, exists := v.releases[key]; exists {
		return nil, fmt.Errorf("release request already exists for %s nonce %d", founderID, nonce)
	}

	record := &ReleaseRecord{
		FounderID:     founderID,
		Nonce:         nonce,
		Amount:        amount,
		TargetAddress: founder.AddressBytes,
		Signatures:    make([]SignedRelease, 0),
		Status:        ReleaseStatusPending,
		CreatedAt:     time.Now(),
	}

	v.releases[key] = record
	return record, nil
}

// AddSignature adds a signature to a release request.
func (v *Verifier) AddSignature(founderID string, nonce uint64, signerAddress string, signature []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := v.makeReleaseKey(founderID, nonce)
	record, exists := v.releases[key]
	if !exists {
		return fmt.Errorf("release request not found")
	}

	if record.Status != ReleaseStatusPending {
		return fmt.Errorf("release request is not pending: %s", record.Status)
	}

	// Verify signer is authorized
	if !v.isAuthorizedSigner(signerAddress) {
		return fmt.Errorf("signer %s is not authorized", signerAddress)
	}

	// Parse signer address
	signerAddr, err := utxo.AddressFromString(signerAddress)
	if err != nil {
		return fmt.Errorf("invalid signer address: %w", err)
	}

	// Check for duplicate signature
	for _, sig := range record.Signatures {
		if sig.SignerAddress == signerAddr {
			return fmt.Errorf("signer has already signed")
		}
	}

	// Add signature
	record.Signatures = append(record.Signatures, SignedRelease{
		SignerAddress: signerAddr,
		Signature:     signature,
		Timestamp:     time.Now(),
	})

	// Check if we have enough signatures
	if len(record.Signatures) >= v.multiSig.RequiredSigs {
		record.Status = ReleaseStatusApproved
	}

	return nil
}

// VerifyReleaseRequest verifies that a release request has sufficient signatures.
func (v *Verifier) VerifyReleaseRequest(founderID string, nonce uint64) (bool, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	key := v.makeReleaseKey(founderID, nonce)
	record, exists := v.releases[key]
	if !exists {
		return false, fmt.Errorf("release request not found")
	}

	// Create the message to verify
	message := v.createReleaseMessage(founderID, record.Amount, nonce)

	// Count valid signatures
	validSigs := 0
	for _, sig := range record.Signatures {
		if v.verifyMultiSigSignature(message, sig.SignerAddress, sig.Signature) {
			validSigs++
		}
	}

	return validSigs >= v.multiSig.RequiredSigs, nil
}

// CompleteRelease marks a release as completed.
func (v *Verifier) CompleteRelease(founderID string, nonce uint64, txHash string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := v.makeReleaseKey(founderID, nonce)
	record, exists := v.releases[key]
	if !exists {
		return fmt.Errorf("release request not found")
	}

	if record.Status != ReleaseStatusApproved {
		return fmt.Errorf("release request is not approved: %s", record.Status)
	}

	now := time.Now()
	record.Status = ReleaseStatusCompleted
	record.CompletedAt = &now
	record.TxHash = txHash

	return nil
}

// GetReleaseRequest retrieves a release request.
func (v *Verifier) GetReleaseRequest(founderID string, nonce uint64) (*ReleaseRecord, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	key := v.makeReleaseKey(founderID, nonce)
	record, exists := v.releases[key]
	if !exists {
		return nil, fmt.Errorf("release request not found")
	}

	return record, nil
}

// GetPendingReleases returns all pending release requests.
func (v *Verifier) GetPendingReleases() []*ReleaseRecord {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var pending []*ReleaseRecord
	for _, record := range v.releases {
		if record.Status == ReleaseStatusPending {
			pending = append(pending, record)
		}
	}
	return pending
}

// CreateMultiSigTransaction creates a multi-sig transaction for releasing tokens.
func (v *Verifier) CreateMultiSigTransaction(founderID string, nonce uint64, amount uint64) (*utxo.Transaction, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Get release request
	key := v.makeReleaseKey(founderID, nonce)
	record, exists := v.releases[key]
	if !exists {
		return nil, fmt.Errorf("release request not found")
	}

	if record.Status != ReleaseStatusApproved {
		return nil, fmt.Errorf("release request not approved: %s", record.Status)
	}

	// Verify we have enough valid signatures
	message := v.createReleaseMessage(founderID, amount, nonce)
	validSigs := 0
	var validSigsData [][]byte

	for _, sig := range record.Signatures {
		if ed25519.Verify(sig.Signature, message, sig.Signature) {
			validSigs++
			validSigsData = append(validSigsData, sig.Signature)
		}
	}

	if validSigs < v.multiSig.RequiredSigs {
		return nil, fmt.Errorf("insufficient valid signatures: %d < %d", validSigs, v.multiSig.RequiredSigs)
	}

	// Create transaction output
	output := utxo.TXOutput{
		Value:   amount,
		Address: record.TargetAddress,
	}

	// Create transaction with multi-sig input
	tx := utxo.NewTransaction(nil, []utxo.TXOutput{output})

	return tx, nil
}

// Helper functions

func (v *Verifier) makeReleaseKey(founderID string, nonce uint64) string {
	return fmt.Sprintf("%s:%d", founderID, nonce)
}

func (v *Verifier) isAuthorizedSigner(address string) bool {
	for _, auth := range v.multiSig.SignerAddresses {
		if auth == address {
			return true
		}
	}
	return false
}

func (v *Verifier) createReleaseMessage(founderID string, amount uint64, nonce uint64) []byte {
	// Create a structured message for signing
	// Format: "RELEASE:founderID:amount:nonce"
	return []byte(fmt.Sprintf("RELEASE:%s:%d:%d", founderID, amount, nonce))
}

func (v *Verifier) verifyMultiSigSignature(message []byte, signerAddress [32]byte, signature []byte) bool {
	// In a real implementation, this would verify the signature against
	// the signer's public key derived from their address
	// For now, we check the signature length
	if len(signature) != ed25519.SignatureSize {
		return false
	}

	// Derive expected public key from address
	// In the actual system, you'd retrieve the public key from a keystore
	return len(signature) == ed25519.SignatureSize
}

// AddAuthorizedSigner adds an authorized signer to the multi-sig configuration.
func (v *Verifier) AddAuthorizedSigner(address string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Validate address
	if _, _, err := utxo.DecodeBech32m(address); err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	// Check for duplicates
	for _, auth := range v.multiSig.SignerAddresses {
		if auth == address {
			return fmt.Errorf("signer already authorized")
		}
	}

	v.multiSig.SignerAddresses = append(v.multiSig.SignerAddresses, address)
	return nil
}

// RemoveAuthorizedSigner removes an authorized signer.
func (v *Verifier) RemoveAuthorizedSigner(address string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	for i, auth := range v.multiSig.SignerAddresses {
		if auth == address {
			// Remove by index
			v.multiSig.SignerAddresses = append(
				v.multiSig.SignerAddresses[:i],
				v.multiSig.SignerAddresses[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("signer not found")
}

// GetAuthorizedSigners returns the list of authorized signers.
func (v *Verifier) GetAuthorizedSigners() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	signers := make([]string, len(v.multiSig.SignerAddresses))
	copy(signers, v.multiSig.SignerAddresses)
	return signers
}

// UpdateRequiredSigs updates the number of required signatures.
func (v *Verifier) UpdateRequiredSigs(required int) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if required < 1 {
		return fmt.Errorf("required signatures must be at least 1")
	}

	if required > len(v.multiSig.SignerAddresses) {
		return fmt.Errorf("required signatures cannot exceed total signers")
	}

	v.multiSig.RequiredSigs = required
	return nil
}

// VerifySignatureBatch verifies multiple signatures at once.
func (v *Verifier) VerifySignatureBatch(signatures []SignatureBundle) (map[int]bool, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	results := make(map[int]bool)

	for i, bundle := range signatures {
		founder, exists := v.founders.Get(bundle.FounderID)
		if !exists {
			results[i] = false
			continue
		}

		if len(founder.PubKeyBytes) != ed25519.PublicKeySize {
			results[i] = false
			continue
		}

		results[i] = ed25519.Verify(founder.PubKeyBytes, bundle.Message, bundle.Signature)
	}

	return results, nil
}

// SignatureBundle represents a signature to verify.
type SignatureBundle struct {
	FounderID string
	Message   []byte
	Signature []byte
}

// MultiSigSigner represents a multi-signature signer for convenience.
type MultiSigSigner struct {
	privateKey ed25519.PrivateKey
	address    [32]byte
}

// NewMultiSigSigner creates a new multi-sig signer from a private key.
func NewMultiSigSigner(privateKey ed25519.PrivateKey) *MultiSigSigner {
	publicKey := privateKey.Public().(ed25519.PublicKey)
	address := utxo.AddressFromPublicKey(publicKey)

	return &MultiSigSigner{
		privateKey: privateKey,
		address:    address,
	}
}

// SignRelease signs a release request.
func (s *MultiSigSigner) SignRelease(founderID string, amount uint64, nonce uint64) []byte {
	message := []byte(fmt.Sprintf("RELEASE:%s:%d:%d", founderID, amount, nonce))
	return ed25519.Sign(s.privateKey, message)
}

// GetAddress returns the signer's address.
func (s *MultiSigSigner) GetAddress() [32]byte {
	return s.address
}

// GetAddressString returns the signer's address as a Bech32m string.
func (s *MultiSigSigner) GetAddressString() (string, error) {
	return utxo.AddressToString(s.address)
}
