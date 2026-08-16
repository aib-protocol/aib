// Package channel implements Lightning-style state channels for AIB 2.0.
// HTLC support for conditional payments with hashlocks and timelocks.
package channel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
)

// ============================================================================
// 原子交换 (AtomicSwap) 核心类型
// ============================================================================

// AtomicSwapStatus 定义原子交换的状态
type AtomicSwapStatus int

const (
	// SwapCreated - 交换已创建，等待接收方认领
	SwapCreated AtomicSwapStatus = iota
	// SwapClaimed - 接收方已通过密钥认领
	SwapClaimed
	// SwapRefunded - 超时后已退款
	SwapRefunded
	// SwapExpired - 交换已过期
	SwapExpired
)

// String 返回交换状态的字符串表示
func (s AtomicSwapStatus) String() string {
	switch s {
	case SwapCreated:
		return "CREATED"
	case SwapClaimed:
		return "CLAIMED"
	case SwapRefunded:
		return "REFUNDED"
	case SwapExpired:
		return "EXPIRED"
	default:
		return "UNKNOWN"
	}
}

// AtomicSwap 代表一个原子交换协议
// 用于在 L2 通道内实现不同资产之间的原子兑换
type AtomicSwap struct {
	ID          [32]byte       // 唯一交换ID
	SwapID      string         // 人类可读的交换ID
	Sender      interfaces.Address // 发送方
	Receiver    interfaces.Address // 接收方
	HashLock    [32]byte       // 哈希锁 SHA256(secret)
	Secret      []byte         // 密钥（仅在认领后公开）
	Amount      uint64         // 交换金额
	AssetIn     string         // 输入资产类型 (如 "AIB", "BTC", "ETH")
	AssetOut    string         // 输出资产类型
	Rate        uint64         // 汇率 (AssetOut per AssetIn * 10^8)
	TimeLock    time.Time      // 超时时间
	Status      AtomicSwapStatus // 交换状态
	ChannelID   [32]byte       // 关联的通道ID
	CreatedAt   time.Time      // 创建时间
	ClaimedAt   *time.Time     // 认领时间
	RefundedAt  *time.Time     // 退款时间
	HTLCID      [32]byte       // 关联的HTLC ID
	Initiator   interfaces.Address // 发起方（支付输入资产的一方）
	Participant interfaces.Address // 参与方（支付输出资产的一方）
	IsCrossChain bool              // 是否跨链交换
	ExternalTxID string             // 外部链交易ID（跨链用）
}

// ============================================================================
// 原子交换错误定义
// ============================================================================

var (
	ErrSwapNotFound      = errors.New("swap not found")
	ErrSwapAlreadyExists = errors.New("swap already exists")
	ErrInvalidSwapState  = errors.New("invalid swap state")
	ErrSwapExpired       = errors.New("swap has expired")
	ErrSwapNotExpired    = errors.New("swap has not expired yet")
	ErrInvalidSecret     = errors.New("invalid secret")
	ErrHashLockMismatch  = errors.New("hash lock mismatch")
	ErrInvalidAsset      = errors.New("invalid asset")
	ErrInvalidAmount     = errors.New("invalid amount")
)

// ============================================================================
// 原子交换管理器
// ============================================================================

// AtomicSwapManager 管理通道内的原子交换
type AtomicSwapManager struct {
	manager       *Manager
	swaps         map[[32]byte]*AtomicSwap // swapID -> AtomicSwap
	swapsByChannel map[[32]byte][][32]byte // channelID -> []swapID
	mu            sync.RWMutex
}

// NewAtomicSwapManager 创建新的原子交换管理器
func NewAtomicSwapManager(m *Manager) *AtomicSwapManager {
	return &AtomicSwapManager{
		manager:       m,
		swaps:         make(map[[32]byte]*AtomicSwap),
		swapsByChannel: make(map[[32]byte][][32]byte),
	}
}

// ============================================================================
// 原子交换核心方法
// ============================================================================

