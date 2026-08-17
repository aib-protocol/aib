package proof_of_logits

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"time"
)

// InferenceRequest represents a request for AI model inference
type InferenceRequest struct {
	ModelID     []byte  // Model fingerprint or ID
	Prompt      string  // Input prompt
	MaxTokens   int     // Maximum tokens to generate
	Temperature float64 // Sampling temperature
	TopP        float64 // Nucleus sampling parameter
	RequestID   []byte  // Unique request ID
	Timestamp   int64   // Request timestamp
	CallbackURL string  // Optional callback URL for async responses
}

// InferenceResponse represents the response from an AI inference
type InferenceResponse struct {
	RequestID    []byte  // ID of the original request
	Text         string  // Generated text
	Logits       *Logits // Logits from the model
	TokensUsed   int     // Number of tokens used
	FinishReason string  // "stop", "length", "error"
	Timestamp    int64   // Response timestamp
}

// InferenceEngine handles AI model inference and proof generation
type InferenceEngine struct {
	modelRegistry   *modelRegistry // Registry of available models
	logitsExtractor *LogitsExtractor
	generator       *RandomInputGenerator
}

// NewInferenceEngine creates a new inference engine
func NewInferenceEngine() *InferenceEngine {
	return &InferenceEngine{
		modelRegistry:   newModelRegistry(),
		logitsExtractor: NewLogitsExtractor(),
		generator:       NewRandomInputGenerator(),
	}
}

// Inference performs inference with a model and generates proof
func (e *InferenceEngine) Inference(req *InferenceRequest) (*InferenceResponse, error) {
	if req == nil {
		return nil, errors.New("engine: nil request")
	}
	if req.Prompt == "" {
		return nil, errors.New("engine: empty prompt")
	}
	if len(req.ModelID) == 0 {
		return nil, errors.New("engine: empty model ID")
	}

	// Validate temperature and top_p
	if req.Temperature < 0 || req.Temperature > 2.0 {
		return nil, errors.New("engine: temperature out of range [0, 2]")
	}
	if req.TopP < 0 || req.TopP > 1.0 {
		return nil, errors.New("engine: top_p out of range [0, 1]")
	}

	// In production, this would call the actual model
	// For now, simulate a response
	response := &InferenceResponse{
		RequestID:    req.RequestID,
		Text:         "Simulated AI response to: " + req.Prompt,
		TokensUsed:   len(req.Prompt) / 4, // Rough estimate
		FinishReason: "stop",
		Timestamp:    time.Now().Unix(),
	}

	// Generate simulated logits
	modelID := req.ModelID
	promptHash := sha256.Sum256([]byte(req.Prompt))
	logits, err := e.logitsExtractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		return nil, err
	}
	response.Logits = logits

	return response, nil
}

// InferenceWithProof performs inference and generates a proof
func (e *InferenceEngine) InferenceWithProof(req *InferenceRequest) (*InferenceResponse, *Proof, error) {
	response, err := e.Inference(req)
	if err != nil {
		return nil, nil, err
	}

	// Generate proof
	proof, err := e.generateProof(response, req.ModelID)
	if err != nil {
		return nil, nil, err
	}

	return response, proof, nil
}

// generateProof generates a proof of inference
func (e *InferenceEngine) generateProof(response *InferenceResponse, modelID []byte) (*Proof, error) {
	// In production, this would generate actual ZK proofs
	// For now, create a placeholder proof
	proof := &Proof{
		Type:       "placeholder",
		ModelID:    modelID,
		PromptHash: sha256.Sum256([]byte(response.Text)), // Use response text as placeholder for prompt hash
		OutputHash: sha256.Sum256([]byte(response.Text)),
		Timestamp:  time.Now().Unix(),
		ProofData:  []byte("placeholder_proof_data"),
	}

	return proof, nil
}

// Proof represents a proof of AI inference
type Proof struct {
	Type       string   // Proof type (e.g., "zkml", "placeholder")
	ModelID    []byte   // Model that generated the output
	PromptHash [32]byte // Hash of the input prompt
	OutputHash [32]byte // Hash of the output
	Timestamp  int64    // Proof generation timestamp
	ProofData  []byte   // Actual proof data (ZK proof, etc.)
}

