package slashing

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// EvidenceCollector collects and manages evidence
type EvidenceCollector struct {
	mu           sync.RWMutex
	pendingQueue []*Evidence
	verified     map[string]*Evidence
	rejected     map[string]*Evidence
	maxQueue     int
}

// NewEvidenceCollector creates a new evidence collector
func NewEvidenceCollector() *EvidenceCollector {
	return &EvidenceCollector{
		pendingQueue: make([]*Evidence, 0),
		verified:     make(map[string]*Evidence),
		rejected:     make(map[string]*Evidence),
		maxQueue:     1000,
	}
}

// Submit submits evidence for review
func (c *EvidenceCollector) Submit(evidence *Evidence) error {
	if evidence == nil {
		return errors.New("evidence: nil evidence")
	}
	if err := c.validate(evidence); err != nil {
		return fmt.Errorf("evidence: validation failed: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check queue capacity
	if len(c.pendingQueue) >= c.maxQueue {
		return errors.New("evidence: queue full")
	}

	// Generate ID if not set
	if evidence.ID == "" {
		evidence.ID = c.generateID(evidence)
	}

	// Check for duplicates
	for _, existing := range c.pendingQueue {
		if existing.ID == evidence.ID {
			return errors.New("evidence: duplicate submission")
		}
	}
	if _, exists := c.verified[evidence.ID]; exists {
		return errors.New("evidence: already verified")
	}

	c.pendingQueue = append(c.pendingQueue, evidence)
	return nil
}

// ProcessNext processes the next evidence in the queue
func (c *EvidenceCollector) ProcessNext() (*Evidence, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.pendingQueue) == 0 {
		return nil, errors.New("evidence: queue empty")
	}

	evidence := c.pendingQueue[0]
	c.pendingQueue = c.pendingQueue[1:]

	return evidence, nil
}

// Verify marks evidence as verified
func (c *EvidenceCollector) Verify(evidenceID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find in pending queue
	for i, ev := range c.pendingQueue {
		if ev.ID == evidenceID {
			c.verified[evidenceID] = ev
			c.pendingQueue = append(c.pendingQueue[:i], c.pendingQueue[i+1:]...)
			return nil
		}
	}

	return errors.New("evidence: not found in pending queue")
}

// Reject marks evidence as rejected
func (c *EvidenceCollector) Reject(evidenceID string, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, ev := range c.pendingQueue {
		if ev.ID == evidenceID {
			c.rejected[evidenceID] = ev
			c.pendingQueue = append(c.pendingQueue[:i], c.pendingQueue[i+1:]...)
			return nil
		}
	}

	return errors.New("evidence: not found in pending queue")
}

// GetPending returns all pending evidence
func (c *EvidenceCollector) GetPending() []*Evidence {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*Evidence, len(c.pendingQueue))
	copy(result, c.pendingQueue)
	return result
}

// GetVerified returns all verified evidence
func (c *EvidenceCollector) GetVerified() []*Evidence {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*Evidence, 0, len(c.verified))
	for _, ev := range c.verified {
		result = append(result, ev)
	}
	return result
}

// PendingCount returns the number of pending evidence items
func (c *EvidenceCollector) PendingCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.pendingQueue)
}

// VerifiedCount returns the number of verified evidence items
func (c *EvidenceCollector) VerifiedCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.verified)
}

// validate validates evidence
func (c *EvidenceCollector) validate(evidence *Evidence) error {
	if len(evidence.Offender) == 0 {
		return errors.New("empty offender")
	}
	if evidence.Type == "" {
		return errors.New("empty violation type")
	}
	if len(evidence.ProofData) == 0 {
		return errors.New("empty proof data")
	}
	if evidence.Timestamp == 0 {
		return errors.New("zero timestamp")
	}
	if evidence.Timestamp > time.Now().Unix()+300 {
		return errors.New("timestamp in future")
	}
	if evidence.Severity < 1 || evidence.Severity > 10 {
		return errors.New("severity must be 1-10")
	}
	return nil
}

// generateID generates a unique ID for evidence
func (c *EvidenceCollector) generateID(evidence *Evidence) string {
	data, _ := json.Marshal(struct {
		Type      ViolationType
		Offender  []byte
		Timestamp int64
		ProofData []byte
	}{
		Type:      evidence.Type,
		Offender:  evidence.Offender,
		Timestamp: evidence.Timestamp,
		ProofData: evidence.ProofData,
	})
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:16])
}

// Export exports the collector state
func (c *EvidenceCollector) Export() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	state := struct {
		PendingQueue []*Evidence
		Verified     map[string]*Evidence
		Rejected     map[string]*Evidence
	}{
		PendingQueue: c.pendingQueue,
		Verified:     c.verified,
		Rejected:     c.rejected,
	}

	return json.Marshal(state)
}

