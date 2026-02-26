package proof_of_logits

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"

	"github.com/aib-protocol/aib/core/crypto"
)

func TestRandomInputGenerator(t *testing.T) {
	generator := NewRandomInputGenerator()

	// Test: Generate random prompt
	prompt, err := generator.GenerateRandomPrompt(5)
	if err != nil {
		t.Fatalf("Failed to generate prompt: %v", err)
	}

	if prompt == "" {
		t.Error("Generated prompt is empty")
	}

	// Test: Generate challenge
	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("Failed to generate challenge: %v", err)
	}

	if challenge == nil {
		t.Fatal("Generated challenge is nil")
	}

	if len(challenge.ID) == 0 {
		t.Error("Challenge ID is empty")
	}

	if challenge.Prompt == "" {
		t.Error("Challenge prompt is empty")
	}

	if challenge.Timeout != 60 {
		t.Errorf("Challenge timeout mismatch: got %d, want 60", challenge.Timeout)
	}

	if challenge.LogitCount != 10 {
		t.Errorf("Challenge logit count mismatch: got %d, want 10", challenge.LogitCount)
	}
}

func TestChallenge(t *testing.T) {
	generator := NewRandomInputGenerator()
	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("Failed to generate challenge: %v", err)
	}

	// Test: Challenge is not expired initially
	if challenge.IsExpired() {
		t.Error("Challenge should not be expired immediately")
	}

	// Test: Challenge hash
	hash := challenge.Hash()
	if len(hash) != 32 {
		t.Errorf("Challenge hash length mismatch: got %d, want 32", len(hash))
	}

	// Same challenge should produce same hash
	hash2 := challenge.Hash()
	if !bytesEqual(hash, hash2) {
		t.Error("Same challenge should produce same hash")
	}
}

func TestLogitsExtractor(t *testing.T) {
	extractor := NewLogitsExtractor()

	// Test: Extract logits
	modelID := []byte("test_model")
	promptHash := sha256.Sum256([]byte("test prompt"))
	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("Failed to extract logits: %v", err)
	}

	if logits == nil {
		t.Fatal("Extracted logits is nil")
	}

	if len(logits.Values) == 0 {
		t.Error("Logits values are empty")
	}

	if !bytesEqual(logits.ModelID, modelID) {
		t.Error("Logits model ID mismatch")
	}

	if !bytesEqual(logits.PromptHash, promptHash[:]) {
		t.Error("Logits prompt hash mismatch")
	}

	// Test: Quantize logits
	extractor.SetQuantization(16)
	quantized, err := extractor.QuantizeLogits(logits)
	if err != nil {
		t.Fatalf("Failed to quantize logits: %v", err)
	}

	if len(quantized) != len(logits.Values) {
		t.Error("Quantized logits length mismatch")
	}
}

func TestLogitsVerifier(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	// Generate a challenge
	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("Failed to generate challenge: %v", err)
	}

	// Extract logits
	extractor := NewLogitsExtractor()
	modelID := []byte("test_model")
	promptHash := sha256.Sum256([]byte(challenge.Prompt))
	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("Failed to extract logits: %v", err)
	}

	// Test: Verify logits
	valid, err := verifier.VerifyLogits(logits, challenge)
	if err != nil {
		t.Fatalf("Failed to verify logits: %v", err)
	}
	if !valid {
		t.Error("Expected valid logits")
	}

	// Test: Verify with nil logits
	valid, err = verifier.VerifyLogits(nil, challenge)
	if err == nil || valid {
		t.Error("Expected error for nil logits")
	}

	// Test: Verify with nil challenge
	valid, err = verifier.VerifyLogits(logits, nil)
	if err == nil || valid {
		t.Error("Expected error for nil challenge")
	}
}

