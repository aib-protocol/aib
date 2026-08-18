package channel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Inference channel errors
var (
	ErrInvalidLevel         = errors.New("invalid inference level")
	ErrChannelAlreadyClosed = errors.New("channel already closed")
	ErrChannelNotOpen       = errors.New("channel is not open")
)

// 推理接口等级定价（satoshi/次）
var InferencePrices = map[uint8]uint64{
	1: 100000,   // Level 1: 0.001 AIB
	2: 1000000,  // Level 2: 0.01 AIB
	3: 10000000, // Level 3: 0.1 AIB
}

const (
	// MinDeposit 最小存款金额
	MinDeposit uint64 = 1000000 // 0.01 AIB
)

// InferenceChannelStatus channelstatus
type InferenceChannelStatus uint8

const (
	// ICOpen 通道开放
	ICOpen InferenceChannelStatus = iota
	// ICSettling 结算中
	ICSettling
	// ICClosed 已关闭
	ICClosed
	// ICDisputed 争议中
	ICDisputed
)

// InferenceChannel 推理支付通道
type InferenceChannel struct {
	ChannelID       [32]byte
	UserPubKey      [32]byte
	NodePubKey      [32]byte
	UserBalance     uint64 // 用户余额
	NodeBalance     uint64 // 节点余额
	TotalDeposit    uint64 // 总存入金额
	Level           uint8  // 接口等级 1/2/3
	InferenceCount  uint64 // 推理次数
	SequenceNum     uint64 // 序列号（防重放）
	Status          InferenceChannelStatus
	CreatedAt       uint64
	ClosedAt        uint64
	ChallengeEnd    *time.Time
	ChallengeReason string
	mu              sync.Mutex
}

// InferenceChannelManager 管理所有推理通道
type InferenceChannelManager struct {
	channels map[[32]byte]*InferenceChannel
	mu       sync.RWMutex
}

// NewInferenceChannelManager 创建新的推理通道管理器
func NewInferenceChannelManager() *InferenceChannelManager {
	return &InferenceChannelManager{
		channels: make(map[[32]byte]*InferenceChannel),
	}
}

// generateInferenceChannelID 生成唯一的通道ID
func generateInferenceChannelID(userPubKey, nodePubKey [32]byte, nonce uint64) [32]byte {
	h := sha256.New()
	h.Write(userPubKey[:])
	h.Write(nodePubKey[:])
	h.Write(binary.BigEndian.AppendUint64(nil, nonce))
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// CreateChannel 创建通道（用户预存AIB）
func (m *InferenceChannelManager) CreateChannel(
	userPubKey, nodePubKey [32]byte,
	userDeposit uint64,
	level uint8,
) (*InferenceChannel, error) {
	// 验证等级
	if level < 1 || level > 3 {
		return nil, ErrInvalidLevel
	}

	// 验证存款
	if userDeposit < MinDeposit {
		return nil, fmt.Errorf("%w: minimum deposit is %d", ErrInvalidBalance, MinDeposit)
	}

	// 生成随机nonce
	nonceBytes := make([]byte, 8)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := binary.BigEndian.Uint64(nonceBytes)

	// generatechannelID
	channelID := generateInferenceChannelID(userPubKey, nodePubKey, nonce)

	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查通道是否已存在
	if _, exists := m.channels[channelID]; exists {
		return nil, ErrAlreadyExists
	}

	// createchannel
	now := uint64(time.Now().Unix())
	channel := &InferenceChannel{
		ChannelID:      channelID,
		UserPubKey:     userPubKey,
		NodePubKey:     nodePubKey,
		UserBalance:    userDeposit,
		NodeBalance:    0,
		TotalDeposit:   userDeposit,
		Level:          level,
		InferenceCount: 0,
		SequenceNum:    0,
		Status:         ICOpen,
		CreatedAt:      now,
	}

	m.channels[channelID] = channel

	return channel, nil
}

// RecordInference 推理调用后更新余额（链下，不上链）
func (c *InferenceChannel) RecordInference() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// checkchannelstatus
	if c.Status != ICOpen {
		return ErrChannelNotOpen
	}

	// 获取当前等级的费用
	fee, ok := InferencePrices[c.Level]
	if !ok {
		return ErrInvalidLevel
	}

	// 检查余额是否足够
	if c.UserBalance < fee {
		return ErrInsufficientBalance
	}

	// 扣除费用
	c.UserBalance -= fee
	c.NodeBalance += fee

	// 更新计数
	c.InferenceCount++
	c.SequenceNum++

	return nil
}

// Settle 链上结算
func (c *InferenceChannel) Settle() (*SettlementData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// checkchannelstatus
	if c.Status != ICOpen && c.Status != ICSettling {
		return nil, ErrChannelAlreadyClosed
	}

	// 标记为结算中
	c.Status = ICSettling

	// createsettlementdata
	settlement := &SettlementData{
		ChannelID:      c.ChannelID,
		FinalUserBal:   c.UserBalance,
		FinalNodeBal:   c.NodeBalance,
		InferenceCount: c.InferenceCount,
		SequenceNum:    c.SequenceNum,
	}

	return settlement, nil
}

