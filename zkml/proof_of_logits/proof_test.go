package proof_of_logits

import (
	"crypto/sha256"
	"math"
	"testing"
	"time"
)

// ============================================================================
// 1. Proof generation tests
// ============================================================================

// TestProofGeneration_BasicInference verifies the basic flow of generating a proof via the inference engine
func TestProofGeneration_BasicInference(t *testing.T) {
	engine := NewInferenceEngine()

	req := &InferenceRequest{
		ModelID:     []byte("proof-gen-model-01"),
		Prompt:      "explain zero knowledge proofs",
		MaxTokens:   128,
		Temperature: 0.5,
		TopP:        0.9,
		RequestID:   []byte("proof-gen-req-001"),
		Timestamp:   time.Now().Unix(),
	}

	resp, proof, err := engine.InferenceWithProof(req)
	if err != nil {
		t.Fatalf("InferenceWithProof failed: %v", err)
	}

	// Verify the response is non-nil
	if resp == nil {
		t.Fatal("inference response is nil")
	}
	if resp.Text == "" {
		t.Fatal("inference response text is empty")
	}
	if resp.Logits == nil {
		t.Fatal("inference response logits is nil")
	}

	// Verify the proof structure is complete
	if proof == nil {
		t.Fatal("generated proof is nil")
	}
	if proof.Type == "" {
		t.Error("proof type is empty")
	}
	if len(proof.ModelID) == 0 {
		t.Error("proof ModelID is empty")
	}
	if proof.Timestamp == 0 {
		t.Error("proof timestamp is zero")
	}
	if len(proof.ProofData) == 0 {
		t.Error("proof data is empty")
	}

	// Verify the proof timestamp is reasonable (not more than 5 minutes in the future)
	now := time.Now().Unix()
	if proof.Timestamp > now+300 {
		t.Errorf("proof timestamp is in the future: proof=%d, now=%d", proof.Timestamp, now)
	}
	if proof.Timestamp < now-60 {
		t.Errorf("proof timestamp is too old: proof=%d, now=%d", proof.Timestamp, now)
	}
}

// TestProofGeneration_MultipleProofs verifies that each of several consecutive proofs is independent
func TestProofGeneration_MultipleProofs(t *testing.T) {
	engine := NewInferenceEngine()
	prompts := []string{
		"what is blockchain consensus",
		"how does proof of work function",
		"describe merkle tree structures",
	}

	var proofs []*Proof
	for i, prompt := range prompts {
		req := &InferenceRequest{
			ModelID:     []byte("multi-proof-model"),
			Prompt:      prompt,
			MaxTokens:   64,
			Temperature: 0.7,
			TopP:        0.9,
			RequestID:   []byte("multi-req-" + string(rune('A'+i))),
			Timestamp:   time.Now().Unix(),
		}

		_, proof, err := engine.InferenceWithProof(req)
		if err != nil {
			t.Fatalf("InferenceWithProof failed on iteration %d: %v", i+1, err)
		}
		if proof == nil {
			t.Fatalf("proof generated on iteration %d is nil", i+1)
		}
		proofs = append(proofs, proof)
	}

	// Verify the correct number of proofs were generated
	if len(proofs) != len(prompts) {
		t.Fatalf("generated proof count mismatch: got=%d, want=%d", len(proofs), len(prompts))
	}

	// Verify each proof has independent data
	for i := 0; i < len(proofs); i++ {
		if proofs[i].Type == "" {
			t.Errorf("proof %d type is empty", i+1)
		}
		if len(proofs[i].ProofData) == 0 {
			t.Errorf("proof %d data is empty", i+1)
		}
	}
}

// TestProofGeneration_WitnessGeneration tests witness data generation
func TestProofGeneration_WitnessGeneration(t *testing.T) {
	modelHash := sha256.Sum256([]byte("test-model-for-witness"))
	promptHash := sha256.Sum256([]byte("explain zero knowledge proofs"))
	outputHash := sha256.Sum256([]byte("A zero knowledge proof is..."))
	logitsHash := sha256.Sum256([]byte("logits-data-hash"))

	circuit := NewVerifierCircuit(modelHash[:], promptHash[:], outputHash[:], logitsHash[:])

	// Use LogitsExtractor to extract real logits
	extractor := NewLogitsExtractor()
	logits, err := extractor.ExtractLogits(nil, modelHash[:], promptHash[:])
	if err != nil {
		t.Fatalf("failed to extract logits: %v", err)
	}

	// Generate witness
	witness, err := circuit.GenerateWitness(logits, []byte("model-weights"))
	if err != nil {
		t.Fatalf("failed to generate witness: %v", err)
	}

	if len(witness) == 0 {
		t.Fatal("witness data is empty")
	}

	// witness should be valid JSON
	if witness[0] != '{' {
		t.Error("witness data is not valid JSON")
	}
}