// CreateSwap 创建新的原子交换
// 参数:
//   - channelID: 通道ID
//   - sender: 发送方（发起交换的一方）
//   - receiver: 接收方
//   - amount: 交换金额
//   - assetIn: 输入资产类型
//   - assetOut: 输出资产类型
//   - timeLockDuration: 锁定期时长
//   - preimage: 预图像（可选，如果为nil则自动生成）
//
// 返回: 原子交换对象和密钥（preimage）
func (asm *AtomicSwapManager) CreateSwap(
	channelID [32]byte,
	sender, receiver interfaces.Address,
	amount uint64,
	assetIn, assetOut string,
	timeLockDuration time.Duration,
	preimage []byte,
) (*AtomicSwap, []byte, error) {
	asm.mu.Lock()
	defer asm.mu.Unlock()

	// 验证参数
	if amount == 0 {
		return nil, nil, ErrInvalidAmount
	}
	if assetIn == "" || assetOut == "" {
		return nil, nil, ErrInvalidAsset
	}
	if sender == receiver {
		return nil, nil, errors.New("sender and receiver cannot be the same")
	}

	// 验证通道存在
	channel, err := asm.manager.GetChannelState(channelID)
	if err != nil {
		return nil, nil, ErrChannelNotFound
	}

	// 验证发送方是通道的一方
	if sender != channel.PartyA && sender != channel.PartyB {
		return nil, nil, errors.New("sender is not a party in the channel")
	}
	if receiver != channel.PartyA && receiver != channel.PartyB {
		return nil, nil, errors.New("receiver is not a party in the channel")
	}

	// 获取或生成预图像
	var secret []byte
	var hashLock [32]byte
	if preimage != nil && len(preimage) > 0 {
		secret = preimage
		hashLock = sha256.Sum256(preimage)
	} else {
		// 自动生成随机密钥
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, nil, fmt.Errorf("failed to generate secret: %w", err)
		}
		hashLock = sha256.Sum256(secret)
	}

	// 生成唯一交换ID
	swapID, err := generateSwapID(channelID, hashLock, amount, time.Now())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate swap ID: %w", err)
	}

	// 检查交换是否已存在
	if _, exists := asm.swaps[swapID]; exists {
		return nil, nil, ErrSwapAlreadyExists
	}

	// 计算超时时间
	timeLock := time.Now().Add(timeLockDuration)

	// 创建原子交换
	swap := &AtomicSwap{
		ID:          swapID,
		SwapID:      fmt.Sprintf("swap-%x", swapID[:8]),
		Sender:      sender,
		Receiver:    receiver,
		HashLock:    hashLock,
		Amount:      amount,
		AssetIn:     assetIn,
		AssetOut:    assetOut,
		TimeLock:    timeLock,
		Status:      SwapCreated,
		ChannelID:   channelID,
		CreatedAt:   time.Now(),
		Initiator:   sender,
		Participant: receiver,
	}

	// 添加到交换映射
	asm.swaps[swapID] = swap
	asm.swapsByChannel[channelID] = append(asm.swapsByChannel[channelID], swapID)

	return swap, secret, nil
}

// ClaimSwap 使用密钥认领原子交换
// 参数:
//   - swapID: 交换ID
//   - secret: 密钥
//
// 返回: 成功认领的交换对象
func (asm *AtomicSwapManager) ClaimSwap(swapID [32]byte, secret []byte) (*AtomicSwap, error) {
	asm.mu.Lock()
	defer asm.mu.Unlock()

	// 获取交换
	swap, exists := asm.swaps[swapID]
	if !exists {
		return nil, ErrSwapNotFound
	}

	// 验证状态
	if swap.Status != SwapCreated {
		return nil, fmt.Errorf("%w: current status is %s", ErrInvalidSwapState, swap.Status)
	}

	// 验证超时
	if time.Now().After(swap.TimeLock) {
		swap.Status = SwapExpired
		return nil, ErrSwapExpired
	}

	// 验证密钥
	hashLock := sha256.Sum256(secret)
	if hashLock != swap.HashLock {
		return nil, ErrInvalidSecret
	}

	// 更新状态
	now := time.Now()
	swap.Secret = secret
	swap.Status = SwapClaimed
	swap.ClaimedAt = &now

	// 在通道内执行原子转账
	if err := asm.executeSwapInChannel(swap); err != nil {
		return nil, fmt.Errorf("failed to execute swap in channel: %w", err)
	}

	return swap, nil
}

