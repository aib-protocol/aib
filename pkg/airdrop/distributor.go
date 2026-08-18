package airdrop

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
)

var (
	// ErrAlreadyClaimed already claimed
	ErrAlreadyClaimed = errors.New("airdrop already claimed")
	// ErrIneligible not eligible
	ErrIneligible = errors.New("not eligible for airdrop")
	// ErrTeamAddress team address excluded
	ErrTeamAddress = errors.New("team address is excluded")
	// ErrContractAddress contract address excluded
	ErrContractAddress = errors.New("contract address is excluded")
	// ErrInvalidSignature signatureinvalid
	ErrInvalidSignature = errors.New("invalid signature")
	// ErrDistributionDisabled distribution disabled
	ErrDistributionDisabled = errors.New("airdrop distribution is disabled")
)

// AirdropAmount airdropamount
type AirdropAmount struct {
	Base  uint64 `json:"base"`  // base amount
	Bonus uint64 `json:"bonus"` // bonus amount
	Total uint64 `json:"total"` // total amount
}

// ClaimRecord claim record
type ClaimRecord struct {
	Address     string         `json:"address"`
	GitHubID    uint64         `json:"github_id"`
	GitHubLogin string         `json:"github_login"`
	Email       string         `json:"email,omitempty"`
	DeviceID    string         `json:"device_id"`
	IPAddress   string         `json:"ip_address"`
	Amount      *AirdropAmount `json:"amount"`
	Score       int            `json:"score"`
	Timestamp   time.Time      `json:"timestamp"`
	TxHash      string         `json:"tx_hash,omitempty"`
	Signature   string         `json:"signature"`
}

// DistributorConfig distributor configuration
type DistributorConfig struct {
	// base airdrop amount
	BaseAmount uint64
	// max bonus multiplier
	MaxBonusMultiplier float64

	// distribution enabled flag
	Enabled bool

	// signature verification requirement
	RequireSignature bool

	// max total airdrop amount
	MaxTotalAmount uint64

	// minimum score per claim
	MinClaimScore int
}

// DefaultDistributorConfig default distributor configuration
func DefaultDistributorConfig() *DistributorConfig {
	baseAmount := uint64(1000)
	for i := 0; i < 18; i++ {
		baseAmount *= 10
	}

	maxTotalAmount := uint64(100000000)
	for i := 0; i < 18; i++ {
		maxTotalAmount *= 10
	}

	return &DistributorConfig{
		BaseAmount:         baseAmount, // 1000 tokens (1e21)
		MaxBonusMultiplier: 5.0,        // up to 5x bonus
		Enabled:            true,
		RequireSignature:   true,
		MaxTotalAmount:     maxTotalAmount, // 100M tokens (1e26)
		MinClaimScore:      50,
	}
}

// Distributor airdrop distributor
type Distributor struct {
	config           *DistributorConfig
	claimer          *AirdropSigner
	claimedAddresses map[string]*ClaimRecord
	claimedGitHubIDs map[uint64]*ClaimRecord
	mu               sync.RWMutex

	// excluded team and contract addresses
	excludedAddresses map[string]bool
	excludedContracts map[string]bool

	// total distributed amount
	distributedAmount uint64

	// distributed storage (optional)
	storage Storage
}

// Storage storageinterface
type Storage interface {
	SaveClaim(record *ClaimRecord) error
	LoadClaim(address string) (*ClaimRecord, error)
	LoadClaimsByGitHub(githubID uint64) ([]*ClaimRecord, error)
}

// AirdropSigner airdropsignature
type AirdropSigner struct {
	signer *crypto.Ed25519Signer
}

// NewAirdropSigner createairdropsignature
func NewAirdropSigner(seed []byte) (*AirdropSigner, error) {
	signer, err := crypto.NewEd25519SignerFromSeed(seed)
	if err != nil {
		return nil, err
	}
	return &AirdropSigner{signer: signer}, nil
}

// SignClaim sign a claim request
func (as *AirdropSigner) SignClaim(address string, amount uint64, timestamp int64) ([]byte, error) {
	message := fmt.Sprintf("%s:%d:%d", address, amount, timestamp)
	return as.signer.Sign([]byte(message))
}