func TestChallengeResponse(t *testing.T) {
	generator := NewRandomInputGenerator()
	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("Failed to generate challenge: %v", err)
	}

	// Extract logits
	extractor := NewLogitsExtractor()
	modelID := []byte("test_model")
	promptHash := sha256.Sum256([]byte(challenge.Prompt))
	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("Failed to extract logits: %v", err)
	}

	// Generate Ed25519 keypair for signing
	pubKey, privKey, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	responseText := "test response"
	responseTimestamp := time.Now().Unix()

	// Create proper signature
	sigData := make([]byte, 0, len(challenge.ID)+len(responseText)+8)
	sigData = append(sigData, challenge.ID...)
	sigData = append(sigData, responseText...)
	sigData = binary.BigEndian.AppendUint64(sigData, uint64(responseTimestamp))
	sigHash := sha256.Sum256(sigData)
	signature := crypto.Ed25519Sign(privKey, sigHash[:])

	// Create response with real signature
	response := &ChallengeResponse{
		ChallengeID: challenge.ID,
		Logits:      logits,
		Response:    responseText,
		Timestamp:   responseTimestamp,
		Signature:   signature,
	}

	// Test: Verify challenge response with correct public key
	verifier := NewLogitsVerifier()
	valid, err := verifier.VerifyChallengeResponse(challenge, response, pubKey)
	if err != nil {
		t.Fatalf("Failed to verify challenge response: %v", err)
	}
	if !valid {
		t.Error("Expected valid challenge response")
	}

	// Test: Verify with wrong public key should fail
	wrongPubKey, _, err := crypto.Ed25519GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate wrong keys: %v", err)
	}
	valid, err = verifier.VerifyChallengeResponse(challenge, response, wrongPubKey)
	if err == nil || valid {
		t.Error("Expected error for wrong public key")
	}

	// Test: Verify with wrong challenge ID
	wrongChallenge := &Challenge{
		ID:        []byte("wrong_id"),
		Prompt:    "wrong prompt",
		Timestamp: time.Now().Unix(),
		Timeout:   60,
	}
	valid, err = verifier.VerifyChallengeResponse(wrongChallenge, response, pubKey)
	if err == nil || valid {
		t.Error("Expected error for wrong challenge ID")
	}

	// Test: Verify with empty signature should fail
	badResponse := &ChallengeResponse{
		ChallengeID: challenge.ID,
		Logits:      logits,
		Response:    responseText,
		Timestamp:   responseTimestamp,
		Signature:   []byte{},
	}
	valid, err = verifier.VerifyChallengeResponse(challenge, badResponse, pubKey)
	if err == nil || valid {
		t.Error("Expected error for empty signature")
	}
}

func TestCompareLogits(t *testing.T) {
	verifier := NewLogitsVerifier()
	extractor := NewLogitsExtractor()

	// Extract two sets of logits
	modelID := []byte("test_model")
	promptHash1 := sha256.Sum256([]byte("prompt1"))
	logits1, err := extractor.ExtractLogits(nil, modelID, promptHash1[:])
	if err != nil {
		t.Fatalf("Failed to extract logits: %v", err)
	}

	promptHash2 := sha256.Sum256([]byte("prompt2"))
	logits2, err := extractor.ExtractLogits(nil, modelID, promptHash2[:])
	if err != nil {
		t.Fatalf("Failed to extract logits: %v", err)
	}

	// Test: Compare logits
	similarity, err := verifier.CompareLogits(logits1, logits2)
	if err != nil {
		t.Fatalf("Failed to compare logits: %v", err)
	}

	if similarity < 0 || similarity > 1 {
		t.Errorf("Similarity out of range: %f", similarity)
	}

	// Test: Compare logits with different lengths
	logits3 := &Logits{
		Values: []float64{1.0, 2.0, 3.0},
	}
	_, err = verifier.CompareLogits(logits1, logits3)
	if err == nil {
		t.Error("Expected error for logits with different lengths")
	}
}

func TestInferenceEngine(t *testing.T) {
	engine := NewInferenceEngine()

	// Register a model
	modelInfo := &ModelInfo{
		ID:      "test_model",
		Name:    "Test Model",
		Version: "1.0.0",
	}
	err := engine.RegisterModel(modelInfo)
	if err != nil {
		t.Fatalf("Failed to register model: %v", err)
	}

	// Create inference request
	request := &InferenceRequest{
		ModelID:     []byte("test_model"),
		Prompt:      "test prompt",
		MaxTokens:   100,
		Temperature: 0.7,
		TopP:        0.9,
		RequestID:   []byte("test_request"),
		Timestamp:   time.Now().Unix(),
	}

	// Test: Perform inference
	response, err := engine.Inference(request)
	if err != nil {
		t.Fatalf("Failed to perform inference: %v", err)
	}

	if response == nil {
		t.Fatal("Inference response is nil")
	}

	if response.Text == "" {
		t.Error("Inference response text is empty")
	}

	if response.Logits == nil {
		t.Error("Inference response logits is nil")
	}

	// Test: Perform inference with proof
	response2, proof, err := engine.InferenceWithProof(request)
	if err != nil {
		t.Fatalf("Failed to perform inference with proof: %v", err)
	}

	if response2 == nil {
		t.Fatal("Inference response is nil")
	}

	if proof == nil {
		t.Error("Proof is nil")
	}

	// Test: Verify proof
	valid, err := engine.VerifyProof(proof, request, response2)
	if err != nil {
		t.Fatalf("Failed to verify proof: %v", err)
	}
	if !valid {
		t.Error("Expected valid proof")
	}
}