// TestProofGeneration_PromptHashConsistency verifies the prompt hash in the proof matches the input
func TestProofGeneration_PromptHashConsistency(t *testing.T) {
	engine := NewInferenceEngine()

	prompt := "verify cryptographic hash consistency"
	req := &InferenceRequest{
		ModelID:     []byte("hash-consistency-model"),
		Prompt:      prompt,
		MaxTokens:   64,
		Temperature: 0.5,
		TopP:        0.9,
		RequestID:   []byte("hash-req-001"),
		Timestamp:   time.Now().Unix(),
	}

	resp, proof, err := engine.InferenceWithProof(req)
	if err != nil {
		t.Fatalf("InferenceWithProof failed: %v", err)
	}

	// Verify the proof output hash matches the hash of the response text
	expectedOutputHash := sha256.Sum256([]byte(resp.Text))
	if proof.OutputHash != expectedOutputHash {
		t.Error("proof OutputHash does not match the response text hash")
	}
}

// ============================================================================
// 2. Proof verification tests
// ============================================================================

// TestProofVerification_ValidProof verifies that a valid proof passes verification
func TestProofVerification_ValidProof(t *testing.T) {
	engine := NewInferenceEngine()

	req := &InferenceRequest{
		ModelID:     []byte("verify-model-01"),
		Prompt:      "describe distributed systems",
		MaxTokens:   128,
		Temperature: 0.7,
		TopP:        0.9,
		RequestID:   []byte("verify-req-001"),
		Timestamp:   time.Now().Unix(),
	}

	resp, proof, err := engine.InferenceWithProof(req)
	if err != nil {
		t.Fatalf("InferenceWithProof failed: %v", err)
	}

	valid, err := engine.VerifyProof(proof, req, resp)
	if err != nil {
		t.Fatalf("VerifyProof returned error: %v", err)
	}
	if !valid {
		t.Error("valid proof should pass verification")
	}
}

// TestProofVerification_CircuitVerify tests proof verification by VerifierCircuit
func TestProofVerification_CircuitVerify(t *testing.T) {
	modelHash := sha256.Sum256([]byte("circuit-verify-model"))

	circuit := NewVerifierCircuit(modelHash[:], nil, nil, nil)

	// Create a proof matching the circuit
	proof := &Proof{
		Type:      "placeholder",
		ModelID:   modelHash[:],
		Timestamp: time.Now().Unix(),
		ProofData: []byte("circuit-proof-data"),
	}

	valid, err := circuit.VerifyProof(proof)
	if err != nil {
		t.Fatalf("circuit verification failed: %v", err)
	}
	if !valid {
		t.Error("matching proof should pass circuit verification")
	}
}

// TestProofVerification_CircuitModelMismatch verifies verification fails when the model hash mismatches
func TestProofVerification_CircuitModelMismatch(t *testing.T) {
	modelHash1 := sha256.Sum256([]byte("model-alpha"))
	modelHash2 := sha256.Sum256([]byte("model-beta"))

	circuit := NewVerifierCircuit(modelHash1[:], nil, nil, nil)

	proof := &Proof{
		Type:      "zkml",
		ModelID:   modelHash2[:],
		Timestamp: time.Now().Unix(),
		ProofData: []byte("mismatched-proof-data"),
	}

	valid, err := circuit.VerifyProof(proof)
	if err == nil {
		t.Error("model hash mismatch should return an error")
	}
	if valid {
		t.Error("proof with mismatched model hash should not pass verification")
	}
}

// TestProofVerification_ConsistencyCheck tests logits consistency verification
func TestProofVerification_ConsistencyCheck(t *testing.T) {
	verifier := NewLogitsConsistencyVerifier()
	extractor := NewLogitsExtractor()

	prompt := "test consistency of logit output"
	modelID := []byte("consistency-model-01")
	promptHash := sha256.Sum256([]byte(prompt))

	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("failed to extract logits: %v", err)
	}

	// Full consistency check
	valid, err := verifier.VerifyFullConsistency(logits, prompt, "generated output text", modelID)
	if err != nil {
		t.Fatalf("full consistency verification failed: %v", err)
	}
	if !valid {
		t.Error("consistent logits should pass full verification")
	}
}