// RefundSwap 超时后退款原子交换
// 参数:
//   - swapID: 交换ID
//
// 返回: 已退款的交换对象
func (asm *AtomicSwapManager) RefundSwap(swapID [32]byte) (*AtomicSwap, error) {
	asm.mu.Lock()
	defer asm.mu.Unlock()

	// 获取交换
	swap, exists := asm.swaps[swapID]
	if !exists {
		return nil, ErrSwapNotFound
	}

	// 验证状态
	if swap.Status != SwapCreated {
		return nil, fmt.Errorf("%w: current status is %s", ErrInvalidSwapState, swap.Status)
	}

	// 验证超时
	if time.Now().Before(swap.TimeLock) {
		return nil, fmt.Errorf("%w: time lock expires at %v", ErrSwapNotExpired, swap.TimeLock)
	}

	// 更新状态
	now := time.Now()
	swap.Status = SwapRefunded
	swap.RefundedAt = &now

	return swap, nil
}

// VerifyHash 验证密钥是否匹配哈希锁
// 参数:
//   - swapID: 交换ID
//   - secret: 待验证的密钥
//
// 返回: 是否匹配
func (asm *AtomicSwapManager) VerifyHash(swapID [32]byte, secret []byte) (bool, error) {
	asm.mu.RLock()
	defer asm.mu.RUnlock()

	swap, exists := asm.swaps[swapID]
	if !exists {
		return false, ErrSwapNotFound
	}

	hashLock := sha256.Sum256(secret)
	return hashLock == swap.HashLock, nil
}

// GetSwap 获取交换
func (asm *AtomicSwapManager) GetSwap(swapID [32]byte) (*AtomicSwap, error) {
	asm.mu.RLock()
	defer asm.mu.RUnlock()

	swap, exists := asm.swaps[swapID]
	if !exists {
		return nil, ErrSwapNotFound
	}

	// 返回副本
	swapCopy := *swap
	return &swapCopy, nil
}

// GetSwapsByChannel 获取通道的所有交换
func (asm *AtomicSwapManager) GetSwapsByChannel(channelID [32]byte) ([]*AtomicSwap, error) {
	asm.mu.RLock()
	defer asm.mu.RUnlock()

	swapIDs, exists := asm.swapsByChannel[channelID]
	if !exists {
		return []*AtomicSwap{}, nil
	}

	result := make([]*AtomicSwap, 0, len(swapIDs))
	for _, id := range swapIDs {
		if swap, exists := asm.swaps[id]; exists {
			swapCopy := *swap
			result = append(result, &swapCopy)
		}
	}

	return result, nil
}

// GetPendingSwaps 获取所有待处理的交换
func (asm *AtomicSwapManager) GetPendingSwaps() []*AtomicSwap {
	asm.mu.RLock()
	defer asm.mu.RUnlock()

	var result []*AtomicSwap
	for _, swap := range asm.swaps {
		if swap.Status == SwapCreated && time.Now().Before(swap.TimeLock) {
			swapCopy := *swap
			result = append(result, &swapCopy)
		}
	}

	return result
}

// GetExpiredSwaps 获取所有已过期的交换
func (asm *AtomicSwapManager) GetExpiredSwaps() []*AtomicSwap {
	asm.mu.RLock()
	defer asm.mu.RUnlock()

	var result []*AtomicSwap
	now := time.Now()
	for _, swap := range asm.swaps {
		if swap.Status == SwapCreated && now.After(swap.TimeLock) {
			swapCopy := *swap
			result = append(result, &swapCopy)
		}
	}

	return result
}

// executeSwapInChannel 在通道内执行原子转账
func (asm *AtomicSwapManager) executeSwapInChannel(swap *AtomicSwap) error {
	// 这里应该调用通道管理器执行实际的资金转账
	// 由于这是一个简化实现，我们假设转账已经通过 HTLC 完成
	// 在实际实现中，这里会更新通道余额或完成 HTLC
	_ = asm.manager
	return nil
}

