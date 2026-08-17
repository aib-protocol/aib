package fingerprint

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
)

// Registry stores and manages model fingerprints on-chain
type Registry struct {
	mu           sync.RWMutex
	fingerprints map[string]*ModelFingerprint // hash -> fingerprint
	nodeModels   map[string][]string          // nodeID -> []fingerprintHash
	bannedNodes  map[string]struct{}          // banned node IDs
	weightsIndex map[string]string            // weightsHash -> fingerprintHash (secondary index for O(1) lookup)
}

// NewRegistry creates a new fingerprint registry
func NewRegistry() *Registry {
	return &Registry{
		fingerprints: make(map[string]*ModelFingerprint),
		nodeModels:   make(map[string][]string),
		bannedNodes:  make(map[string]struct{}),
		weightsIndex: make(map[string]string),
	}
}

// Register adds a new model fingerprint to the registry
func (r *Registry) Register(fp *ModelFingerprint, publicKey []byte) error {
	if fp == nil {
		return errors.New("registry: nil fingerprint")
	}

	// Acquire write lock FIRST to prevent TOCTOU race condition
	// (ban check + registration must be atomic)
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if node is banned (under lock)
	nodeIDStr := string(fp.NodeID)
	if _, banned := r.bannedNodes[nodeIDStr]; banned {
		return errors.New("registry: node is banned")
	}

	// Verify fingerprint
	valid, err := VerifyFingerprint(fp, publicKey)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("registry: invalid fingerprint signature")
	}

	// Generate hash for storage
	hash, err := HashFingerprint(fp)
	if err != nil {
		return err
	}
	hashStr := string(hash)

	// Check for duplicate registration
	if _, exists := r.fingerprints[hashStr]; exists {
		return errors.New("registry: fingerprint already registered")
	}

	// Check for duplicate weights hash (same model registered by different node)
	if _, exists := r.weightsIndex[string(fp.WeightsHash)]; exists {
		return errors.New("registry: model already registered by another node")
	}

	// Store fingerprint
	r.fingerprints[hashStr] = fp
	r.nodeModels[nodeIDStr] = append(r.nodeModels[nodeIDStr], hashStr)
	r.weightsIndex[string(fp.WeightsHash)] = hashStr

	return nil
}

// HasFingerprint checks if a fingerprint is already registered (O(1) via secondary index)
func (r *Registry) HasFingerprint(weightsHash []byte) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.weightsIndex[string(weightsHash)]
	return exists
}

// GetFingerprint retrieves a fingerprint by its weights hash (O(1) via secondary index)
func (r *Registry) GetFingerprint(weightsHash []byte) *ModelFingerprint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if hashStr, exists := r.weightsIndex[string(weightsHash)]; exists {
		return r.fingerprints[hashStr]
	}
	return nil
}

// GetNodeModels returns all model fingerprints for a node
func (r *Registry) GetNodeModels(nodeID []byte) []*ModelFingerprint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodeIDStr := string(nodeID)
	hashes := r.nodeModels[nodeIDStr]

	models := make([]*ModelFingerprint, 0, len(hashes))
	for _, h := range hashes {
		if fp, exists := r.fingerprints[h]; exists {
			models = append(models, fp)
		}
	}
	return models
}

// BanNode bans a node and removes all its fingerprints
func (r *Registry) BanNode(nodeID []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	nodeIDStr := string(nodeID)

	// Remove all fingerprints for this node (and clean up weightsIndex)
	for _, hash := range r.nodeModels[nodeIDStr] {
		if fp, exists := r.fingerprints[hash]; exists {
			delete(r.weightsIndex, string(fp.WeightsHash))
		}
		delete(r.fingerprints, hash)
	}
	delete(r.nodeModels, nodeIDStr)

	// Add to banned list
	r.bannedNodes[nodeIDStr] = struct{}{}
}

// IsBanned checks if a node is banned
func (r *Registry) IsBanned(nodeID []byte) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, banned := r.bannedNodes[string(nodeID)]
	return banned
}

// Size returns the number of registered fingerprints
func (r *Registry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.fingerprints)
}