// TestProofVerification_SimilarityCheck tests similarity of logits produced from the same input
func TestProofVerification_SimilarityCheck(t *testing.T) {
	simVerifier := NewLogitsSimilarityVerifier()
	extractor := NewLogitsExtractor()

	modelID := []byte("similarity-model")
	promptHash := sha256.Sum256([]byte("similarity check prompt"))

	logits1, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("failed to extract logits1: %v", err)
	}

	// Create an identical copy of the logits
	logits2 := &Logits{
		Values:     make([]float64, len(logits1.Values)),
		TokenIDs:   make([]int, len(logits1.TokenIDs)),
		TopK:       logits1.TopK,
		Timestamp:  logits1.Timestamp,
		ModelID:    logits1.ModelID,
		PromptHash: logits1.PromptHash,
	}
	copy(logits2.Values, logits1.Values)
	copy(logits2.TokenIDs, logits1.TokenIDs)

	// Identical logits should have similarity 1.0
	similarity, err := simVerifier.ComputeSimilarity(logits1, logits2)
	if err != nil {
		t.Fatalf("failed to compute similarity: %v", err)
	}
	if similarity != 1.0 {
		t.Errorf("identical logits similarity should be 1.0, got: %f", similarity)
	}

	// Test copy detection - identical logits should be detected as a copy
	isCopy, maxSim, idx, err := simVerifier.DetectCopying(logits1, []*Logits{logits2})
	if err != nil {
		t.Fatalf("copy detection failed: %v", err)
	}
	if !isCopy {
		t.Error("identical logits should be detected as a copy")
	}
	if maxSim < 0.95 {
		t.Errorf("copy detection similarity too low: %f", maxSim)
	}
	if idx != 0 {
		t.Errorf("most similar index should be 0, got: %d", idx)
	}
}

// TestProofVerification_QuantizedLogits verifies correctness of quantized logits
func TestProofVerification_QuantizedLogits(t *testing.T) {
	extractor := NewLogitsExtractor()
	modelID := []byte("quantize-model")
	promptHash := sha256.Sum256([]byte("quantization test prompt"))

	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("failed to extract logits: %v", err)
	}

	// Test three quantization levels
	quantizationBits := []int{8, 16, 32}
	for _, bits := range quantizationBits {
		extractor.SetQuantization(bits)
		quantized, err := extractor.QuantizeLogits(logits)
		if err != nil {
			t.Fatalf("%d-bit quantization failed: %v", bits, err)
		}
		if len(quantized) != len(logits.Values) {
			t.Errorf("%d-bit quantized length mismatch: got=%d, want=%d", bits, len(quantized), len(logits.Values))
		}

		// Verify quantized values are in a reasonable range
		for j, v := range quantized {
			switch bits {
			case 8:
				if v < 0 || v > 255 {
					t.Errorf("8-bit quantized value out of range [0,255]: index=%d, value=%d", j, v)
				}
			case 16:
				if v < math.MinInt16 || v > math.MaxInt16 {
					t.Errorf("16-bit quantized value out of range: index=%d, value=%d", j, v)
				}
			}
		}
	}
}

// ============================================================================
// 3. Proof error handling tests
// ============================================================================

// TestProofError_NilProof tests error handling for a nil proof
func TestProofError_NilProof(t *testing.T) {
	engine := NewInferenceEngine()
	req := &InferenceRequest{
		ModelID:     []byte("error-model"),
		Prompt:      "test nil proof handling",
		MaxTokens:   64,
		Temperature: 0.5,
		TopP:        0.9,
		RequestID:   []byte("err-req-nil"),
		Timestamp:   time.Now().Unix(),
	}
	resp, err := engine.Inference(req)
	if err != nil {
		t.Fatalf("Inference failed: %v", err)
	}

	valid, err := engine.VerifyProof(nil, req, resp)
	if err == nil {
		t.Error("nil proof should return an error")
	}
	if valid {
		t.Error("nil proof should not pass verification")
	}
}

