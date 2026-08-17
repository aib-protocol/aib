package proof_of_logits

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"math"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
)

// Logits represents the output logits from an AI model inference
type Logits struct {
	Values     []float64 // Logit values (pre-softmax)
	TokenIDs   []int     // Optional token IDs corresponding to logits
	TopK       int       // Top-K tokens (if applicable)
	Timestamp  int64     // Generation timestamp
	ModelID    []byte    // Model fingerprint hash
	PromptHash []byte    // Hash of the input prompt
}

// Challenge represents a random challenge for the AI to respond to
type Challenge struct {
	ID         []byte // Unique challenge ID
	Prompt     string // Input prompt for the model
	Seed       []byte // Random seed for reproducibility
	Timestamp  int64  // Challenge creation time
	Timeout    int64  // Response timeout in seconds
	LogitCount int    // Number of logits to return (0 = all)
}

// ChallengeResponse contains the AI's response to a challenge
type ChallengeResponse struct {
	ChallengeID []byte  // ID of the original challenge
	Logits      *Logits // Generated logits
	Response    string  // Text response (if applicable)
	Signature   []byte  // Node signature of the response
	Timestamp   int64   // Response timestamp
}

// RandomInputGenerator generates random input prompts for challenges
type RandomInputGenerator struct {
	wordlist []string
}

// NewRandomInputGenerator creates a new random input generator
func NewRandomInputGenerator() *RandomInputGenerator {
	// Default wordlist for test purposes
	defaultWordlist := []string{
		"hello", "world", "the", "quick", "brown", "fox",
		"jumps", "over", "lazy", "dog", "what", "is",
		"the", "meaning", "of", "life", "explain", "quantum",
		"computing", "in", "simple", "terms", "tell", "me",
		"a", "story", "about", "a", "robot", "learning",
		"to", "paint", "summarize", "the", "history", "of",
		"artificial", "intelligence", "how", "does", "machine",
		"learning", "work", "what", "are", "neural", "networks",
	}

	return &RandomInputGenerator{
		wordlist: defaultWordlist,
	}
}

// SetWordlist sets the wordlist for prompt generation
func (g *RandomInputGenerator) SetWordlist(words []string) {
	if len(words) > 0 {
		g.wordlist = words
	}
}

// GenerateRandomPrompt generates a random prompt of specified word count
func (g *RandomInputGenerator) GenerateRandomPrompt(wordCount int) (string, error) {
	if wordCount <= 0 {
		return "", errors.New("generator: word count must be positive")
	}
	if len(g.wordlist) == 0 {
		return "", errors.New("generator: empty wordlist")
	}

	prompt := ""
	for i := 0; i < wordCount; i++ {
		idx, err := randomInt(len(g.wordlist))
		if err != nil {
			return "", err
		}
		if i > 0 {
			prompt += " "
		}
		prompt += g.wordlist[idx]
	}

	return prompt, nil
}

// GenerateChallenge creates a new random challenge
func (g *RandomInputGenerator) GenerateChallenge(timeoutSeconds int64, logitCount int) (*Challenge, error) {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}

	challengeID := make([]byte, 16)
	if _, err := rand.Read(challengeID); err != nil {
		return nil, err
	}

	// Generate random prompt (3-7 words)
	wordCount, err := randomIntRange(3, 8)
	if err != nil {
		return nil, err
	}

	prompt, err := g.GenerateRandomPrompt(wordCount)
	if err != nil {
		return nil, err
	}

	return &Challenge{
		ID:         challengeID,
		Prompt:     prompt,
		Seed:       seed,
		Timestamp:  time.Now().Unix(),
		Timeout:    timeoutSeconds,
		LogitCount: logitCount,
	}, nil
}

// IsExpired checks if the challenge has expired
func (c *Challenge) IsExpired() bool {
	return time.Now().Unix() > c.Timestamp+c.Timeout
}

// Hash computes a hash of the challenge
func (c *Challenge) Hash() []byte {
	data := make([]byte, 0, len(c.ID)+len(c.Prompt)+len(c.Seed)+16)
	data = append(data, c.ID...)
	data = append(data, c.Prompt...)
	data = append(data, c.Seed...)
	data = binary.BigEndian.AppendUint64(data, uint64(c.Timestamp))
	data = binary.BigEndian.AppendUint64(data, uint64(c.Timeout))
	hash := sha256.Sum256(data)
	return hash[:]
}

