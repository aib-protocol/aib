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
	// ErrAlreadyClaimed 已经认领
	ErrAlreadyClaimed = errors.New("airdrop already claimed")
	// ErrIneligible 不符合资格
	ErrIneligible = errors.New("not eligible for airdrop")
	// ErrTeamAddress 团队地址被排除
	ErrTeamAddress = errors.New("team address is excluded")
	// ErrContractAddress 合约地址被排除
	ErrContractAddress = errors.New("contract address is excluded")
	// ErrInvalidSignature 签名无效
	ErrInvalidSignature = errors.New("invalid signature")
	// ErrDistributionDisabled 分发已禁用
	ErrDistributionDisabled = errors.New("airdrop distribution is disabled")
)

// AirdropAmount 空投金额
type AirdropAmount struct {
	Base  uint64 `json:"base"`  // 基础金额
	Bonus uint64 `json:"bonus"` // 奖励金额
	Total uint64 `json:"total"` // 总金额
}

// ClaimRecord 认领记录
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

// DistributorConfig 分发器配置
type DistributorConfig struct {
	// 基础空投量
	BaseAmount uint64
	// 最大奖励倍数
	MaxBonusMultiplier float64

	// 分发启用状态
	Enabled bool

	// 签名验证要求
	RequireSignature bool

	// 最大总空投量
	MaxTotalAmount uint64

	// 单次认领最小分数
	MinClaimScore int
}

// DefaultDistributorConfig 默认分发器配置
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
		MaxBonusMultiplier: 5.0,        // 最多 5x 奖励
		Enabled:            true,
		RequireSignature:   true,
		MaxTotalAmount:     maxTotalAmount, // 1亿 tokens (1e26)
		MinClaimScore:      50,
	}
}

// Distributor 空投分发器
type Distributor struct {
	config           *DistributorConfig
	claimer          *AirdropSigner
	claimedAddresses map[string]*ClaimRecord
	claimedGitHubIDs map[uint64]*ClaimRecord
	mu               sync.RWMutex

	// 团队和合约地址排除列表
	excludedAddresses map[string]bool
	excludedContracts map[string]bool

	// 已分发总量
	distributedAmount uint64

	// 分布式存储（可选）
	storage Storage
}

// Storage 存储接口
type Storage interface {
	SaveClaim(record *ClaimRecord) error
	LoadClaim(address string) (*ClaimRecord, error)
	LoadClaimsByGitHub(githubID uint64) ([]*ClaimRecord, error)
}

// AirdropSigner 空投签名器
type AirdropSigner struct {
	signer *crypto.Ed25519Signer
}

// NewAirdropSigner 创建空投签名器
func NewAirdropSigner(seed []byte) (*AirdropSigner, error) {
	signer, err := crypto.NewEd25519SignerFromSeed(seed)
	if err != nil {
		return nil, err
	}
	return &AirdropSigner{signer: signer}, nil
}

// SignClaim 对认领请求签名
func (as *AirdropSigner) SignClaim(address string, amount uint64, timestamp int64) ([]byte, error) {
	message := fmt.Sprintf("%s:%d:%d", address, amount, timestamp)
	return as.signer.Sign([]byte(message))
}

// PublicKey 返回公钥
func (as *AirdropSigner) PublicKey() []byte {
	return as.signer.PublicKey()
}

// NewDistributor 创建分发器
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

// CalculateAmount 计算空投金额
func (d *Distributor) CalculateAmount(score int, maxScore int) *AirdropAmount {
	if score < d.config.MinClaimScore {
		return &AirdropAmount{Base: 0, Bonus: 0, Total: 0}
	}

	// 基础金额
	base := d.config.BaseAmount

	// 计算奖励倍数（基于分数比例）
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

// CanClaim 检查是否可以认领
func (d *Distributor) CanClaim(address string, githubID uint64, score int) (*AirdropAmount, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 检查分发是否启用
	if !d.config.Enabled {
		return nil, ErrDistributionDisabled
	}

	// 检查分数
	if score < d.config.MinClaimScore {
		return nil, ErrIneligible
	}

	// 检查地址是否已认领
	if _, claimed := d.claimedAddresses[address]; claimed {
		return nil, ErrAlreadyClaimed
	}

	// 检查 GitHub ID 是否已认领
	if _, claimed := d.claimedGitHubIDs[githubID]; claimed {
		return nil, ErrAlreadyClaimed
	}

	// 检查排除地址
	if d.excludedAddresses[address] {
		return nil, ErrTeamAddress
	}

	// checkcontractaddress
	if d.isContractAddress(address) {
		return nil, ErrContractAddress
	}

	// 检查总量限制
	amount := d.CalculateAmount(score, 100)
	if d.distributedAmount+amount.Total > d.config.MaxTotalAmount {
		return nil, errors.New("airdrop pool exhausted")
	}

	return amount, nil
}

// Claim 认领空投
func (d *Distributor) Claim(req *ClaimRequest) (*ClaimRecord, error) {
	// 先检查资格（不需要锁）
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

	// 3. 创建认领记录
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

	// 4. 记录签名（如果需要）
	if d.config.RequireSignature {
		sigHex := hex.EncodeToString(req.Signature)
		record.Signature = sigHex
	}

	// 5. updatestatus
	d.claimedAddresses[req.Address] = record
	d.claimedGitHubIDs[req.GitHubID] = record
	d.distributedAmount += amount.Total

	// 6. 持久化
	if d.storage != nil {
		if err := d.storage.SaveClaim(record); err != nil {
			// 回滚
			delete(d.claimedAddresses, req.Address)
			delete(d.claimedGitHubIDs, req.GitHubID)
			d.distributedAmount -= amount.Total
			return nil, fmt.Errorf("save claim: %w", err)
		}
	}

	return record, nil
}

// ClaimRequest 认领请求
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
		// 从十六进制解析签名
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

// CalculateAmountForRequest 计算认领请求的金额（辅助方法）
func (req *ClaimRequest) CalculateAmount(score int, maxScore int) *AirdropAmount {
	// 简化的金额计算，与 Distributor.CalculateAmount 类似
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

// AddExcludedAddress 添加排除地址
func (d *Distributor) AddExcludedAddress(address string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.excludedAddresses[address] = true
}

// AddExcludedContract 添加排除合约
func (d *Distributor) AddExcludedContract(address string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.excludedContracts[address] = true
}

// isContractAddress 检查是否为合约地址
func (d *Distributor) isContractAddress(address string) bool {
	return d.excludedContracts[address]
}

// GetClaim 获取认领记录
func (d *Distributor) GetClaim(address string) (*ClaimRecord, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	record, ok := d.claimedAddresses[address]
	return record, ok
}

// GetClaimByGitHub 获取 GitHub ID 的认领记录
func (d *Distributor) GetClaimByGitHub(githubID uint64) (*ClaimRecord, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	record, ok := d.claimedGitHubIDs[githubID]
	return record, ok
}

// GetStats 获取统计信息
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

// DistributorStats 分发器统计
type DistributorStats struct {
	TotalClaims       int    `json:"total_claims"`
	DistributedAmount uint64 `json:"distributed_amount"`
	RemainingAmount   uint64 `json:"remaining_amount"`
	Enabled           bool   `json:"enabled"`
}

// SetStorage 设置存储
func (d *Distributor) SetStorage(storage Storage) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.storage = storage
}

// LoadFromStorage 从存储加载
func (d *Distributor) LoadFromStorage() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.storage == nil {
		return nil
	}

	// TODO: 实现从存储加载所有认领记录
	return nil
}