// TestProofError_NilRequest tests error handling for a nil request
func TestProofError_NilRequest(t *testing.T) {
	engine := NewInferenceEngine()

	proof := &Proof{
		Type:      "placeholder",
		ModelID:   []byte("some-model"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte("some-proof-data"),
	}

	valid, err := engine.VerifyProof(proof, nil, nil)
	if err == nil {
		t.Error("nil request should return an error")
	}
	if valid {
		t.Error("nil request should not pass verification")
	}
}

// TestProofError_EmptyPromptInference tests error handling for inference with an empty prompt
func TestProofError_EmptyPromptInference(t *testing.T) {
	engine := NewInferenceEngine()

	req := &InferenceRequest{
		ModelID:     []byte("error-model"),
		Prompt:      "",
		MaxTokens:   64,
		Temperature: 0.5,
		TopP:        0.9,
		RequestID:   []byte("err-empty-prompt"),
		Timestamp:   time.Now().Unix(),
	}

	_, _, err := engine.InferenceWithProof(req)
	if err == nil {
		t.Error("empty prompt should return an error")
	}
}

// TestProofError_EmptyModelID tests error handling for inference with an empty ModelID
func TestProofError_EmptyModelID(t *testing.T) {
	engine := NewInferenceEngine()

	req := &InferenceRequest{
		ModelID:     []byte{},
		Prompt:      "test empty model ID",
		MaxTokens:   64,
		Temperature: 0.5,
		TopP:        0.9,
		RequestID:   []byte("err-empty-model"),
		Timestamp:   time.Now().Unix(),
	}

	_, _, err := engine.InferenceWithProof(req)
	if err == nil {
		t.Error("empty ModelID should return an error")
	}
}

// TestProofError_InvalidTemperature tests error handling for invalid temperature parameters
func TestProofError_InvalidTemperature(t *testing.T) {
	engine := NewInferenceEngine()

	testCases := []struct {
		name        string
		temperature float64
	}{
		{"negative temperature", -0.1},
		{"temperature too high", 2.5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &InferenceRequest{
				ModelID:     []byte("temp-error-model"),
				Prompt:      "test invalid temperature",
				MaxTokens:   64,
				Temperature: tc.temperature,
				TopP:        0.9,
				RequestID:   []byte("err-temp"),
				Timestamp:   time.Now().Unix(),
			}

			_, _, err := engine.InferenceWithProof(req)
			if err == nil {
				t.Errorf("temperature %.1f should return an error", tc.temperature)
			}
		})
	}
}

// TestProofError_InvalidTopP tests error handling for invalid TopP parameters
func TestProofError_InvalidTopP(t *testing.T) {
	engine := NewInferenceEngine()

	testCases := []struct {
		name string
		topP float64
	}{
		{"negative TopP", -0.1},
		{"TopP too high", 1.5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &InferenceRequest{
				ModelID:     []byte("topp-error-model"),
				Prompt:      "test invalid top_p",
				MaxTokens:   64,
				Temperature: 0.5,
				TopP:        tc.topP,
				RequestID:   []byte("err-topp"),
				Timestamp:   time.Now().Unix(),
			}

			_, _, err := engine.InferenceWithProof(req)
			if err == nil {
				t.Errorf("TopP %.1f should return an error", tc.topP)
			}
		})
	}
}

// TestProofError_NilInferenceRequest tests error handling for a nil inference request
func TestProofError_NilInferenceRequest(t *testing.T) {
	engine := NewInferenceEngine()

	_, _, err := engine.InferenceWithProof(nil)
	if err == nil {
		t.Error("nil request should return an error")
	}
}

// TestProofError_CircuitNilProof tests circuit verification with a nil proof
func TestProofError_CircuitNilProof(t *testing.T) {
	circuit := NewVerifierCircuit(nil, nil, nil, nil)

	valid, err := circuit.VerifyProof(nil)
	if err == nil {
		t.Error("nil proof should return an error")
	}
	if valid {
		t.Error("nil proof should not pass circuit verification")
	}
}