// LogitsExtractor extracts logits from model outputs
type LogitsExtractor struct {
	quantizationBits int // For quantized logit representation
}

// NewLogitsExtractor creates a new logits extractor
func NewLogitsExtractor() *LogitsExtractor {
	return &LogitsExtractor{
		quantizationBits: 16, // 16-bit quantization by default
	}
}

// SetQuantization sets the quantization level
func (e *LogitsExtractor) SetQuantization(bits int) {
	if bits == 8 || bits == 16 || bits == 32 {
		e.quantizationBits = bits
	}
}

// ExtractLogits extracts logits from model output
// This is a placeholder - in real implementation, this would interface with the model
func (e *LogitsExtractor) ExtractLogits(modelOutput interface{}, modelID, promptHash []byte) (*Logits, error) {
	// Placeholder: in real implementation, this would extract actual logits from model
	// For now, we'll create a test logit vector

	logitCount := 10 // Simulate 10 logits
	logits := &Logits{
		Values:     make([]float64, logitCount),
		TokenIDs:   make([]int, logitCount),
		Timestamp:  time.Now().Unix(),
		ModelID:    modelID,
		PromptHash: promptHash,
	}

	// Generate some plausible looking logits (sorted in descending order)
	for i := 0; i < logitCount; i++ {
		// Logits typically follow a distribution where top few have positive values
		if i < 3 {
			logits.Values[i] = 5.0 - float64(i)*1.5
		} else {
			logits.Values[i] = -float64(i)
		}
		logits.TokenIDs[i] = i
	}
	logits.TopK = 5

	return logits, nil
}

// QuantizeLogits quantizes logits for efficient storage/transmission
func (e *LogitsExtractor) QuantizeLogits(logits *Logits) ([]int32, error) {
	if logits == nil || len(logits.Values) == 0 {
		return nil, errors.New("extractor: nil or empty logits")
	}

	quantized := make([]int32, len(logits.Values))
	switch e.quantizationBits {
	case 8:
		// 8-bit quantization
		minVal, maxVal := minMax(logits.Values)
		rangeVal := maxVal - minVal
		if rangeVal == 0 {
			rangeVal = 1
		}
		for i, v := range logits.Values {
			normalized := (v - minVal) / rangeVal
			quantized[i] = int32(math.Round(normalized * 255))
		}
	case 16:
		// 16-bit quantization (scale to int16 range)
		for i, v := range logits.Values {
			// Simple scaling: multiply by 1000 and clamp
			scaled := v * 1000
			if scaled > float64(math.MaxInt16) {
				scaled = float64(math.MaxInt16)
			} else if scaled < float64(math.MinInt16) {
				scaled = float64(math.MinInt16)
			}
			quantized[i] = int32(math.Round(scaled))
		}
	default:
		// 32-bit: just convert to int32 (no quantization)
		for i, v := range logits.Values {
			quantized[i] = int32(math.Round(v))
		}
	}

	return quantized, nil
}

// LogitsVerifier verifies that logits were genuinely produced by a model
type LogitsVerifier struct {
	// In production, this would hold verification keys, model info, etc.
	tolerance float64 // Tolerance for floating point comparisons
}

// NewLogitsVerifier creates a new logits verifier
func NewLogitsVerifier() *LogitsVerifier {
	return &LogitsVerifier{
		tolerance: 0.001,
	}
}

// SetTolerance sets the tolerance for floating point comparisons
func (v *LogitsVerifier) SetTolerance(tol float64) {
	if tol > 0 {
		v.tolerance = tol
	}
}

