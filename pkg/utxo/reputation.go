// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Reputation Module (Trust Network Scoring System)
package utxo

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"
)

// ScoreContent 评分内容
type ScoreContent struct {
	TargetPubKey [32]byte // 被评分节点公钥
	Score        float64  // 0.0 - 10.0
	Reason       string   // "good_response", "bad_response", "timeout", "cheating"
	Timestamp    uint64   // 评分时间戳
}

// ReputationScore 评分记录（带签名）
type ReputationScore struct {
	Content   ScoreContent // 评分内容
	Signer    [32]byte     // 评分者公钥
	Signature []byte       // 评分者签名
}

// ReputationManager 管理评分
type ReputationManager struct {
	scores      map[[32]byte][]ReputationScore // 节点pubkey -> 收到的评分
	averages    map[[32]byte]float64           // 节点pubkey -> 平均分
	spamRecords map[[64]byte][]uint64          // (signer+target) -> 时间戳列表
	mu          sync.RWMutex
}

// NewReputationManager 创建新的评分管理器
func NewReputationManager() *ReputationManager {
	return &ReputationManager{
		scores:      make(map[[32]byte][]ReputationScore),
		averages:    make(map[[32]byte]float64),
		spamRecords: make(map[[64]byte][]uint64),
	}
}

// serializeContent 将ScoreContent序列化为字节
func serializeContent(content *ScoreContent) []byte {
	var buf bytes.Buffer

	buf.Write(content.TargetPubKey[:])

	// 将float64转换为固定字节表示
	scoreBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(scoreBytes, math.Float64bits(content.Score))
	buf.Write(scoreBytes)

	binary.Write(&buf, binary.BigEndian, uint32(len(content.Reason)))
	buf.Write([]byte(content.Reason))

	binary.Write(&buf, binary.BigEndian, content.Timestamp)

	return buf.Bytes()
}

// SignScore 签名评分
func SignScore(content *ScoreContent, privKey ed25519.PrivateKey) *ReputationScore {
	// 序列化内容
	data := serializeContent(content)

	// 用私钥签名
	signature := ed25519.Sign(privKey, data)

	// 获取签名者公钥
	pubKey := privKey.Public().(ed25519.PublicKey)
	var signer [32]byte
	copy(signer[:], pubKey)

	return &ReputationScore{
		Content:   *content,
		Signer:    signer,
		Signature: signature,
	}
}

// VerifyScoreSignature 验证评分签名
func VerifyScoreSignature(score *ReputationScore) bool {
	if score == nil || len(score.Signature) == 0 {
		return false
	}

	// 序列化内容
	data := serializeContent(&score.Content)

	// verifysign
	return ed25519.Verify(ed25519.PublicKey(score.Signer[:]), data, score.Signature)
}

// SubmitScore 提交评分
func (rm *ReputationManager) SubmitScore(score *ReputationScore) error {
	if score == nil {
		return fmt.Errorf("score is nil")
	}

	// verifysign
	if !VerifyScoreSignature(score) {
		return fmt.Errorf("invalid score signature")
	}

	// 验证评分范围
	if score.Content.Score < 0.0 || score.Content.Score > 10.0 {
		return fmt.Errorf("score must be between 0.0 and 10.0")
	}

	// 验证reason有效性
	validReasons := map[string]bool{
		"good_response": true,
		"bad_response":  true,
		"timeout":       true,
		"cheating":      true,
	}
	if !validReasons[score.Content.Reason] {
		return fmt.Errorf("invalid reason: %s", score.Content.Reason)
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// 记录spam检测数据
	rm.recordSpamRecord(score.Signer, score.Content.TargetPubKey, score.Content.Timestamp)

	// 添加评分
	target := score.Content.TargetPubKey
	rm.scores[target] = append(rm.scores[target], *score)

	// 重新计算平均分
	rm.recalculateAverage(target)

	return nil
}

// recalculateAverage 重新计算指定节点的平均分
func (rm *ReputationManager) recalculateAverage(target [32]byte) {
	scores := rm.scores[target]
	if len(scores) == 0 {
		rm.averages[target] = 5.0 // 默认中等分数
		return
	}

	var total float64
	for _, s := range scores {
		total += s.Content.Score
	}
	rm.averages[target] = total / float64(len(scores))
}

// GetAverageScore 获取平均分
func (rm *ReputationManager) GetAverageScore(node [32]byte) float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	score, ok := rm.averages[node]
	if !ok {
		return 5.0 // 默认中等分数
	}
	return score
}

// CalculateWeightMultiplier 计算评分乘数
// 公式: 1.0 + (score - 5.0) / 10.0
// 评分5.0 -> 乘数1.0, 评分10.0 -> 乘数1.5, 评分0.0 -> 乘数0.5
func CalculateWeightMultiplier(score float64) float64 {
	return 1.0 + (score-5.0)/10.0
}

// GetEffectiveWeight 获取有效权重
// 有效权重 = stake * CalculateWeightMultiplier(averageScore)
func (rm *ReputationManager) GetEffectiveWeight(node [32]byte, stake uint64) float64 {
	averageScore := rm.GetAverageScore(node)
	multiplier := CalculateWeightMultiplier(averageScore)
	return float64(stake) * multiplier
}

// DetectSpam 检测作弊（短时间内大量评分给同一节点）
// 如果同一个评分者在1小时内给同一目标节点评分超过10次，返回true
func (rm *ReputationManager) DetectSpam(signer [32]byte, target [32]byte) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var key [64]byte
	copy(key[:32], signer[:])
	copy(key[32:], target[:])

	timestamps, ok := rm.spamRecords[key]
	if !ok {
		return false
	}

	if len(timestamps) >= 10 {
		return true
	}

	return false
}

// recordSpamRecord 记录评分时间戳用于spam检测
func (rm *ReputationManager) recordSpamRecord(signer [32]byte, target [32]byte, timestamp uint64) {
	var key [64]byte
	copy(key[:32], signer[:])
	copy(key[32:], target[:])

	rm.spamRecords[key] = append(rm.spamRecords[key], timestamp)

	// 只保留最近1小时的时间戳（假设1小时 = 3600秒）
	cutoff := timestamp - 3600
	records := rm.spamRecords[key]
	var validRecords []uint64
	for _, t := range records {
		if t > cutoff {
			validRecords = append(validRecords, t)
		}
	}
	rm.spamRecords[key] = validRecords
}

// GetScoreCount 获取节点收到的评分数量
func (rm *ReputationManager) GetScoreCount(node [32]byte) int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.scores[node])
}

// GetScores 获取节点的所有评分
func (rm *ReputationManager) GetScores(node [32]byte) []ReputationScore {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	scores := make([]ReputationScore, len(rm.scores[node]))
	copy(scores, rm.scores[node])
	return scores
}

// GetTopValidators 获取评分最高的验证者
func (rm *ReputationManager) GetTopValidators(validators []*Validator, count int) []*Validator {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// 按平均分排序
	type scoredValidator struct {
		validator *Validator
		score     float64
	}

	var scored []scoredValidator
	for _, v := range validators {
		score := rm.averages[v.Address]
		scored = append(scored, scoredValidator{
			validator: v,
			score:     score,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]*Validator, 0, count)
	for i := 0; i < count && i < len(scored); i++ {
		result = append(result, scored[i].validator)
	}

	return result
}