// PublicKey returns the public key
func (as *AirdropSigner) PublicKey() []byte {
	return as.signer.PublicKey()
}

// NewDistributor creates a distributor
func NewDistributor(config *DistributorConfig, signerSeed []byte) (*Distributor, error) {
	if config == nil {
		config = DefaultDistributorConfig()
	}

	signer, err := NewAirdropSigner(signerSeed)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}

	return &Distributor{
		config:            config,
		claimer:           signer,
		claimedAddresses:  make(map[string]*ClaimRecord),
		claimedGitHubIDs:  make(map[uint64]*ClaimRecord),
		excludedAddresses: make(map[string]bool),
		excludedContracts: make(map[string]bool),
	}, nil
}

// CalculateAmount computeairdropamount
func (d *Distributor) CalculateAmount(score int, maxScore int) *AirdropAmount {
	if score < d.config.MinClaimScore {
		return &AirdropAmount{Base: 0, Bonus: 0, Total: 0}
	}

	// base amount
	base := d.config.BaseAmount

	// calculate bonus multiplier (based on score ratio)
	ratio := float64(score) / float64(maxScore)
	bonusMultiplier := ratio * d.config.MaxBonusMultiplier
	bonus := uint64(float64(base) * bonusMultiplier)

	total := base + bonus

	return &AirdropAmount{
		Base:  base,
		Bonus: bonus,
		Total: total,
	}
}

// CanClaim checks whether a claim can be made
func (d *Distributor) CanClaim(address string, githubID uint64, score int) (*AirdropAmount, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// check whether distribution is enabled
	if !d.config.Enabled {
		return nil, ErrDistributionDisabled
	}

	// check score
	if score < d.config.MinClaimScore {
		return nil, ErrIneligible
	}

	// check whether address already claimed
	if _, claimed := d.claimedAddresses[address]; claimed {
		return nil, ErrAlreadyClaimed
	}

	// check whether GitHub ID already claimed
	if _, claimed := d.claimedGitHubIDs[githubID]; claimed {
		return nil, ErrAlreadyClaimed
	}

	// check excluded addresses
	if d.excludedAddresses[address] {
		return nil, ErrTeamAddress
	}

	// checkcontractaddress
	if d.isContractAddress(address) {
		return nil, ErrContractAddress
	}

	// check total amount limit
	amount := d.CalculateAmount(score, 100)
	if d.distributedAmount+amount.Total > d.config.MaxTotalAmount {
		return nil, errors.New("airdrop pool exhausted")
	}

	return amount, nil
}