// generateSwapID 生成唯一的交换ID
func generateSwapID(channelID [32]byte, hashLock [32]byte, amount uint64, timestamp time.Time) ([32]byte, error) {
	h := sha256.New()
	h.Write(channelID[:])
	h.Write(hashLock[:])
	h.Write(binary.BigEndian.AppendUint64(nil, amount))
	h.Write(binary.BigEndian.AppendUint64(nil, uint64(timestamp.UnixNano())))

	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return [32]byte{}, err
	}
	h.Write(nonce)

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}

// ============================================================================
// 资产交换辅助函数
// ============================================================================

// AssetInfo 代表资产信息
type AssetInfo struct {
	Symbol       string
	Name         string
	Decimals     uint8
	IsNative     bool // 是否是链原生资产
	ChainID      string
	ContractAddr string // 对于代币合约地址
}

// 预定义的资产类型
var (
	AssetAIB = AssetInfo{
		Symbol:   "AIB",
		Name:     "AIB Token",
		Decimals: 8,
		IsNative: true,
		ChainID:  "aib-mainnet",
	}
	AssetBTC = AssetInfo{
		Symbol:   "BTC",
		Name:    "Bitcoin",
		Decimals: 8,
		IsNative: true,
		ChainID:  "bitcoin",
	}
	AssetETH = AssetInfo{
		Symbol:   "ETH",
		Name:    "Ethereum",
		Decimals: 18,
		IsNative: true,
		ChainID:  "ethereum",
	}
	AssetUSDT = AssetInfo{
		Symbol:       "USDT",
		Name:         "Tether USD",
		Decimals:     6,
		IsNative:     false,
		ChainID:      "ethereum",
		ContractAddr: "0xdAC17F958D2ee523a2206206994597C13D831ec7",
	}
)

// GetAssetInfo 获取资产信息
func GetAssetInfo(symbol string) (AssetInfo, bool) {
	switch symbol {
	case "AIB":
		return AssetAIB, true
	case "BTC":
		return AssetBTC, true
	case "ETH":
		return AssetETH, true
	case "USDT":
		return AssetUSDT, true
	default:
		return AssetInfo{}, false
	}
}

// IsValidAsset 检查资产是否有效
func IsValidAsset(symbol string) bool {
	_, ok := GetAssetInfo(symbol)
	return ok
}

// ============================================================================
// HTLC 类型定义 (原有)
// ============================================================================

// HTLC represents a Hash Time-Locked Contract.
type HTLC struct {
	ChannelID    [32]byte
	ID           [32]byte
	HashLock     [32]byte
	TimeLock     time.Time
	Amount       uint64
	Sender       interfaces.Address
	Receiver     interfaces.Address
	Preimage     []byte
	State        int // 0=pending, 1=completed, 2=expired
	CreatedAt    time.Time
	CompletedAt  *time.Time
}

// HTLCStates defines the possible states of an HTLC
const (
	HTLCPending = iota
	HTLCCompleted
	HTLCExpired
)

// HTLCConfig holds HTLC-related configuration
type HTLCConfig struct {
	MinimumExpiryDuration time.Duration
	MaximumExpiryDuration time.Duration
	MinHTLCAmount         uint64
	MaxHTLCAmount         uint64
}

// NewHTLC creates a new pending HTLC.
func NewHTLC(
	channelID [32]byte,
	hashLock [32]byte,
	timeLock time.Time,
	amount uint64,
	sender, receiver interfaces.Address,
) (*HTLC, error) {
	// Validate parameters
	if amount == 0 {
		return nil, errors.New("HTLC amount must be greater than zero")
	}

	if timeLock.IsZero() {
		return nil, errors.New("time lock must be set")
	}

	if time.Now().After(timeLock) {
		return nil, errors.New("time lock must be in the future")
	}

	// Generate unique HTLC ID
	htlcID, err := generateHTLCID(channelID, hashLock, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to generate HTLC ID: %w", err)
	}

	return &HTLC{
		ChannelID:    channelID,
		ID:           htlcID,
		HashLock:     hashLock,
		TimeLock:     timeLock,
		Amount:       amount,
		Sender:       sender,
		Receiver:     receiver,
		State:        HTLCPending,
		CreatedAt:    time.Now(),
	}, nil
}