// Enable 启用分发
func (d *Distributor) Enable() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.config.Enabled = true
}

// Disable 禁用分发
func (d *Distributor) Disable() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.config.Enabled = false
}

// IsEnabled 检查是否启用
func (d *Distributor) IsEnabled() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.config.Enabled
}

// GetDistributedAmount 获取已分发总量
func (d *Distributor) GetDistributedAmount() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.distributedAmount
}

// GetRemainingAmount 获取剩余量
func (d *Distributor) GetRemainingAmount() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.distributedAmount >= d.config.MaxTotalAmount {
		return 0
	}
	return d.config.MaxTotalAmount - d.distributedAmount
}

// IsPoolEmpty 检查空投池是否为空
func (d *Distributor) IsPoolEmpty() bool {
	return d.GetRemainingAmount() == 0
}

// VerifyClaimSignature 验证认领签名（公开接口）
func (d *Distributor) VerifyClaimSignature(address string, amount uint64, timestamp int64, signature []byte) bool {
	message := fmt.Sprintf("%s:%d:%d", address, amount, timestamp)
	return crypto.Ed25519Verify(d.claimer.PublicKey(), []byte(message), signature)
}

// GetPublicKey 获取签名器公钥
func (d *Distributor) GetPublicKey() []byte {
	return d.claimer.PublicKey()
}

// GetPublicKeyHex 获取签名器公钥（十六进制）
func (d *Distributor) GetPublicKeyHex() string {
	return hex.EncodeToString(d.claimer.PublicKey())
}

// ExportClaims 导出所有认领记录
func (d *Distributor) ExportClaims() ([]*ClaimRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	claims := make([]*ClaimRecord, 0, len(d.claimedAddresses))
	for _, record := range d.claimedAddresses {
		claims = append(claims, record)
	}

	return claims, nil
}

// ImportClaims 导入认领记录
func (d *Distributor) ImportClaims(claims []*ClaimRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, record := range claims {
		// 检查冲突
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

// GenerateMerkleTree 生成默克尔树（用于链上验证）
func (d *Distributor) GenerateMerkleTree() (*MerkleTree, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	claims := make([]*ClaimRecord, 0, len(d.claimedAddresses))
	for _, record := range d.claimedAddresses {
		claims = append(claims, record)
	}

	return NewMerkleTree(claims), nil
}

// MerkleTree 默克尔树
type MerkleTree struct {
	Root   []byte
	Leaves []*MerkleLeaf
}

// MerkleLeaf 默克尔树叶节点
type MerkleLeaf struct {
	Address string
	Amount  uint64
	Proof   [][]byte
}

// NewMerkleTree 创建默克尔树
func NewMerkleTree(claims []*ClaimRecord) *MerkleTree {
	leaves := make([]*MerkleLeaf, len(claims))

	// 哈希每个认领记录
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

	// 构建树
	root := buildMerkleRoot(hashes)

	return &MerkleTree{
		Root:   root,
		Leaves: leaves,
	}
}

// buildMerkleRoot 构建默克尔根
func buildMerkleRoot(hashes [][]byte) []byte {
	if len(hashes) == 0 {
		return make([]byte, 32)
	}

	if len(hashes) == 1 {
		return hashes[0]
	}

	// 确保偶数个节点
	if len(hashes)%2 != 0 {
		hashes = append(hashes, hashes[len(hashes)-1])
	}

	// 下一层
	nextLevel := make([][]byte, len(hashes)/2)
	for i := 0; i < len(hashes); i += 2 {
		combined := append(hashes[i], hashes[i+1]...)
		hash := sha256.Sum256(combined)
		nextLevel[i/2] = hash[:]
	}

	return buildMerkleRoot(nextLevel)
}