// TestProofError_CircuitEmptyProofData tests a non-placeholder type with no proof data
func TestProofError_CircuitEmptyProofData(t *testing.T) {
	circuit := NewVerifierCircuit(nil, nil, nil, nil)

	proof := &Proof{
		Type:      "zkml",
		ModelID:   []byte("some-model"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte{}, // empty proof data
	}

	valid, err := circuit.VerifyProof(proof)
	if err == nil {
		t.Error("non-placeholder type with empty proof data should return an error")
	}
	if valid {
		t.Error("empty proof data should not pass verification")
	}
}

// TestProofError_WitnessNilLogits tests nil logits during witness generation
func TestProofError_WitnessNilLogits(t *testing.T) {
	circuit := NewVerifierCircuit(nil, nil, nil, nil)

	_, err := circuit.GenerateWitness(nil, []byte("weights"))
	if err == nil {
		t.Error("nil logits should return an error during witness generation")
	}
}

// TestProofError_QuantizeNilLogits tests quantizing nil logits
func TestProofError_QuantizeNilLogits(t *testing.T) {
	extractor := NewLogitsExtractor()

	_, err := extractor.QuantizeLogits(nil)
	if err == nil {
		t.Error("nil logits should return an error during quantization")
	}
}

// TestProofError_QuantizeEmptyLogits tests quantizing empty logits
func TestProofError_QuantizeEmptyLogits(t *testing.T) {
	extractor := NewLogitsExtractor()

	emptyLogits := &Logits{Values: []float64{}}
	_, err := extractor.QuantizeLogits(emptyLogits)
	if err == nil {
		t.Error("empty logits should return an error during quantization")
	}
}

// TestProofError_ConsistencyNilLogits tests consistency verification with nil logits
func TestProofError_ConsistencyNilLogits(t *testing.T) {
	verifier := NewLogitsConsistencyVerifier()

	testCases := []struct {
		name   string
		logits *Logits
		prompt string
		output string
		model  []byte
	}{
		{"nil logits to output", nil, "", "output", nil},
		{"nil logits to prompt", nil, "prompt", "", nil},
		{"nil logits to model", nil, "", "", []byte("model")},
		{"empty prompt", &Logits{Values: []float64{1.0}}, "", "output", nil},
		{"empty output", &Logits{Values: []float64{1.0}}, "prompt", "", nil},
		{"empty model", &Logits{Values: []float64{1.0}}, "", "", []byte{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.logits == nil && tc.output != "" {
				_, err := verifier.VerifyLogitsToOutput(tc.logits, tc.output)
				if err == nil {
					t.Error("should return an error")
				}
			}
			if tc.logits == nil && tc.prompt != "" {
				_, err := verifier.VerifyLogitsToPrompt(tc.logits, tc.prompt)
				if err == nil {
					t.Error("should return an error")
				}
			}
			if tc.logits == nil && len(tc.model) > 0 {
				_, err := verifier.VerifyLogitsToModel(tc.logits, tc.model)
				if err == nil {
					t.Error("should return an error")
				}
			}
			if tc.logits != nil && tc.prompt == "" && tc.output != "" {
				_, err := verifier.VerifyLogitsToOutput(tc.logits, tc.output)
				// Here logits have values but only one element, which may fail the "all same" check
				_ = err
			}
			if tc.logits != nil && tc.output == "" && tc.prompt != "" {
				_, err := verifier.VerifyLogitsToPrompt(tc.logits, tc.prompt)
				_ = err
			}
			if tc.logits != nil && len(tc.model) == 0 {
				_, err := verifier.VerifyLogitsToModel(tc.logits, tc.model)
				if err == nil {
					t.Error("empty modelID should return an error")
				}
			}
		})
	}
}

// TestProofError_SimilarityNilLogits tests similarity computation with nil logits
func TestProofError_SimilarityNilLogits(t *testing.T) {
	verifier := NewLogitsSimilarityVerifier()

	_, err := verifier.ComputeSimilarity(nil, nil)
	if err == nil {
		t.Error("nil logits should return an error during similarity computation")
	}

	logits := &Logits{Values: []float64{1.0, 2.0}}
	_, err = verifier.ComputeSimilarity(logits, nil)
	if err == nil {
		t.Error("a single nil logits should return an error")
	}

	_, err = verifier.ComputeSimilarity(nil, logits)
	if err == nil {
		t.Error("a single nil logits should return an error")
	}
}

// TestProofError_SimilarityLengthMismatch tests similarity of logits with mismatched lengths
func TestProofError_SimilarityLengthMismatch(t *testing.T) {
	verifier := NewLogitsSimilarityVerifier()

	logits1 := &Logits{Values: []float64{1.0, 2.0, 3.0}}
	logits2 := &Logits{Values: []float64{1.0, 2.0}}

	_, err := verifier.ComputeSimilarity(logits1, logits2)
	if err == nil {
		t.Error("logits with mismatched lengths should return an error")
	}
}

// TestProofError_DetectCopyingNilLogits tests nil logits in copy detection
func TestProofError_DetectCopyingNilLogits(t *testing.T) {
	verifier := NewLogitsSimilarityVerifier()

	_, _, _, err := verifier.DetectCopying(nil, []*Logits{})
	if err == nil {
		t.Error("nil logits should return an error during copy detection")
	}
}

// TestProofError_PromptHashMismatch tests verification with mismatched prompt hash
func TestProofError_PromptHashMismatch(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("failed to generate challenge: %v", err)
	}

	// Create logits but with a different prompt hash
	wrongPromptHash := sha256.Sum256([]byte("completely different prompt"))
	logits := &Logits{
		Values:     []float64{5.0, 3.5, 2.0, -1.0, -2.0},
		TokenIDs:   []int{0, 1, 2, 3, 4},
		TopK:       3,
		Timestamp:  time.Now().Unix(),
		ModelID:    []byte("test-model"),
		PromptHash: wrongPromptHash[:],
	}

	valid, err := verifier.VerifyLogits(logits, challenge)
	if err == nil || valid {
		t.Error("mismatched prompt hash should fail verification")
	}
}