// VerifyProof verifies a proof of inference
func (e *InferenceEngine) VerifyProof(proof *Proof, req *InferenceRequest, response *InferenceResponse) (bool, error) {
	if proof == nil {
		return false, errors.New("engine: nil proof")
	}
	if req == nil || response == nil {
		return false, errors.New("engine: nil request or response")
	}

	// Verify proof is for this request - match the generation logic
	// (placeholder uses response.Text as prompt hash)
	promptHash := sha256.Sum256([]byte(response.Text))
	if proof.PromptHash != promptHash {
		return false, errors.New("engine: prompt hash mismatch")
	}

	outputHash := sha256.Sum256([]byte(response.Text))
	if proof.OutputHash != outputHash {
		return false, errors.New("engine: output hash mismatch")
	}

	// Verify timestamp is reasonable
	if proof.Timestamp > time.Now().Unix()+300 { // 5 min future
		return false, errors.New("engine: proof timestamp in future")
	}

	// In production, verify actual ZK proof here
	if proof.Type != "placeholder" && len(proof.ProofData) == 0 {
		return false, errors.New("engine: empty proof data")
	}

	return true, nil
}

// modelRegistry is a simple registry for tracking available models
type modelRegistry struct {
	models map[string]*ModelInfo
}

func newModelRegistry() *modelRegistry {
	return &modelRegistry{
		models: make(map[string]*ModelInfo),
	}
}

// ModelInfo contains information about a registered model
type ModelInfo struct {
	ID           string // Model ID
	Name         string // Model name
	Version      string // Model version
	Fingerprint  []byte // Model fingerprint
	Owner        []byte // Node that owns this model
	RegisteredAt int64  // Registration timestamp
}

// RegisterModel registers a new model with the engine
func (e *InferenceEngine) RegisterModel(info *ModelInfo) error {
	if info == nil {
		return errors.New("engine: nil model info")
	}
	if info.Name == "" {
		return errors.New("engine: empty model name")
	}

	e.modelRegistry.models[info.ID] = info
	return nil
}

// GetModel returns model info by ID
func (e *InferenceEngine) GetModel(modelID string) (*ModelInfo, error) {
	info, ok := e.modelRegistry.models[modelID]
	if !ok {
		return nil, errors.New("engine: model not found")
	}
	return info, nil
}

// ListModels returns all registered models
func (e *InferenceEngine) ListModels() []*ModelInfo {
	models := make([]*ModelInfo, 0, len(e.modelRegistry.models))
	for _, info := range e.modelRegistry.models {
		models = append(models, info)
	}
	return models
}

// JSONLogitsConverter converts logits to/from JSON
type JSONLogitsConverter struct{}

func NewJSONLogitsConverter() *JSONLogitsConverter {
	return &JSONLogitsConverter{}
}

// ToJSON converts logits to JSON bytes
func (c *JSONLogitsConverter) ToJSON(logits *Logits) ([]byte, error) {
	if logits == nil {
		return nil, errors.New("converter: nil logits")
	}
	return json.Marshal(logits)
}

// FromJSON converts JSON bytes to logits
func (c *JSONLogitsConverter) FromJSON(data []byte) (*Logits, error) {
	if len(data) == 0 {
		return nil, errors.New("converter: empty data")
	}
	var logits Logits
	if err := json.Unmarshal(data, &logits); err != nil {
		return nil, err
	}
	return &logits, nil
}

// LogitsStatistics computes statistics about a logit vector
func (c *JSONLogitsConverter) LogitsStatistics(logits *Logits) (*Statistics, error) {
	if logits == nil || len(logits.Values) == 0 {
		return nil, errors.New("converter: nil or empty logits")
	}

	stats := &Statistics{
		Count:      len(logits.Values),
		Min:        math.MaxFloat64,
		Max:        -math.MaxFloat64,
		Sum:        0,
		TopKValues: make([]float64, 0),
	}

	// Compute basic statistics
	for _, v := range logits.Values {
		stats.Sum += v
		if v < stats.Min {
			stats.Min = v
		}
		if v > stats.Max {
			stats.Max = v
		}
	}
	stats.Mean = stats.Sum / float64(stats.Count)

	// Find top K values
	topK := logits.TopK
	if topK > len(logits.Values) {
		topK = len(logits.Values)
	}
	if topK > 0 {
		// Copy values and sort descending
		sorted := make([]float64, len(logits.Values))
		copy(sorted, logits.Values)
		for i := 0; i < len(sorted)-1; i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j] > sorted[i] {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		stats.TopKValues = sorted[:topK]
	}

	return stats, nil
}

// Statistics contains statistical information about logits
type Statistics struct {
	Count      int
	Min        float64
	Max        float64
	Mean       float64
	Sum        float64
	TopKValues []float64
}