// Claim claims the airdrop
func (d *Distributor) Claim(req *ClaimRequest) (*ClaimRecord, error) {
	// check eligibility first (no lock needed)
	amount, err := d.CanClaim(req.Address, req.GitHubID, req.Score)
	if err != nil {
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// 1. verifysign
	if d.config.RequireSignature {
		if req.Signature == nil {
			return nil, ErrInvalidSignature
		}

		if !d.verifySignature(req) {
			return nil, ErrInvalidSignature
		}
	}

	// 3. create claim record
	timestamp := time.Now()
	record := &ClaimRecord{
		Address:     req.Address,
		GitHubID:    req.GitHubID,
		GitHubLogin: req.GitHubLogin,
		Email:       req.Email,
		DeviceID:    req.DeviceID,
		IPAddress:   req.IPAddress,
		Amount:      amount,
		Score:       req.Score,
		Timestamp:   timestamp,
		Signature:   req.SignatureHex,
	}

	// 4. record signature (if required)
	if d.config.RequireSignature {
		sigHex := hex.EncodeToString(req.Signature)
		record.Signature = sigHex
	}

	// 5. updatestatus
	d.claimedAddresses[req.Address] = record
	d.claimedGitHubIDs[req.GitHubID] = record
	d.distributedAmount += amount.Total

	// 6. persist
	if d.storage != nil {
		if err := d.storage.SaveClaim(record); err != nil {
			// rollback
			delete(d.claimedAddresses, req.Address)
			delete(d.claimedGitHubIDs, req.GitHubID)
			d.distributedAmount -= amount.Total
			return nil, fmt.Errorf("save claim: %w", err)
		}
	}

	return record, nil
}

// ClaimRequest claim request
type ClaimRequest struct {
	Address      string `json:"address"`
	GitHubID     uint64 `json:"github_id"`
	GitHubLogin  string `json:"github_login"`
	Email        string `json:"email,omitempty"`
	DeviceID     string `json:"device_id"`
	IPAddress    string `json:"ip_address"`
	Score        int    `json:"score"`
	Timestamp    int64  `json:"timestamp"`
	Signature    []byte `json:"signature,omitempty"`
	SignatureHex string `json:"signature_hex,omitempty"`
}

// verifySignature verifysign
func (d *Distributor) verifySignature(req *ClaimRequest) bool {
	message := fmt.Sprintf("%s:%d:%d", req.Address, req.CalculateAmount(req.Score, 100).Total, req.Timestamp)

	signature := req.Signature
	if signature == nil && req.SignatureHex != "" {
		// parse signature from hex
		var err error
		signature, err = hex.DecodeString(req.SignatureHex)
		if err != nil {
			return false
		}
	}

	if len(signature) != ed25519.SignatureSize {
		return false
	}

	return crypto.Ed25519Verify(d.claimer.PublicKey(), []byte(message), signature)
}

// CalculateAmountForRequest calculates the amount for a claim request (helper method)
func (req *ClaimRequest) CalculateAmount(score int, maxScore int) *AirdropAmount {
	// simplified amount calculation, similar to Distributor.CalculateAmount
	if score < 50 {
		return &AirdropAmount{Base: 0, Bonus: 0, Total: 0}
	}

	baseAmount := uint64(1000)
	for i := 0; i < 18; i++ {
		baseAmount *= 10
	}

	ratio := float64(score) / float64(maxScore)
	bonus := uint64(float64(baseAmount) * ratio * 5.0)

	return &AirdropAmount{
		Base:  baseAmount,
		Bonus: bonus,
		Total: baseAmount + bonus,
	}
}

// AddExcludedAddress adds an excluded address
func (d *Distributor) AddExcludedAddress(address string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.excludedAddresses[address] = true
}

// AddExcludedContract adds an excluded contract
func (d *Distributor) AddExcludedContract(address string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.excludedContracts[address] = true
}

// isContractAddress checks whether it is a contract address
func (d *Distributor) isContractAddress(address string) bool {
	return d.excludedContracts[address]
}

// GetClaim gets a claim record
func (d *Distributor) GetClaim(address string) (*ClaimRecord, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	record, ok := d.claimedAddresses[address]
	return record, ok
}

// GetClaimByGitHub gets the claim record for a GitHub ID
func (d *Distributor) GetClaimByGitHub(githubID uint64) (*ClaimRecord, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	record, ok := d.claimedGitHubIDs[githubID]
	return record, ok
}

// GetStats gets statistics
func (d *Distributor) GetStats() *DistributorStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return &DistributorStats{
		TotalClaims:       len(d.claimedAddresses),
		DistributedAmount: d.distributedAmount,
		RemainingAmount:   d.config.MaxTotalAmount - d.distributedAmount,
		Enabled:           d.config.Enabled,
	}
}

// DistributorStats distributor statistics
type DistributorStats struct {
	TotalClaims       int    `json:"total_claims"`
	DistributedAmount uint64 `json:"distributed_amount"`
	RemainingAmount   uint64 `json:"remaining_amount"`
	Enabled           bool   `json:"enabled"`
}

// SetStorage setstorage
func (d *Distributor) SetStorage(storage Storage) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.storage = storage
}

// LoadFromStorage loads from storage
func (d *Distributor) LoadFromStorage() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.storage == nil {
		return nil
	}

	// TODO: implement loading all claim records from storage
	return nil
}

// Enable enables distribution
func (d *Distributor) Enable() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.config.Enabled = true
}

// Disable disables distribution
func (d *Distributor) Disable() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.config.Enabled = false
}

// IsEnabled checks whether enabled
func (d *Distributor) IsEnabled() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.config.Enabled
}

// GetDistributedAmount gets the total distributed amount
func (d *Distributor) GetDistributedAmount() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.distributedAmount
}