// generateHTLCID creates a unique identifier for an HTLC.
func generateHTLCID(channelID [32]byte, hashLock [32]byte, amount uint64) ([32]byte, error) {
	h := sha256.New()
	h.Write(channelID[:])
	h.Write(hashLock[:])
	binary.BigEndian.PutUint64(make([]byte, 8), amount)
	h.Write(binary.BigEndian.AppendUint64(nil, amount))

	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return [32]byte{}, err
	}
	h.Write(nonce)

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}

// GenerateHashLock creates a hash lock from a preimage.
func GenerateHashLock(preimage []byte) ([32]byte, error) {
	if len(preimage) == 0 {
		return [32]byte{}, errors.New("preimage must not be empty")
	}
	return sha256.Sum256(preimage), nil
}

// NewRandomHashLock generates a random hash lock with corresponding preimage.
func NewRandomHashLock() ([32]byte, []byte, error) {
	preimage := make([]byte, 32)
	if _, err := rand.Read(preimage); err != nil {
		return [32]byte{}, nil, err
	}
	hashLock := sha256.Sum256(preimage)
	return hashLock, preimage, nil
}

// AddHTLC adds an HTLC to a channel.
func (m *Manager) AddHTLC(channelID [32]byte, htlc *HTLC) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	channelState, exists := m.states[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	if channelState.Status != StateOpen {
		return fmt.Errorf("channel not open: status %d", channelState.Status)
	}

	// Verify HTLC is for this channel
	if htlc.ChannelID != channelID {
		return errors.New("HTLC channel ID mismatch")
	}

	// Verify sender and receiver are valid parties
	if htlc.Sender != channel.PartyA && htlc.Sender != channel.PartyB {
		return errors.New("invalid sender")
	}
	if htlc.Receiver != channel.PartyA && htlc.Receiver != channel.PartyB {
		return errors.New("invalid receiver")
	}
	if htlc.Sender == htlc.Receiver {
		return errors.New("sender and receiver cannot be the same")
	}

	// Check if HTLC already exists
	if _, exists := channelState.PendingHTLCs[htlc.ID]; exists {
		return errors.New("HTLC already exists")
	}

	// Check sender's available balance including existing pending HTLCs
	senderBalance := getAvailableBalance(m, channel, channelState, htlc.Sender)
	if htlc.Amount > senderBalance {
		return ErrInsufficientBalance
	}

	// Add to pending HTLCs
	channelState.PendingHTLCs[htlc.ID] = htlc

	return nil
}

// getAvailableBalance calculates the available balance for a party,
// considering pending HTLCs.
func getAvailableBalance(m *Manager, channel *interfaces.Channel, state *ChannelState, addr interfaces.Address) uint64 {
	if addr == channel.PartyA {
		available := channel.BalanceA
		for _, htlc := range state.PendingHTLCs {
			if htlc.Sender == channel.PartyA {
				available -= htlc.Amount
			}
		}
		if available < 0 {
			return 0
		}
		return uint64(available)
	} else if addr == channel.PartyB {
		available := channel.BalanceB
		for _, htlc := range state.PendingHTLCs {
			if htlc.Sender == channel.PartyB {
				available -= htlc.Amount
			}
		}
		if available < 0 {
			return 0
		}
		return uint64(available)
	}
	return 0
}

// GetHTLC retrieves an HTLC by ID.
func (m *Manager) GetHTLC(channelID, htlcID [32]byte) (*HTLC, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	htlc, exists := channelState.PendingHTLCs[htlcID]
	if !exists {
		return nil, errors.New("HTLC not found")
	}

	return htlc, nil
}

// GetHTLCs returns all pending HTLCs for a channel.
func (m *Manager) GetHTLCs(channelID [32]byte) ([]*HTLC, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	var htlcs []*HTLC
	for _, htlc := range channelState.PendingHTLCs {
		htlcs = append(htlcs, htlc)
	}
	return htlcs, nil
}