// AllFingerprints returns all registered fingerprints
func (r *Registry) AllFingerprints() []*ModelFingerprint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fps := make([]*ModelFingerprint, 0, len(r.fingerprints))
	for _, fp := range r.fingerprints {
		fps = append(fps, fp)
	}
	return fps
}

// Export exports the registry state to JSON
func (r *Registry) Export() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state := struct {
		Fingerprints map[string]*ModelFingerprint
		BannedNodes  []string
	}{
		Fingerprints: r.fingerprints,
		BannedNodes:  make([]string, 0, len(r.bannedNodes)),
	}

	for nodeID := range r.bannedNodes {
		state.BannedNodes = append(state.BannedNodes, nodeID)
	}

	return json.Marshal(state)
}

// Import imports registry state from JSON
func (r *Registry) Import(data []byte) error {
	var state struct {
		Fingerprints map[string]*ModelFingerprint
		BannedNodes  []string
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.fingerprints = state.Fingerprints
	r.bannedNodes = make(map[string]struct{})
	for _, nodeID := range state.BannedNodes {
		r.bannedNodes[nodeID] = struct{}{}
	}

	// Rebuild nodeModels and weightsIndex
	r.nodeModels = make(map[string][]string)
	r.weightsIndex = make(map[string]string)
	for hash, fp := range r.fingerprints {
		nodeIDStr := string(fp.NodeID)
		r.nodeModels[nodeIDStr] = append(r.nodeModels[nodeIDStr], hash)
		r.weightsIndex[string(fp.WeightsHash)] = hash
	}

	return nil
}

// VerificationRequest represents a challenge for model verification
type VerificationRequest struct {
	FingerprintHash []byte
	Challenge       []byte // Random challenge data
	Timestamp       int64
	ExpiresAt       int64
}

// NewVerificationRequest creates a new verification request
func NewVerificationRequest(fingerprintHash []byte, challenge []byte, ttlSeconds int64) *VerificationRequest {
	now := time.Now().Unix()
	return &VerificationRequest{
		FingerprintHash: fingerprintHash,
		Challenge:       challenge,
		Timestamp:       now,
		ExpiresAt:       now + ttlSeconds,
	}
}

// IsExpired checks if the verification request has expired
func (r *VerificationRequest) IsExpired() bool {
	return time.Now().Unix() > r.ExpiresAt
}

// VerificationResponse represents a node's response to a verification request
type VerificationResponse struct {
	RequestHash []byte // Hash of the original request
	Response    []byte // The response data (e.g., logits)
	MerkleProof [][]byte
	Timestamp   int64
	Signature   []byte
}

// VerifyResponse verifies a verification response
func (r *Registry) VerifyResponse(req *VerificationRequest, resp *VerificationResponse, publicKey []byte) (bool, error) {
	if req == nil || resp == nil {
		return false, errors.New("registry: nil request or response")
	}

	// Check request expiration
	if req.IsExpired() {
		return false, errors.New("registry: verification request expired")
	}

	// Verify request hash matches
	reqHash := hashRequest(req)
	if !bytesEqual(reqHash, resp.RequestHash) {
		return false, errors.New("registry: request hash mismatch")
	}

	// Verify response signature
	respHash := hashResponse(resp)
	if !crypto.Ed25519Verify(publicKey, respHash, resp.Signature) {
		return false, errors.New("registry: invalid response signature")
	}

	return true, nil
}

// Helper functions for registry

func hashRequest(req *VerificationRequest) []byte {
	data, _ := json.Marshal(req)
	hash := sha256.Sum256(data)
	return hash[:]
}

func hashResponse(resp *VerificationResponse) []byte {
	// Hash without signature
	data, _ := json.Marshal(struct {
		RequestHash []byte
		Response    []byte
		MerkleProof [][]byte
		Timestamp   int64
	}{
		RequestHash: resp.RequestHash,
		Response:    resp.Response,
		MerkleProof: resp.MerkleProof,
		Timestamp:   resp.Timestamp,
	})
	hash := sha256.Sum256(data)
	return hash[:]
}
