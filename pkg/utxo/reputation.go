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

// ScoreContent represents the content of a score.
type ScoreContent struct {
	TargetPubKey [32]byte // public key of the node being scored
	Score        float64  // 0.0 - 10.0
	Reason       string   // "good_response", "bad_response", "timeout", "cheating"
	Timestamp    uint64   // scoring timestamp
}

// ReputationScore is a score record (with signature).
type ReputationScore struct {
	Content   ScoreContent // score content
	Signer    [32]byte     // public key of the scorer
	Signature []byte       // scorer's signature
}

// ReputationManager manages scores.
type ReputationManager struct {
	scores      map[[32]byte][]ReputationScore // node pubkey -> received scores
	averages    map[[32]byte]float64           // node pubkey -> average score
	spamRecords map[[64]byte][]uint64          // (signer+target) -> list of timestamps
	mu          sync.RWMutex
}

// NewReputationManager creates a new reputation manager.
func NewReputationManager() *ReputationManager {
	return &ReputationManager{
		scores:      make(map[[32]byte][]ReputationScore),
		averages:    make(map[[32]byte]float64),
		spamRecords: make(map[[64]byte][]uint64),
	}
}

// serializeContent serializes ScoreContent to bytes.
func serializeContent(content *ScoreContent) []byte {
	var buf bytes.Buffer

	buf.Write(content.TargetPubKey[:])

	// convert float64 to a fixed byte representation
	scoreBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(scoreBytes, math.Float64bits(content.Score))
	buf.Write(scoreBytes)

	binary.Write(&buf, binary.BigEndian, uint32(len(content.Reason)))
	buf.Write([]byte(content.Reason))

	binary.Write(&buf, binary.BigEndian, content.Timestamp)

	return buf.Bytes()
}

// SignScore signs a score.
func SignScore(content *ScoreContent, privKey ed25519.PrivateKey) *ReputationScore {
	// serialize the content
	data := serializeContent(content)

	// sign with the private key
	signature := ed25519.Sign(privKey, data)

	// get the signer's public key
	pubKey := privKey.Public().(ed25519.PublicKey)
	var signer [32]byte
	copy(signer[:], pubKey)

	return &ReputationScore{
		Content:   *content,
		Signer:    signer,
		Signature: signature,
	}
}

// VerifyScoreSignature verifies a score signature.
func VerifyScoreSignature(score *ReputationScore) bool {
	if score == nil || len(score.Signature) == 0 {
		return false
	}

	// serialize the content
	data := serializeContent(&score.Content)

	// verifysign
	return ed25519.Verify(ed25519.PublicKey(score.Signer[:]), data, score.Signature)
}

// SubmitScore submits a score.
func (rm *ReputationManager) SubmitScore(score *ReputationScore) error {
	if score == nil {
		return fmt.Errorf("score is nil")
	}

	// verifysign
	if !VerifyScoreSignature(score) {
		return fmt.Errorf("invalid score signature")
	}

	// validate the score range
	if score.Content.Score < 0.0 || score.Content.Score > 10.0 {
		return fmt.Errorf("score must be between 0.0 and 10.0")
	}

	// validate the reason
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

	// record data for spam detection
	rm.recordSpamRecord(score.Signer, score.Content.TargetPubKey, score.Content.Timestamp)

	// add the score
	target := score.Content.TargetPubKey
	rm.scores[target] = append(rm.scores[target], *score)

	// recalculate the average score
	rm.recalculateAverage(target)

	return nil
}

// recalculateAverage recalculates the average score for the given node.
func (rm *ReputationManager) recalculateAverage(target [32]byte) {
	scores := rm.scores[target]
	if len(scores) == 0 {
		rm.averages[target] = 5.0 // default medium score
		return
	}

	var total float64
	for _, s := range scores {
		total += s.Content.Score
	}
	rm.averages[target] = total / float64(len(scores))
}

// GetAverageScore returns the average score.
func (rm *ReputationManager) GetAverageScore(node [32]byte) float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	score, ok := rm.averages[node]
	if !ok {
		return 5.0 // default medium score
	}
	return score
}

// CalculateWeightMultiplier calculates the score multiplier.
// Formula: 1.0 + (score - 5.0) / 10.0
// score 5.0 -> multiplier 1.0, score 10.0 -> multiplier 1.5, score 0.0 -> multiplier 0.5
func CalculateWeightMultiplier(score float64) float64 {
	return 1.0 + (score-5.0)/10.0
}

// GetEffectiveWeight getvalidweight
// effective weight = stake * CalculateWeightMultiplier(averageScore)
func (rm *ReputationManager) GetEffectiveWeight(node [32]byte, stake uint64) float64 {
	averageScore := rm.GetAverageScore(node)
	multiplier := CalculateWeightMultiplier(averageScore)
	return float64(stake) * multiplier
}

// DetectSpam detects spam (a flood of scores to the same node in a short time).
// Returns true if the same signer scored the same target node more than 10 times within 1 hour.
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

// recordSpamRecord records score timestamps for spam detection.
func (rm *ReputationManager) recordSpamRecord(signer [32]byte, target [32]byte, timestamp uint64) {
	var key [64]byte
	copy(key[:32], signer[:])
	copy(key[32:], target[:])

	rm.spamRecords[key] = append(rm.spamRecords[key], timestamp)

	// keep only timestamps from the last hour (assume 1 hour = 3600 seconds)
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

// GetScoreCount returns the number of scores a node has received.
func (rm *ReputationManager) GetScoreCount(node [32]byte) int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.scores[node])
}

// GetScores returns all scores of a node.
func (rm *ReputationManager) GetScores(node [32]byte) []ReputationScore {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	scores := make([]ReputationScore, len(rm.scores[node]))
	copy(scores, rm.scores[node])
	return scores
}

// GetTopValidators returns the validators with the highest scores.
func (rm *ReputationManager) GetTopValidators(validators []*Validator, count int) []*Validator {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// sort by average score
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