// CompleteHTLC completes an HTLC with the preimage.
func (m *Manager) CompleteHTLC(channelID, htlcID [32]byte, preimage []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	channelState, exists := m.states[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	htlc, exists := channelState.PendingHTLCs[htlcID]
	if !exists {
		return errors.New("HTLC not found")
	}

	if htlc.State != HTLCPending {
		return fmt.Errorf("HTLC not pending: state %d", htlc.State)
	}

	// Verify preimage matches hash lock
	calculatedHash := sha256.Sum256(preimage)
	if calculatedHash != htlc.HashLock {
		return errors.New("invalid preimage")
	}

	// Update balances
	if htlc.Sender == channel.PartyA && htlc.Receiver == channel.PartyB {
		channel.BalanceA -= htlc.Amount
		channel.BalanceB += htlc.Amount
	} else if htlc.Sender == channel.PartyB && htlc.Receiver == channel.PartyA {
		channel.BalanceB -= htlc.Amount
		channel.BalanceA += htlc.Amount
	}

	// Increment channel sequence number
	channel.Sequence++

	// Complete the HTLC
	htlc.State = HTLCCompleted
	htlc.Preimage = preimage
	now := time.Now()
	htlc.CompletedAt = &now

	// Update state hash and last update time
	channel.StateHash = computeStateHash(channel)
	channelState.LastUpdate = now

	return nil
}

// ExpireHTLC expires an HTLC that has timed out.
func (m *Manager) ExpireHTLC(channelID, htlcID [32]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	htlc, exists := channelState.PendingHTLCs[htlcID]
	if !exists {
		return errors.New("HTLC not found")
	}

	if htlc.State != HTLCPending {
		return fmt.Errorf("HTLC not pending: state %d", htlc.State)
	}

	// Check if time lock has expired
	if time.Now().Before(htlc.TimeLock) {
		return errors.New("time lock not expired")
	}

	// Mark as expired
	htlc.State = HTLCExpired
	now := time.Now()
	htlc.CompletedAt = &now

	return nil
}

// RemoveHTLC removes an HTLC from pending HTLCs.
func (m *Manager) RemoveHTLC(channelID, htlcID [32]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	delete(channelState.PendingHTLCs, htlcID)
	return nil
}

// SettleExpiredHTLCs settles all expired HTLCs for a channel.
func (m *Manager) SettleExpiredHTLCs(channelID [32]byte) ([]*HTLC, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	var expired []*HTLC
	for _, htlc := range channelState.PendingHTLCs {
		if htlc.State == HTLCPending && time.Now().After(htlc.TimeLock) {
			htlc.State = HTLCExpired
			now := time.Now()
			htlc.CompletedAt = &now
			expired = append(expired, htlc)
		}
	}

	return expired, nil
}

// CountHTLCs returns the number of pending HTLCs for a channel.
func (m *Manager) CountHTLCs(channelID [32]byte) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return 0, ErrChannelNotFound
	}

	return len(channelState.PendingHTLCs), nil
}

// GetTotalHTLCAmount returns the total pending HTLC amount for a channel.
func (m *Manager) GetTotalHTLCAmount(channelID [32]byte) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return 0, ErrChannelNotFound
	}

	var total uint64
	for _, htlc := range channelState.PendingHTLCs {
		total += htlc.Amount
	}
	return total, nil
}

// GetHTLCsBySender returns all pending HTLCs sent by a specific party.
func (m *Manager) GetHTLCsBySender(channelID [32]byte, sender interfaces.Address) ([]*HTLC, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	var htlcs []*HTLC
	for _, htlc := range channelState.PendingHTLCs {
		if htlc.Sender == sender {
			htlcs = append(htlcs, htlc)
		}
	}
	return htlcs, nil
}

// GetHTLCsByReceiver returns all pending HTLCs received by a specific party.
func (m *Manager) GetHTLCsByReceiver(channelID [32]byte, receiver interfaces.Address) ([]*HTLC, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelState, exists := m.states[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	var htlcs []*HTLC
	for _, htlc := range channelState.PendingHTLCs {
		if htlc.Receiver == receiver {
			htlcs = append(htlcs, htlc)
		}
	}
	return htlcs, nil
}

// CheckHTLCExpiration checks if an HTLC is expired.
func (m *Manager) CheckHTLCExpiration(channelID, htlcID [32]byte) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	htlc, err := m.GetHTLC(channelID, htlcID)
	if err != nil {
		return false, err
	}

	return time.Now().After(htlc.TimeLock), nil
}