// TestProofError_FutureTimestamp tests logits with a future timestamp
func TestProofError_FutureTimestamp(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("failed to generate challenge: %v", err)
	}

	promptHash := sha256.Sum256([]byte(challenge.Prompt))
	logits := &Logits{
		Values:     []float64{5.0, 3.5, 2.0, -1.0, -2.0},
		TokenIDs:   []int{0, 1, 2, 3, 4},
		TopK:       3,
		Timestamp:  time.Now().Unix() + 600, // 10 minutes ahead, exceeding the 5-minute tolerance
		ModelID:    []byte("test-model"),
		PromptHash: promptHash[:],
	}

	valid, err := verifier.VerifyLogits(logits, challenge)
	if err == nil || valid {
		t.Error("logits with a future timestamp should fail verification")
	}
}

// TestProofError_AllNaNLogits tests all-NaN logits
func TestProofError_AllNaNLogits(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("failed to generate challenge: %v", err)
	}

	logits := &Logits{
		Values:     []float64{math.NaN(), math.NaN(), math.NaN()},
		TokenIDs:   []int{0, 1, 2},
		TopK:       3,
		Timestamp:  time.Now().Unix(),
		ModelID:    []byte("test-model"),
		PromptHash: nil,
	}

	valid, err := verifier.VerifyLogits(logits, challenge)
	if err == nil || valid {
		t.Error("all-NaN logits should fail verification")
	}
}

// TestProofError_AllInfLogits tests all-Inf logits
func TestProofError_AllInfLogits(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("failed to generate challenge: %v", err)
	}

	logits := &Logits{
		Values:     []float64{math.Inf(1), math.Inf(-1), math.Inf(1)},
		TokenIDs:   []int{0, 1, 2},
		TopK:       3,
		Timestamp:  time.Now().Unix(),
		ModelID:    []byte("test-model"),
		PromptHash: nil,
	}

	valid, err := verifier.VerifyLogits(logits, challenge)
	if err == nil || valid {
		t.Error("all-Inf logits should fail verification")
	}
}

// TestProofError_EmptyLogitsVerification tests verification of empty logits
func TestProofError_EmptyLogitsVerification(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("failed to generate challenge: %v", err)
	}

	logits := &Logits{
		Values:    []float64{},
		TokenIDs:  []int{},
		Timestamp: time.Now().Unix(),
	}

	valid, err := verifier.VerifyLogits(logits, challenge)
	if err == nil || valid {
		t.Error("empty logits should fail verification")
	}
}

// ============================================================================
// 4. Proof performance tests
// ============================================================================

// TestPerformance_ProofGeneration tests proof generation performance
func TestPerformance_ProofGeneration(t *testing.T) {
	engine := NewInferenceEngine()
	iterations := 100

	req := &InferenceRequest{
		ModelID:     []byte("perf-gen-model"),
		Prompt:      "benchmark proof generation latency",
		MaxTokens:   128,
		Temperature: 0.7,
		TopP:        0.9,
		RequestID:   []byte("perf-gen-req"),
		Timestamp:   time.Now().Unix(),
	}

	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, proof, err := engine.InferenceWithProof(req)
		if err != nil {
			t.Fatalf("proof generation failed on iteration %d: %v", i+1, err)
		}
		if proof == nil {
			t.Fatalf("proof generated on iteration %d is nil", i+1)
		}
	}
	elapsed := time.Since(start)

	avgDuration := elapsed / time.Duration(iterations)
	t.Logf("Proof generation performance: %d iterations, total %v, avg %v/iter", iterations, elapsed, avgDuration)

	// A single proof generation should not exceed 100ms
	if avgDuration > 100*time.Millisecond {
		t.Errorf("proof generation average duration too long: %v (threshold 100ms)", avgDuration)
	}
}