// GetRemainingAmount gets the remaining amount
func (d *Distributor) GetRemainingAmount() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.distributedAmount >= d.config.MaxTotalAmount {
		return 0
	}
	return d.config.MaxTotalAmount - d.distributedAmount
}

// IsPoolEmpty checks whether the airdrop pool is empty
func (d *Distributor) IsPoolEmpty() bool {
	return d.GetRemainingAmount() == 0
}

// VerifyClaimSignature verifies a claim signature (public interface)
func (d *Distributor) VerifyClaimSignature(address string, amount uint64, timestamp int64, signature []byte) bool {
	message := fmt.Sprintf("%s:%d:%d", address, amount, timestamp)
	return crypto.Ed25519Verify(d.claimer.PublicKey(), []byte(message), signature)
}

// GetPublicKey gets the signer public key
func (d *Distributor) GetPublicKey() []byte {
	return d.claimer.PublicKey()
}

// GetPublicKeyHex gets the signer public key (hex)
func (d *Distributor) GetPublicKeyHex() string {
	return hex.EncodeToString(d.claimer.PublicKey())
}

// ExportClaims exports all claim records
func (d *Distributor) ExportClaims() ([]*ClaimRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	claims := make([]*ClaimRecord, 0, len(d.claimedAddresses))
	for _, record := range d.claimedAddresses {
		claims = append(claims, record)
	}

	return claims, nil
}

// ImportClaims imports claim records
func (d *Distributor) ImportClaims(claims []*ClaimRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, record := range claims {
		// check conflicts
		if _, exists := d.claimedAddresses[record.Address]; exists {
			return fmt.Errorf("address already claimed: %s", record.Address)
		}
		if _, exists := d.claimedGitHubIDs[record.GitHubID]; exists {
			return fmt.Errorf("github id already claimed: %d", record.GitHubID)
		}
	}

	// import
	for _, record := range claims {
		d.claimedAddresses[record.Address] = record
		d.claimedGitHubIDs[record.GitHubID] = record
		d.distributedAmount += record.Amount.Total
	}

	return nil
}

// GenerateMerkleTree generates a Merkle tree (for on-chain verification)
func (d *Distributor) GenerateMerkleTree() (*MerkleTree, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	claims := make([]*ClaimRecord, 0, len(d.claimedAddresses))
	for _, record := range d.claimedAddresses {
		claims = append(claims, record)
	}

	return NewMerkleTree(claims), nil
}

// MerkleTree Merkle tree
type MerkleTree struct {
	Root   []byte
	Leaves []*MerkleLeaf
}

// MerkleLeaf Merkle tree leaf node
type MerkleLeaf struct {
	Address string
	Amount  uint64
	Proof   [][]byte
}

// NewMerkleTree creates a Merkle tree
func NewMerkleTree(claims []*ClaimRecord) *MerkleTree {
	leaves := make([]*MerkleLeaf, len(claims))

	// hash each claim record
	hashes := make([][]byte, len(claims))
	for i, claim := range claims {
		leaf := &MerkleLeaf{
			Address: claim.Address,
			Amount:  claim.Amount.Total,
		}
		leaves[i] = leaf

		data := fmt.Sprintf("%s:%d", claim.Address, claim.Amount.Total)
		hash := sha256.Sum256([]byte(data))
		hashes[i] = hash[:]
	}

	// build the tree
	root := buildMerkleRoot(hashes)

	return &MerkleTree{
		Root:   root,
		Leaves: leaves,
	}
}

// buildMerkleRoot builds the Merkle root
func buildMerkleRoot(hashes [][]byte) []byte {
	if len(hashes) == 0 {
		return make([]byte, 32)
	}

	if len(hashes) == 1 {
		return hashes[0]
	}

	// ensure an even number of nodes
	if len(hashes)%2 != 0 {
		hashes = append(hashes, hashes[len(hashes)-1])
	}

	// next level
	nextLevel := make([][]byte, len(hashes)/2)
	for i := 0; i < len(hashes); i += 2 {
		combined := append(hashes[i], hashes[i+1]...)
		hash := sha256.Sum256(combined)
		nextLevel[i/2] = hash[:]
	}

	return buildMerkleRoot(nextLevel)
}