func TestJSONLogitsConverter(t *testing.T) {
	converter := NewJSONLogitsConverter()
	extractor := NewLogitsExtractor()

	// Extract logits
	modelID := []byte("test_model")
	promptHash := sha256.Sum256([]byte("test prompt"))
	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("Failed to extract logits: %v", err)
	}

	// Test: Convert to JSON
	jsonData, err := converter.ToJSON(logits)
	if err != nil {
		t.Fatalf("Failed to convert logits to JSON: %v", err)
	}

	if len(jsonData) == 0 {
		t.Error("JSON data is empty")
	}

	// Test: Convert from JSON
	logits2, err := converter.FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to convert JSON to logits: %v", err)
	}

	if len(logits2.Values) != len(logits.Values) {
		t.Error("Logits values length mismatch after JSON conversion")
	}

	// Test: Compute statistics
	stats, err := converter.LogitsStatistics(logits)
	if err != nil {
		t.Fatalf("Failed to compute logits statistics: %v", err)
	}

	if stats.Count != len(logits.Values) {
		t.Error("Statistics count mismatch")
	}

	if stats.Min > stats.Max {
		t.Error("Statistics min/max mismatch")
	}
}

func TestLogitsConsistencyVerifier(t *testing.T) {
	verifier := NewLogitsConsistencyVerifier()
	extractor := NewLogitsExtractor()

	// Extract logits
	modelID := []byte("test_model")
	prompt := "test prompt"
	promptHash := sha256.Sum256([]byte(prompt))
	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("Failed to extract logits: %v", err)
	}

	output := "test output"

	// Test: Verify logits to output
	valid, err := verifier.VerifyLogitsToOutput(logits, output)
	if err != nil {
		t.Fatalf("Failed to verify logits to output: %v", err)
	}
	if !valid {
		t.Error("Expected valid logits to output")
	}

	// Test: Verify logits to prompt
	valid, err = verifier.VerifyLogitsToPrompt(logits, prompt)
	if err != nil {
		t.Fatalf("Failed to verify logits to prompt: %v", err)
	}
	if !valid {
		t.Error("Expected valid logits to prompt")
	}

	// Test: Verify logits to model
	valid, err = verifier.VerifyLogitsToModel(logits, modelID)
	if err != nil {
		t.Fatalf("Failed to verify logits to model: %v", err)
	}
	if !valid {
		t.Error("Expected valid logits to model")
	}

	// Test: Full consistency check
	valid, err = verifier.VerifyFullConsistency(logits, prompt, output, modelID)
	if err != nil {
		t.Fatalf("Failed to verify full consistency: %v", err)
	}
	if !valid {
		t.Error("Expected full consistency")
	}
}

func TestLogitsSimilarityVerifier(t *testing.T) {
	verifier := NewLogitsSimilarityVerifier()
	extractor := NewLogitsExtractor()

	// Extract logits
	modelID := []byte("test_model")
	promptHash := sha256.Sum256([]byte("test prompt"))
	logits1, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("Failed to extract logits: %v", err)
	}

	// Create similar logits
	logits2 := &Logits{
		Values:     make([]float64, len(logits1.Values)),
		ModelID:    logits1.ModelID,
		PromptHash: logits1.PromptHash,
	}
	copy(logits2.Values, logits1.Values)
	// Slightly modify the values
	for i := range logits2.Values {
		logits2.Values[i] += 0.01 * float64(i)
	}

	// Test: Compute similarity
	similarity, err := verifier.ComputeSimilarity(logits1, logits2)
	if err != nil {
		t.Fatalf("Failed to compute similarity: %v", err)
	}

	if similarity < 0 || similarity > 1 {
		t.Errorf("Similarity out of range: %f", similarity)
	}

	// Test: Check if similar
	_, computedSimilarity, err := verifier.AreSimilar(logits1, logits2)
	if err != nil {
		t.Fatalf("Failed to check similarity: %v", err)
	}

	if computedSimilarity != similarity {
		t.Error("Computed similarity mismatch")
	}

	// Test: Detect copying
	referenceLogits := []*Logits{logits1, logits2}
	isCopy, maxSimilarity, idx, err := verifier.DetectCopying(logits1, referenceLogits)
	if err != nil {
		t.Fatalf("Failed to detect copying: %v", err)
	}

	if !isCopy {
		t.Error("Expected to detect copying")
	}

	if maxSimilarity < 0.95 {
		t.Error("Expected high similarity for copy detection")
	}

	if idx != 0 {
		t.Error("Expected most similar index to be 0")
	}
}

func TestVerifierCircuit(t *testing.T) {
	modelHash := []byte("model_hash")
	promptHash := []byte("prompt_hash")
	outputHash := []byte("output_hash")
	logitsHash := []byte("logits_hash")

	circuit := NewVerifierCircuit(modelHash, promptHash, outputHash, logitsHash)

	// Create a proof
	proof := &Proof{
		Type:      "placeholder",
		ModelID:   modelHash,
		Timestamp: time.Now().Unix(),
		ProofData: []byte("proof_data"),
	}

	// Test: Verify proof
	valid, err := circuit.VerifyProof(proof)
	if err != nil {
		t.Fatalf("Failed to verify proof: %v", err)
	}
	if !valid {
		t.Error("Expected valid proof")
	}

	// Test: Get parameters
	params := circuit.GetParameters()
	if params == nil {
		t.Error("Parameters are nil")
	}

	if params.LogitsCount != 10 {
		t.Errorf("Expected logits count 10, got %d", params.LogitsCount)
	}
}