// Import imports the collector state
func (c *EvidenceCollector) Import(data []byte) error {
	var state struct {
		PendingQueue []*Evidence
		Verified     map[string]*Evidence
		Rejected     map[string]*Evidence
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pendingQueue = state.PendingQueue
	c.verified = state.Verified
	c.rejected = state.Rejected

	return nil
}

// SybilDetector detects sybil attacks by analyzing behavior patterns
type SybilDetector struct {
	mu              sync.RWMutex
	similarityThreshold float64
	nodeProfiles    map[string]*NodeProfile
}

// NodeProfile represents behavioral profile of a node
type NodeProfile struct {
	NodeID          []byte
	ResponseTimes   []float64 // Average response times
	LogitPatterns   []float64 // Logit distribution pattern
	TimingFingerprint []float64 // Timing fingerprint
	LastUpdated     int64
}

// NewSybilDetector creates a new sybil detector
func NewSybilDetector() *SybilDetector {
	return &SybilDetector{
		similarityThreshold: 0.95, // 95% similarity = suspicious
		nodeProfiles:        make(map[string]*NodeProfile),
	}
}

// UpdateProfile updates a node's behavioral profile
func (d *SybilDetector) UpdateProfile(nodeID []byte, responseTimes, logitPatterns, timingFingerprint []float64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.nodeProfiles[string(nodeID)] = &NodeProfile{
		NodeID:          nodeID,
		ResponseTimes:   responseTimes,
		LogitPatterns:   logitPatterns,
		TimingFingerprint: timingFingerprint,
		LastUpdated:     time.Now().Unix(),
	}
}

// DetectSybil checks if a node's behavior is suspiciously similar to another
func (d *SybilDetector) DetectSybil(nodeID []byte) (bool, []byte, float64) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	profile, exists := d.nodeProfiles[string(nodeID)]
	if !exists {
		return false, nil, 0
	}

	maxSimilarity := 0.0
	var mostSimilarNode []byte

	for otherID, otherProfile := range d.nodeProfiles {
		if otherID == string(nodeID) {
			continue
		}

		similarity := d.computeSimilarity(profile, otherProfile)
		if similarity > maxSimilarity {
			maxSimilarity = similarity
			mostSimilarNode = otherProfile.NodeID
		}
	}

	isSybil := maxSimilarity >= d.similarityThreshold
	return isSybil, mostSimilarNode, maxSimilarity
}

// computeSimilarity computes similarity between two node profiles
func (d *SybilDetector) computeSimilarity(a, b *NodeProfile) float64 {
	similarities := make([]float64, 0, 3)

	// Compare response times
	if len(a.ResponseTimes) > 0 && len(b.ResponseTimes) > 0 {
		sim := cosineSimilarity(a.ResponseTimes, b.ResponseTimes)
		similarities = append(similarities, sim)
	}

	// Compare logit patterns
	if len(a.LogitPatterns) > 0 && len(b.LogitPatterns) > 0 {
		sim := cosineSimilarity(a.LogitPatterns, b.LogitPatterns)
		similarities = append(similarities, sim)
	}

	// Compare timing fingerprints
	if len(a.TimingFingerprint) > 0 && len(b.TimingFingerprint) > 0 {
		sim := cosineSimilarity(a.TimingFingerprint, b.TimingFingerprint)
		similarities = append(similarities, sim)
	}

	if len(similarities) == 0 {
		return 0
	}

	// Average similarity
	total := 0.0
	for _, s := range similarities {
		total += s
	}
	return total / float64(len(similarities))
}

// cosineSimilarity computes cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	if minLen == 0 {
		return 0
	}

	dotProduct := 0.0
	normA := 0.0
	normB := 0.0

	for i := 0; i < minLen; i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return (dotProduct/(math.Sqrt(normA)*math.Sqrt(normB)) + 1) / 2
}

// GetSuspiciousPairs returns pairs of nodes with suspicious similarity
func (d *SybilDetector) GetSuspiciousPairs() [][2][]byte {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var pairs [][2][]byte
	checked := make(map[string]struct{})

	for id1, profile1 := range d.nodeProfiles {
		for id2, profile2 := range d.nodeProfiles {
			if id1 >= id2 {
				continue
			}
			pairKey := id1 + ":" + id2
			if _, exists := checked[pairKey]; exists {
				continue
			}
			checked[pairKey] = struct{}{}

			similarity := d.computeSimilarity(profile1, profile2)
			if similarity >= d.similarityThreshold {
				pairs = append(pairs, [2][]byte{profile1.NodeID, profile2.NodeID})
			}
		}
	}

	return pairs
}