// VerifyLogits checks if logits are valid
func (v *LogitsVerifier) VerifyLogits(logits *Logits, challenge *Challenge) (bool, error) {
	if logits == nil {
		return false, errors.New("verifier: nil logits")
	}
	if challenge == nil {
		return false, errors.New("verifier: nil challenge")
	}

	// Basic sanity checks
	if len(logits.Values) == 0 {
		return false, errors.New("verifier: empty logits")
	}

	// Check timestamp is not in the future
	if logits.Timestamp > time.Now().Unix()+300 { // Allow 5 min skew
		return false, errors.New("verifier: logit timestamp in future")
	}

	// Check that logits have reasonable values (not all NaN or Inf)
	hasValidValues := false
	for _, v := range logits.Values {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			hasValidValues = true
			break
		}
	}
	if !hasValidValues {
		return false, errors.New("verifier: no valid logit values")
	}

	// Check prompt hash matches challenge
	if len(challenge.Prompt) > 0 {
		promptHash := sha256.Sum256([]byte(challenge.Prompt))
		if len(logits.PromptHash) > 0 && !bytesEqual(logits.PromptHash, promptHash[:]) {
			return false, errors.New("verifier: prompt hash mismatch")
		}
	}

	return true, nil
}

// CompareLogits compares two logit vectors for similarity
// Returns similarity score (0.0 - 1.0)
func (v *LogitsVerifier) CompareLogits(logits1, logits2 *Logits) (float64, error) {
	if logits1 == nil || logits2 == nil {
		return 0, errors.New("verifier: nil logits")
	}

	if len(logits1.Values) != len(logits2.Values) {
		return 0, errors.New("verifier: logit length mismatch")
	}

	// Compute cosine similarity
	dotProduct := 0.0
	norm1 := 0.0
	norm2 := 0.0

	for i := range logits1.Values {
		v1 := logits1.Values[i]
		v2 := logits2.Values[i]
		dotProduct += v1 * v2
		norm1 += v1 * v1
		norm2 += v2 * v2
	}

	if norm1 == 0 || norm2 == 0 {
		return 0, nil
	}

	similarity := dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))

	// Normalize to 0-1 range
	return (similarity + 1) / 2, nil
}

// VerifyChallengeResponse verifies a complete challenge-response pair
func (v *LogitsVerifier) VerifyChallengeResponse(challenge *Challenge, response *ChallengeResponse, publicKey []byte) (bool, error) {
	if challenge == nil {
		return false, errors.New("verifier: nil challenge")
	}
	if response == nil {
		return false, errors.New("verifier: nil response")
	}

	// Check challenge ID matches
	if !bytesEqual(challenge.ID, response.ChallengeID) {
		return false, errors.New("verifier: challenge ID mismatch")
	}

	// Check challenge not expired
	if challenge.IsExpired() {
		return false, errors.New("verifier: challenge expired")
	}

	// Check response timestamp is within reasonable time
	if response.Timestamp < challenge.Timestamp {
		return false, errors.New("verifier: response before challenge")
	}
	if response.Timestamp > challenge.Timestamp+challenge.Timeout+300 { // 5 min grace
		return false, errors.New("verifier: response too late")
	}

	// Verify logits
	valid, err := v.VerifyLogits(response.Logits, challenge)
	if err != nil || !valid {
		return false, err
	}

	// Verify signature using Ed25519
	if len(response.Signature) == 0 {
		return false, errors.New("verifier: missing signature")
	}
	if len(publicKey) == 0 {
		return false, errors.New("verifier: missing public key")
	}

	// Hash the response data for signature verification
	sigData := make([]byte, 0, len(response.ChallengeID)+len(response.Response)+8)
	sigData = append(sigData, response.ChallengeID...)
	sigData = append(sigData, response.Response...)
	sigData = binary.BigEndian.AppendUint64(sigData, uint64(response.Timestamp))
	sigHash := sha256.Sum256(sigData)

	if !crypto.Ed25519Verify(publicKey, sigHash[:], response.Signature) {
		return false, errors.New("verifier: invalid signature")
	}

	return true, nil
}

// Helper functions

func randomInt(max int) (int, error) {
	if max <= 0 {
		return 0, errors.New("max must be positive")
	}

	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}

	val := binary.BigEndian.Uint32(b)
	return int(val % uint32(max)), nil
}

func randomIntRange(min, max int) (int, error) {
	if min >= max {
		return 0, errors.New("min must be less than max")
	}
	rangeVal := max - min
	val, err := randomInt(rangeVal)
	if err != nil {
		return 0, err
	}
	return min + val, nil
}

func minMax(values []float64) (min, max float64) {
	if len(values) == 0 {
		return 0, 0
	}
	min = values[0]
	max = values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

// bytesEqual uses constant-time comparison to prevent timing side-channel attacks
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