// RouteHTLC sets up a multi-hop HTLC route.
func (m *Manager) RouteHTLC(
	route []interfaces.Address,
	amount uint64,
	hashLock [32]byte,
	finalTimeLock time.Time,
	hopExpiry time.Duration,
) ([]*HTLC, [32]byte, error) {
	if len(route) < 2 {
		return nil, hashLock, errors.New("route must have at least two parties")
	}

	var htlcs []*HTLC
	currentTimeLock := finalTimeLock

	for i := 0; i < len(route)-1; i++ {
		sender := route[i]
		receiver := route[i+1]

		// Find channel between sender and receiver
		var channel *interfaces.Channel
		for _, ch := range m.channels {
			if (ch.PartyA == sender && ch.PartyB == receiver) ||
				(ch.PartyA == receiver && ch.PartyB == sender) {
				channel = ch
				break
			}
		}

		if channel == nil {
			return nil, hashLock, fmt.Errorf("no channel between %x and %x", sender, receiver)
		}

		// Create HTLC for this hop
		htlc, err := NewHTLC(
			channel.ID,
			hashLock,
			currentTimeLock,
			amount,
			sender,
			receiver,
		)
		if err != nil {
			return nil, hashLock, err
		}

		// Add to channel
		if err := m.AddHTLC(channel.ID, htlc); err != nil {
			return nil, hashLock, err
		}

		htlcs = append(htlcs, htlc)

		// Decrement time lock for next hop
		currentTimeLock = currentTimeLock.Add(-hopExpiry)
		if currentTimeLock.Before(time.Now()) {
			// Cleanup previously added HTLCs
			for _, addedHTLC := range htlcs {
				m.RemoveHTLC(addedHTLC.ChannelID, addedHTLC.ID)
			}
			return nil, hashLock, errors.New("time lock for intermediate hop would be in the past")
		}
	}

	return htlcs, hashLock, nil
}

// RoutePayment sets up a multi-hop payment with HTLCs.
func (m *Manager) RoutePayment(
	route []interfaces.Address,
	amount uint64,
	preimage []byte,
	expiry time.Duration,
) ([]*HTLC, [32]byte, error) {
	hashLock := sha256.Sum256(preimage)
	finalTimeLock := time.Now().Add(expiry)
	hopExpiry := time.Second * 3600 // 1 hour per hop

	return m.RouteHTLC(route, amount, hashLock, finalTimeLock, hopExpiry)
}

// CompleteRouteHTLC completes a multi-hop HTLC route.
func (m *Manager) CompleteRouteHTLC(htlcs []*HTLC, preimage []byte) error {
	for _, htlc := range htlcs {
		if err := m.CompleteHTLC(htlc.ChannelID, htlc.ID, preimage); err != nil {
			// If any HTLC fails, try to expire the ones we completed
			for _, completed := range htlcs {
				if completed != htlc { // Skip the failing one
					m.ExpireHTLC(completed.ChannelID, completed.ID)
				}
			}
			return err
		}
	}
	return nil
}

// ExpireRouteHTLC expires all HTLCs in a route.
func (m *Manager) ExpireRouteHTLC(htlcs []*HTLC) error {
	for _, htlc := range htlcs {
		if err := m.ExpireHTLC(htlc.ChannelID, htlc.ID); err != nil {
			// Continue with other HTLCs even if one fails
			continue
		}
	}
	return nil
}