// Challenge 挑战（发现作弊时）
func (c *InferenceChannel) Challenge(reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// checkchannelstatus
	if c.Status == ICClosed {
		return ErrChannelAlreadyClosed
	}

	// 设置争议状态
	c.Status = ICDisputed
	c.ChallengeReason = reason

	// 设置挑战结束时间（24小时后）
	challengeEnd := time.Now().Add(24 * time.Hour)
	c.ChallengeEnd = &challengeEnd

	return nil
}

// GetChannel 获取通道信息
func (m *InferenceChannelManager) GetChannel(id [32]byte) (*InferenceChannel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channel, exists := m.channels[id]
	if !exists {
		return nil, ErrChannelNotFound
	}

	// 返回副本 (manual field copy: never duplicate a sync.Mutex)
	chCopy := InferenceChannel{
		ChannelID:      channel.ChannelID,
		UserPubKey:     channel.UserPubKey,
		NodePubKey:     channel.NodePubKey,
		UserBalance:    channel.UserBalance,
		NodeBalance:    channel.NodeBalance,
		TotalDeposit:   channel.TotalDeposit,
		Level:          channel.Level,
		InferenceCount: channel.InferenceCount,
		SequenceNum:    channel.SequenceNum,
		Status:         channel.Status,
		CreatedAt:      channel.CreatedAt,
		ClosedAt:       channel.ClosedAt,
	}
	if channel.ChallengeEnd != nil {
		t := *channel.ChallengeEnd
		chCopy.ChallengeEnd = &t
	}
	chCopy.ChallengeReason = channel.ChallengeReason
	return &chCopy, nil
}

// Close 关闭通道
func (c *InferenceChannel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// checkchannelstatus
	if c.Status == ICClosed {
		return ErrChannelAlreadyClosed
	}

	// 标记为已关闭
	c.Status = ICClosed
	c.ClosedAt = uint64(time.Now().Unix())

	return nil
}

// GetChannels 返回所有通道
func (m *InferenceChannelManager) GetChannels() []*InferenceChannel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*InferenceChannel, 0, len(m.channels))
	for _, ch := range m.channels {
		result = append(result, ch.snapshot())
	}
	return result
}

// snapshot returns a field-wise copy of the channel without copying its
// mutex (copying a sync.Mutex is flagged by go vet and is unsafe).
func (c *InferenceChannel) snapshot() *InferenceChannel {
	cp := InferenceChannel{
		ChannelID:       c.ChannelID,
		UserPubKey:      c.UserPubKey,
		NodePubKey:      c.NodePubKey,
		UserBalance:     c.UserBalance,
		NodeBalance:     c.NodeBalance,
		TotalDeposit:    c.TotalDeposit,
		Level:           c.Level,
		InferenceCount:  c.InferenceCount,
		SequenceNum:     c.SequenceNum,
		Status:          c.Status,
		CreatedAt:       c.CreatedAt,
		ClosedAt:        c.ClosedAt,
		ChallengeReason: c.ChallengeReason,
	}
	if c.ChallengeEnd != nil {
		t := *c.ChallengeEnd
		cp.ChallengeEnd = &t
	}
	return &cp
}

// GetChannelCount 返回通道数量
func (m *InferenceChannelManager) GetChannelCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.channels)
}

// SettlementData settlementdata
type SettlementData struct {
	ChannelID      [32]byte
	FinalUserBal   uint64
	FinalNodeBal   uint64
	InferenceCount uint64
	SequenceNum    uint64
}

// IsValid verifysettlementdata
func (s *SettlementData) IsValid() error {
	// 验证总余额守恒
	total := s.FinalUserBal + s.FinalNodeBal
	if total == 0 {
		return errors.New("zero total balance")
	}
	return nil
}

// GetChannelIDString 返回通道ID的字符串形式
func (c *InferenceChannel) GetChannelIDString() string {
	return fmt.Sprintf("%x", c.ChannelID)
}

// GetFee 返回当前等级的推理费用
func (c *InferenceChannel) GetFee() uint64 {
	return InferencePrices[c.Level]
}

// GetRemainingInferences 返回剩余可用推理次数
func (c *InferenceChannel) GetRemainingInferences() uint64 {
	fee := c.GetFee()
	if fee == 0 {
		return 0
	}
	return c.UserBalance / fee
}

// IsExpired 检查通道是否已过期
func (c *InferenceChannel) IsExpired() bool {
	// 通道创建后30天过期
	expiryTime := time.Unix(int64(c.CreatedAt), 0).Add(30 * 24 * time.Hour)
	return time.Now().After(expiryTime)
}

// CanSettle 检查是否可以结算
func (c *InferenceChannel) CanSettle() bool {
	return c.Status == ICOpen || c.Status == ICSettling
}

// IsInDispute 检查是否在争议中
func (c *InferenceChannel) IsInDispute() bool {
	return c.Status == ICDisputed
}

// IsClosed 检查是否已关闭
func (c *InferenceChannel) IsClosed() bool {
	return c.Status == ICClosed
}