// TestPerformance_ProofVerification tests proof verification performance
func TestPerformance_ProofVerification(t *testing.T) {
	engine := NewInferenceEngine()
	iterations := 100

	req := &InferenceRequest{
		ModelID:     []byte("perf-verify-model"),
		Prompt:      "benchmark proof verification speed",
		MaxTokens:   128,
		Temperature: 0.7,
		TopP:        0.9,
		RequestID:   []byte("perf-verify-req"),
		Timestamp:   time.Now().Unix(),
	}

	// Generate a proof first
	resp, proof, err := engine.InferenceWithProof(req)
	if err != nil {
		t.Fatalf("proof generation failed: %v", err)
	}

	start := time.Now()
	for i := 0; i < iterations; i++ {
		valid, err := engine.VerifyProof(proof, req, resp)
		if err != nil {
			t.Fatalf("verification failed on iteration %d: %v", i+1, err)
		}
		if !valid {
			t.Fatalf("verification result on iteration %d should be true", i+1)
		}
	}
	elapsed := time.Since(start)

	avgDuration := elapsed / time.Duration(iterations)
	t.Logf("Proof verification performance: %d iterations, total %v, avg %v/iter", iterations, elapsed, avgDuration)

	// A single verification should not exceed 10ms
	if avgDuration > 10*time.Millisecond {
		t.Errorf("proof verification average duration too long: %v (threshold 10ms)", avgDuration)
	}
}

// TestPerformance_BatchProofGeneration tests batch proof generation performance
func TestPerformance_BatchProofGeneration(t *testing.T) {
	engine := NewInferenceEngine()
	batchSize := 50

	prompts := make([]string, batchSize)
	for i := 0; i < batchSize; i++ {
		gen := NewRandomInputGenerator()
		prompt, err := gen.GenerateRandomPrompt(5)
		if err != nil {
			t.Fatalf("failed to generate random prompt %d: %v", i+1, err)
		}
		prompts[i] = prompt
	}

	start := time.Now()
	proofs := make([]*Proof, 0, batchSize)
	for i, prompt := range prompts {
		req := &InferenceRequest{
			ModelID:     []byte("batch-model"),
			Prompt:      prompt,
			MaxTokens:   64,
			Temperature: 0.7,
			TopP:        0.9,
			RequestID:   []byte("batch-req"),
			Timestamp:   time.Now().Unix(),
		}

		_, proof, err := engine.InferenceWithProof(req)
		if err != nil {
			t.Fatalf("batch proof generation failed on iteration %d: %v", i+1, err)
		}
		proofs = append(proofs, proof)
	}
	elapsed := time.Since(start)

	if len(proofs) != batchSize {
		t.Fatalf("batch generated proof count mismatch: got=%d, want=%d", len(proofs), batchSize)
	}

	avgDuration := elapsed / time.Duration(batchSize)
	t.Logf("Batch proof generation: %d proofs, total %v, avg %v/proof", batchSize, elapsed, avgDuration)
}

// TestPerformance_CircuitVerification tests circuit verification performance
func TestPerformance_CircuitVerification(t *testing.T) {
	modelHash := sha256.Sum256([]byte("perf-circuit-model"))
	circuit := NewVerifierCircuit(modelHash[:], nil, nil, nil)
	iterations := 500

	proof := &Proof{
		Type:      "placeholder",
		ModelID:   modelHash[:],
		Timestamp: time.Now().Unix(),
		ProofData: []byte("perf-circuit-proof-data"),
	}

	start := time.Now()
	for i := 0; i < iterations; i++ {
		valid, err := circuit.VerifyProof(proof)
		if err != nil {
			t.Fatalf("circuit verification failed on iteration %d: %v", i+1, err)
		}
		if !valid {
			t.Fatalf("circuit verification result on iteration %d should be true", i+1)
		}
	}
	elapsed := time.Since(start)

	avgDuration := elapsed / time.Duration(iterations)
	t.Logf("Circuit verification performance: %d iterations, total %v, avg %v/iter", iterations, elapsed, avgDuration)

	// Circuit verification should be very fast
	if avgDuration > 1*time.Millisecond {
		t.Errorf("circuit verification average duration too long: %v (threshold 1ms)", avgDuration)
	}
}