// CreateAtomicSwap creates an atomic swap between two chains using HTLCs.
func (m *Manager) CreateAtomicSwap(
	sourceChain, destinationChain string,
	sourceParty, destinationParty interfaces.Address,
	amountA, amountB uint64,
	hashLock [32]byte,
	timeLock time.Time,
) (*HTLC, *HTLC, error) {
	// Find source channel
	var sourceChannel *interfaces.Channel
	for _, ch := range m.channels {
		if (ch.PartyA == sourceParty || ch.PartyB == sourceParty) &&
			isChainSupported(sourceChain, ch) {
			sourceChannel = ch
			break
		}
	}
	if sourceChannel == nil {
		return nil, nil, errors.New("no supported channel on source chain")
	}

	// Find destination channel
	var destChannel *interfaces.Channel
	for _, ch := range m.channels {
		if (ch.PartyA == destinationParty || ch.PartyB == destinationParty) &&
			isChainSupported(destinationChain, ch) {
			destChannel = ch
			break
		}
	}
	if destChannel == nil {
		return nil, nil, errors.New("no supported channel on destination chain")
	}

	// Create HTLCs for both sides
	htlcA, err := NewHTLC(
		sourceChannel.ID,
		hashLock,
		timeLock,
		amountA,
		sourceParty,
		destinationParty,
	)
	if err != nil {
		return nil, nil, err
	}

	htlcB, err := NewHTLC(
		destChannel.ID,
		hashLock,
		timeLock,
		amountB,
		destinationParty,
		sourceParty,
	)
	if err != nil {
		return nil, nil, err
	}

	// Add both HTLCs to their respective channels
	if err := m.AddHTLC(sourceChannel.ID, htlcA); err != nil {
		return nil, nil, err
	}

	if err := m.AddHTLC(destChannel.ID, htlcB); err != nil {
		if err2 := m.RemoveHTLC(htlcA.ChannelID, htlcA.ID); err2 != nil {
			// Log the error but continue, since we failed to add the second HTLC
		}
		return nil, nil, err
	}

	return htlcA, htlcB, nil
}

// isChainSupported checks if a chain is supported by a channel.
func isChainSupported(chain string, ch *interfaces.Channel) bool {
	// In a real implementation, this would check channel metadata
	// For now, we assume all channels support all chains
	_ = chain
	return true
}

// CompleteAtomicSwap completes an atomic swap by revealing the preimage.
func (m *Manager) CompleteAtomicSwap(htlcA, htlcB *HTLC, preimage []byte) error {
	// First complete both HTLCs
	err1 := m.CompleteHTLC(htlcA.ChannelID, htlcA.ID, preimage)
	err2 := m.CompleteHTLC(htlcB.ChannelID, htlcB.ID, preimage)

	if err1 != nil || err2 != nil {
		// If either fails, try to expire both
		if errExp1 := m.ExpireHTLC(htlcA.ChannelID, htlcA.ID); errExp1 != nil {
			// Log error
		}
		if errExp2 := m.ExpireHTLC(htlcB.ChannelID, htlcB.ID); errExp2 != nil {
			// Log error
		}

		if err1 != nil {
			return err1
		}
		return err2
	}

	return nil
}

// ExpireAtomicSwap expires an atomic swap.
func (m *Manager) ExpireAtomicSwap(htlcA, htlcB *HTLC) error {
	err1 := m.ExpireHTLC(htlcA.ChannelID, htlcA.ID)
	err2 := m.ExpireHTLC(htlcB.ChannelID, htlcB.ID)

	if err1 != nil || err2 != nil {
		if err1 != nil {
			return err1
		}
		return err2
	}

	return nil
}

// GetAtomicSwapStatus returns the status of an atomic swap.
func (m *Manager) GetAtomicSwapStatus(htlcA, htlcB *HTLC) (string, error) {
	statusA, err := m.GetHTLC(htlcA.ChannelID, htlcA.ID)
	if err != nil {
		return "failed", err
	}

	statusB, err := m.GetHTLC(htlcB.ChannelID, htlcB.ID)
	if err != nil {
		return "failed", err
	}

	if statusA.State == HTLCCompleted && statusB.State == HTLCCompleted {
		return "completed", nil
	} else if statusA.State == HTLCExpired && statusB.State == HTLCExpired {
		return "expired", nil
	} else if statusA.State == HTLCCompleted || statusB.State == HTLCCompleted {
		return "partial", nil
	} else {
		return "pending", nil
	}
}