// TestPerformance_WitnessGeneration tests witness generation performance
func TestPerformance_WitnessGeneration(t *testing.T) {
	modelHash := sha256.Sum256([]byte("perf-witness-model"))
	promptHash := sha256.Sum256([]byte("benchmark witness generation"))
	outputHash := sha256.Sum256([]byte("witness output"))
	logitsHash := sha256.Sum256([]byte("witness logits"))

	circuit := NewVerifierCircuit(modelHash[:], promptHash[:], outputHash[:], logitsHash[:])
	extractor := NewLogitsExtractor()
	iterations := 200

	logits, err := extractor.ExtractLogits(nil, modelHash[:], promptHash[:])
	if err != nil {
		t.Fatalf("failed to extract logits: %v", err)
	}

	start := time.Now()
	for i := 0; i < iterations; i++ {
		witness, err := circuit.GenerateWitness(logits, []byte("model-weights-data"))
		if err != nil {
			t.Fatalf("witness generation failed on iteration %d: %v", i+1, err)
		}
		if len(witness) == 0 {
			t.Fatalf("witness data empty on iteration %d", i+1)
		}
	}
	elapsed := time.Since(start)

	avgDuration := elapsed / time.Duration(iterations)
	t.Logf("Witness generation performance: %d iterations, total %v, avg %v/iter", iterations, elapsed, avgDuration)

	if avgDuration > 5*time.Millisecond {
		t.Errorf("Witness generation average duration too long: %v (threshold 5ms)", avgDuration)
	}
}

// TestPerformance_LogitsSimilarity tests logits similarity computation performance
func TestPerformance_LogitsSimilarity(t *testing.T) {
	verifier := NewLogitsSimilarityVerifier()
	extractor := NewLogitsExtractor()
	iterations := 500

	modelID := []byte("perf-similarity-model")
	promptHash1 := sha256.Sum256([]byte("similarity benchmark prompt 1"))
	promptHash2 := sha256.Sum256([]byte("similarity benchmark prompt 2"))

	logits1, err := extractor.ExtractLogits(nil, modelID, promptHash1[:])
	if err != nil {
		t.Fatalf("failed to extract logits1: %v", err)
	}
	logits2, err := extractor.ExtractLogits(nil, modelID, promptHash2[:])
	if err != nil {
		t.Fatalf("failed to extract logits2: %v", err)
	}

	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, err := verifier.ComputeSimilarity(logits1, logits2)
		if err != nil {
			t.Fatalf("similarity computation failed on iteration %d: %v", i+1, err)
		}
	}
	elapsed := time.Since(start)

	avgDuration := elapsed / time.Duration(iterations)
	t.Logf("Logits similarity performance: %d iterations, total %v, avg %v/iter", iterations, elapsed, avgDuration)

	if avgDuration > 1*time.Millisecond {
		t.Errorf("Logits similarity average duration too long: %v (threshold 1ms)", avgDuration)
	}
}

// TestPerformance_Quantization tests quantization performance
func TestPerformance_Quantization(t *testing.T) {
	extractor := NewLogitsExtractor()
	iterations := 300

	modelID := []byte("perf-quant-model")
	promptHash := sha256.Sum256([]byte("quantization benchmark prompt"))
	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("failed to extract logits: %v", err)
	}

	quantBits := []int{8, 16, 32}
	for _, bits := range quantBits {
		extractor.SetQuantization(bits)

		start := time.Now()
		for i := 0; i < iterations; i++ {
			q, err := extractor.QuantizeLogits(logits)
			if err != nil {
				t.Fatalf("%d-bit quantization failed on iteration %d: %v", bits, i+1, err)
			}
			if len(q) == 0 {
				t.Fatalf("%d-bit quantization result empty on iteration %d", bits, i+1)
			}
		}
		elapsed := time.Since(start)

		avgDuration := elapsed / time.Duration(iterations)
		t.Logf("%d-bit quantization performance: %d iterations, total %v, avg %v/iter", bits, iterations, elapsed, avgDuration)
	}
}
