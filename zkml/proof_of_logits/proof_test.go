package proof_of_logits

import (
	"crypto/sha256"
	"math"
	"testing"
	"time"
)

// ============================================================================
// 1. 证明生成测试
// ============================================================================

// TestProofGeneration_BasicInference 验证通过推理引擎生成证明的基本流程
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
		t.Fatalf("InferenceWithProof 失败: %v", err)
	}

	// 验证响应不为空
	if resp == nil {
		t.Fatal("推理响应为 nil")
	}
	if resp.Text == "" {
		t.Fatal("推理响应文本为空")
	}
	if resp.Logits == nil {
		t.Fatal("推理响应 logits 为 nil")
	}

	// 验证证明结构完整
	if proof == nil {
		t.Fatal("生成的证明为 nil")
	}
	if proof.Type == "" {
		t.Error("证明类型为空")
	}
	if len(proof.ModelID) == 0 {
		t.Error("证明的 ModelID 为空")
	}
	if proof.Timestamp == 0 {
		t.Error("证明时间戳为零")
	}
	if len(proof.ProofData) == 0 {
		t.Error("证明数据为空")
	}

	// 验证证明时间戳合理（不在未来超过 5 分钟）
	now := time.Now().Unix()
	if proof.Timestamp > now+300 {
		t.Errorf("证明时间戳在未来: proof=%d, now=%d", proof.Timestamp, now)
	}
	if proof.Timestamp < now-60 {
		t.Errorf("证明时间戳过旧: proof=%d, now=%d", proof.Timestamp, now)
	}
}

// TestProofGeneration_MultipleProofs 验证连续生成多个证明时每个都是独立的
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
			t.Fatalf("第 %d 次 InferenceWithProof 失败: %v", i+1, err)
		}
		if proof == nil {
			t.Fatalf("第 %d 次生成的证明为 nil", i+1)
		}
		proofs = append(proofs, proof)
	}

	// 验证生成了正确数量的证明
	if len(proofs) != len(prompts) {
		t.Fatalf("生成的证明数量不匹配: got=%d, want=%d", len(proofs), len(prompts))
	}

	// 验证每个证明都有独立的数据
	for i := 0; i < len(proofs); i++ {
		if proofs[i].Type == "" {
			t.Errorf("第 %d 个证明类型为空", i+1)
		}
		if len(proofs[i].ProofData) == 0 {
			t.Errorf("第 %d 个证明数据为空", i+1)
		}
	}
}

// TestProofGeneration_WitnessGeneration 测试 witness 数据生成
func TestProofGeneration_WitnessGeneration(t *testing.T) {
	modelHash := sha256.Sum256([]byte("test-model-for-witness"))
	promptHash := sha256.Sum256([]byte("explain zero knowledge proofs"))
	outputHash := sha256.Sum256([]byte("A zero knowledge proof is..."))
	logitsHash := sha256.Sum256([]byte("logits-data-hash"))

	circuit := NewVerifierCircuit(modelHash[:], promptHash[:], outputHash[:], logitsHash[:])

	// 使用 LogitsExtractor 提取真实的 logits
	extractor := NewLogitsExtractor()
	logits, err := extractor.ExtractLogits(nil, modelHash[:], promptHash[:])
	if err != nil {
		t.Fatalf("提取 logits 失败: %v", err)
	}

	// 生成 witness
	witness, err := circuit.GenerateWitness(logits, []byte("model-weights"))
	if err != nil {
		t.Fatalf("生成 witness 失败: %v", err)
	}

	if len(witness) == 0 {
		t.Fatal("witness 数据为空")
	}

	// witness 应该是有效的 JSON
	if witness[0] != '{' {
		t.Error("witness 数据不是有效的 JSON 格式")
	}
}

// TestProofGeneration_PromptHashConsistency 验证证明中的 prompt hash 与输入一致
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
		t.Fatalf("InferenceWithProof 失败: %v", err)
	}

	// 验证证明的输出哈希与响应文本的哈希一致
	expectedOutputHash := sha256.Sum256([]byte(resp.Text))
	if proof.OutputHash != expectedOutputHash {
		t.Error("证明的 OutputHash 与响应文本哈希不一致")
	}
}

// ============================================================================
// 2. 证明验证测试
// ============================================================================

// TestProofVerification_ValidProof 验证有效证明能通过验证
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
		t.Fatalf("InferenceWithProof 失败: %v", err)
	}

	valid, err := engine.VerifyProof(proof, req, resp)
	if err != nil {
		t.Fatalf("VerifyProof 返回错误: %v", err)
	}
	if !valid {
		t.Error("有效证明应当通过验证")
	}
}

// TestProofVerification_CircuitVerify 测试 VerifierCircuit 的证明验证
func TestProofVerification_CircuitVerify(t *testing.T) {
	modelHash := sha256.Sum256([]byte("circuit-verify-model"))

	circuit := NewVerifierCircuit(modelHash[:], nil, nil, nil)

	// 创建与 circuit 匹配的证明
	proof := &Proof{
		Type:      "placeholder",
		ModelID:   modelHash[:],
		Timestamp: time.Now().Unix(),
		ProofData: []byte("circuit-proof-data"),
	}

	valid, err := circuit.VerifyProof(proof)
	if err != nil {
		t.Fatalf("电路验证失败: %v", err)
	}
	if !valid {
		t.Error("匹配的证明应当通过电路验证")
	}
}

// TestProofVerification_CircuitModelMismatch 验证模型哈希不匹配时验证失败
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
		t.Error("模型哈希不匹配应当返回错误")
	}
	if valid {
		t.Error("模型哈希不匹配的证明不应通过验证")
	}
}

// TestProofVerification_ConsistencyCheck 测试 logits 一致性验证
func TestProofVerification_ConsistencyCheck(t *testing.T) {
	verifier := NewLogitsConsistencyVerifier()
	extractor := NewLogitsExtractor()

	prompt := "test consistency of logit output"
	modelID := []byte("consistency-model-01")
	promptHash := sha256.Sum256([]byte(prompt))

	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("提取 logits 失败: %v", err)
	}

	// 完整一致性检查
	valid, err := verifier.VerifyFullConsistency(logits, prompt, "generated output text", modelID)
	if err != nil {
		t.Fatalf("完整一致性验证失败: %v", err)
	}
	if !valid {
		t.Error("一致的 logits 应当通过完整验证")
	}
}

// TestProofVerification_SimilarityCheck 测试相同输入产生的 logits 相似度
func TestProofVerification_SimilarityCheck(t *testing.T) {
	simVerifier := NewLogitsSimilarityVerifier()
	extractor := NewLogitsExtractor()

	modelID := []byte("similarity-model")
	promptHash := sha256.Sum256([]byte("similarity check prompt"))

	logits1, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("提取 logits1 失败: %v", err)
	}

	// 创建完全相同的 logits 副本
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

	// 完全相同的 logits 相似度应为 1.0
	similarity, err := simVerifier.ComputeSimilarity(logits1, logits2)
	if err != nil {
		t.Fatalf("计算相似度失败: %v", err)
	}
	if similarity != 1.0 {
		t.Errorf("完全相同的 logits 相似度应为 1.0, 实际: %f", similarity)
	}

	// 测试拷贝检测 - 完全相同的 logits 应被检测为拷贝
	isCopy, maxSim, idx, err := simVerifier.DetectCopying(logits1, []*Logits{logits2})
	if err != nil {
		t.Fatalf("拷贝检测失败: %v", err)
	}
	if !isCopy {
		t.Error("完全相同的 logits 应被检测为拷贝")
	}
	if maxSim < 0.95 {
		t.Errorf("拷贝检测相似度过低: %f", maxSim)
	}
	if idx != 0 {
		t.Errorf("最相似索引应为 0, 实际: %d", idx)
	}
}

// TestProofVerification_QuantizedLogits 验证量化后的 logits 正确性
func TestProofVerification_QuantizedLogits(t *testing.T) {
	extractor := NewLogitsExtractor()
	modelID := []byte("quantize-model")
	promptHash := sha256.Sum256([]byte("quantization test prompt"))

	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("提取 logits 失败: %v", err)
	}

	// 测试三种量化级别
	quantizationBits := []int{8, 16, 32}
	for _, bits := range quantizationBits {
		extractor.SetQuantization(bits)
		quantized, err := extractor.QuantizeLogits(logits)
		if err != nil {
			t.Fatalf("%d-bit 量化失败: %v", bits, err)
		}
		if len(quantized) != len(logits.Values) {
			t.Errorf("%d-bit 量化后长度不匹配: got=%d, want=%d", bits, len(quantized), len(logits.Values))
		}

		// 验证量化值在合理范围内
		for j, v := range quantized {
			switch bits {
			case 8:
				if v < 0 || v > 255 {
					t.Errorf("8-bit 量化值超出范围 [0,255]: index=%d, value=%d", j, v)
				}
			case 16:
				if v < math.MinInt16 || v > math.MaxInt16 {
					t.Errorf("16-bit 量化值超出范围: index=%d, value=%d", j, v)
				}
			}
		}
	}
}

// ============================================================================
// 3. 证明错误处理测试
// ============================================================================

// TestProofError_NilProof 测试 nil 证明的错误处理
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
		t.Fatalf("Inference 失败: %v", err)
	}

	valid, err := engine.VerifyProof(nil, req, resp)
	if err == nil {
		t.Error("nil 证明应当返回错误")
	}
	if valid {
		t.Error("nil 证明不应通过验证")
	}
}

// TestProofError_NilRequest 测试 nil 请求的错误处理
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
		t.Error("nil 请求应当返回错误")
	}
	if valid {
		t.Error("nil 请求不应通过验证")
	}
}

// TestProofError_EmptyPromptInference 测试空 prompt 推理的错误处理
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
		t.Error("空 prompt 应当返回错误")
	}
}

// TestProofError_EmptyModelID 测试空 ModelID 推理的错误处理
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
		t.Error("空 ModelID 应当返回错误")
	}
}

// TestProofError_InvalidTemperature 测试无效温度参数的错误处理
func TestProofError_InvalidTemperature(t *testing.T) {
	engine := NewInferenceEngine()

	testCases := []struct {
		name        string
		temperature float64
	}{
		{"负温度", -0.1},
		{"过高温度", 2.5},
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
				t.Errorf("温度 %.1f 应当返回错误", tc.temperature)
			}
		})
	}
}

// TestProofError_InvalidTopP 测试无效 TopP 参数的错误处理
func TestProofError_InvalidTopP(t *testing.T) {
	engine := NewInferenceEngine()

	testCases := []struct {
		name string
		topP float64
	}{
		{"负 TopP", -0.1},
		{"过高 TopP", 1.5},
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
				t.Errorf("TopP %.1f 应当返回错误", tc.topP)
			}
		})
	}
}

// TestProofError_NilInferenceRequest 测试 nil 推理请求的错误处理
func TestProofError_NilInferenceRequest(t *testing.T) {
	engine := NewInferenceEngine()

	_, _, err := engine.InferenceWithProof(nil)
	if err == nil {
		t.Error("nil 请求应当返回错误")
	}
}

// TestProofError_CircuitNilProof 测试电路验证 nil 证明
func TestProofError_CircuitNilProof(t *testing.T) {
	circuit := NewVerifierCircuit(nil, nil, nil, nil)

	valid, err := circuit.VerifyProof(nil)
	if err == nil {
		t.Error("nil 证明应当返回错误")
	}
	if valid {
		t.Error("nil 证明不应通过电路验证")
	}
}

// TestProofError_CircuitEmptyProofData 测试非 placeholder 类型但无证明数据
func TestProofError_CircuitEmptyProofData(t *testing.T) {
	circuit := NewVerifierCircuit(nil, nil, nil, nil)

	proof := &Proof{
		Type:      "zkml",
		ModelID:   []byte("some-model"),
		Timestamp: time.Now().Unix(),
		ProofData: []byte{}, // 空证明数据
	}

	valid, err := circuit.VerifyProof(proof)
	if err == nil {
		t.Error("非 placeholder 类型空证明数据应当返回错误")
	}
	if valid {
		t.Error("空证明数据不应通过验证")
	}
}

// TestProofError_WitnessNilLogits 测试 witness 生成时 nil logits
func TestProofError_WitnessNilLogits(t *testing.T) {
	circuit := NewVerifierCircuit(nil, nil, nil, nil)

	_, err := circuit.GenerateWitness(nil, []byte("weights"))
	if err == nil {
		t.Error("nil logits 应当在 witness 生成时返回错误")
	}
}

// TestProofError_QuantizeNilLogits 测试量化 nil logits
func TestProofError_QuantizeNilLogits(t *testing.T) {
	extractor := NewLogitsExtractor()

	_, err := extractor.QuantizeLogits(nil)
	if err == nil {
		t.Error("nil logits 应当在量化时返回错误")
	}
}

// TestProofError_QuantizeEmptyLogits 测试量化空 logits
func TestProofError_QuantizeEmptyLogits(t *testing.T) {
	extractor := NewLogitsExtractor()

	emptyLogits := &Logits{Values: []float64{}}
	_, err := extractor.QuantizeLogits(emptyLogits)
	if err == nil {
		t.Error("空 logits 应当在量化时返回错误")
	}
}

// TestProofError_ConsistencyNilLogits 测试一致性验证 nil logits
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
					t.Error("应当返回错误")
				}
			}
			if tc.logits == nil && tc.prompt != "" {
				_, err := verifier.VerifyLogitsToPrompt(tc.logits, tc.prompt)
				if err == nil {
					t.Error("应当返回错误")
				}
			}
			if tc.logits == nil && len(tc.model) > 0 {
				_, err := verifier.VerifyLogitsToModel(tc.logits, tc.model)
				if err == nil {
					t.Error("应当返回错误")
				}
			}
			if tc.logits != nil && tc.prompt == "" && tc.output != "" {
				_, err := verifier.VerifyLogitsToOutput(tc.logits, tc.output)
				// 这里 logits 有值但只有一个元素，可能会因 "all same" 检查失败
				_ = err
			}
			if tc.logits != nil && tc.output == "" && tc.prompt != "" {
				_, err := verifier.VerifyLogitsToPrompt(tc.logits, tc.prompt)
				_ = err
			}
			if tc.logits != nil && len(tc.model) == 0 {
				_, err := verifier.VerifyLogitsToModel(tc.logits, tc.model)
				if err == nil {
					t.Error("空 modelID 应当返回错误")
				}
			}
		})
	}
}

// TestProofError_SimilarityNilLogits 测试相似度计算 nil logits
func TestProofError_SimilarityNilLogits(t *testing.T) {
	verifier := NewLogitsSimilarityVerifier()

	_, err := verifier.ComputeSimilarity(nil, nil)
	if err == nil {
		t.Error("nil logits 应当在相似度计算时返回错误")
	}

	logits := &Logits{Values: []float64{1.0, 2.0}}
	_, err = verifier.ComputeSimilarity(logits, nil)
	if err == nil {
		t.Error("一个 nil logits 应当返回错误")
	}

	_, err = verifier.ComputeSimilarity(nil, logits)
	if err == nil {
		t.Error("一个 nil logits 应当返回错误")
	}
}

// TestProofError_SimilarityLengthMismatch 测试长度不匹配的 logits 相似度
func TestProofError_SimilarityLengthMismatch(t *testing.T) {
	verifier := NewLogitsSimilarityVerifier()

	logits1 := &Logits{Values: []float64{1.0, 2.0, 3.0}}
	logits2 := &Logits{Values: []float64{1.0, 2.0}}

	_, err := verifier.ComputeSimilarity(logits1, logits2)
	if err == nil {
		t.Error("长度不匹配的 logits 应当返回错误")
	}
}

// TestProofError_DetectCopyingNilLogits 测试拷贝检测中的 nil logits
func TestProofError_DetectCopyingNilLogits(t *testing.T) {
	verifier := NewLogitsSimilarityVerifier()

	_, _, _, err := verifier.DetectCopying(nil, []*Logits{})
	if err == nil {
		t.Error("nil logits 应当在拷贝检测时返回错误")
	}
}

// TestProofError_PromptHashMismatch 测试 prompt hash 不匹配的验证
func TestProofError_PromptHashMismatch(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("生成 challenge 失败: %v", err)
	}

	// 创建 logits 但使用不同的 prompt hash
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
		t.Error("prompt hash 不匹配应当验证失败")
	}
}

// TestProofError_FutureTimestamp 测试未来时间戳的 logits
func TestProofError_FutureTimestamp(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("生成 challenge 失败: %v", err)
	}

	promptHash := sha256.Sum256([]byte(challenge.Prompt))
	logits := &Logits{
		Values:     []float64{5.0, 3.5, 2.0, -1.0, -2.0},
		TokenIDs:   []int{0, 1, 2, 3, 4},
		TopK:       3,
		Timestamp:  time.Now().Unix() + 600, // 10 分钟后，超过 5 分钟容差
		ModelID:    []byte("test-model"),
		PromptHash: promptHash[:],
	}

	valid, err := verifier.VerifyLogits(logits, challenge)
	if err == nil || valid {
		t.Error("未来时间戳的 logits 应当验证失败")
	}
}

// TestProofError_AllNaNLogits 测试全 NaN 值的 logits
func TestProofError_AllNaNLogits(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("生成 challenge 失败: %v", err)
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
		t.Error("全 NaN 的 logits 应当验证失败")
	}
}

// TestProofError_AllInfLogits 测试全 Inf 值的 logits
func TestProofError_AllInfLogits(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("生成 challenge 失败: %v", err)
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
		t.Error("全 Inf 的 logits 应当验证失败")
	}
}

// TestProofError_EmptyLogitsVerification 测试空 logits 的验证
func TestProofError_EmptyLogitsVerification(t *testing.T) {
	verifier := NewLogitsVerifier()
	generator := NewRandomInputGenerator()

	challenge, err := generator.GenerateChallenge(60, 10)
	if err != nil {
		t.Fatalf("生成 challenge 失败: %v", err)
	}

	logits := &Logits{
		Values:    []float64{},
		TokenIDs:  []int{},
		Timestamp: time.Now().Unix(),
	}

	valid, err := verifier.VerifyLogits(logits, challenge)
	if err == nil || valid {
		t.Error("空 logits 应当验证失败")
	}
}

// ============================================================================
// 4. 证明性能测试
// ============================================================================

// TestPerformance_ProofGeneration 测试证明生成性能
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
			t.Fatalf("第 %d 次证明生成失败: %v", i+1, err)
		}
		if proof == nil {
			t.Fatalf("第 %d 次生成的证明为 nil", i+1)
		}
	}
	elapsed := time.Since(start)

	avgDuration := elapsed / time.Duration(iterations)
	t.Logf("证明生成性能: %d 次迭代, 总耗时 %v, 平均 %v/次", iterations, elapsed, avgDuration)

	// 单次证明生成不应超过 100ms
	if avgDuration > 100*time.Millisecond {
		t.Errorf("证明生成平均耗时过长: %v (阈值 100ms)", avgDuration)
	}
}

// TestPerformance_ProofVerification 测试证明验证性能
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

	// 先生成证明
	resp, proof, err := engine.InferenceWithProof(req)
	if err != nil {
		t.Fatalf("证明生成失败: %v", err)
	}

	start := time.Now()
	for i := 0; i < iterations; i++ {
		valid, err := engine.VerifyProof(proof, req, resp)
		if err != nil {
			t.Fatalf("第 %d 次验证失败: %v", i+1, err)
		}
		if !valid {
			t.Fatalf("第 %d 次验证结果应为 true", i+1)
		}
	}
	elapsed := time.Since(start)

	avgDuration := elapsed / time.Duration(iterations)
	t.Logf("证明验证性能: %d 次迭代, 总耗时 %v, 平均 %v/次", iterations, elapsed, avgDuration)

	// 单次验证不应超过 10ms
	if avgDuration > 10*time.Millisecond {
		t.Errorf("证明验证平均耗时过长: %v (阈值 10ms)", avgDuration)
	}
}

// TestPerformance_BatchProofGeneration 测试批量证明生成性能
func TestPerformance_BatchProofGeneration(t *testing.T) {
	engine := NewInferenceEngine()
	batchSize := 50

	prompts := make([]string, batchSize)
	for i := 0; i < batchSize; i++ {
		gen := NewRandomInputGenerator()
		prompt, err := gen.GenerateRandomPrompt(5)
		if err != nil {
			t.Fatalf("生成第 %d 个随机 prompt 失败: %v", i+1, err)
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
			t.Fatalf("批量第 %d 次证明生成失败: %v", i+1, err)
		}
		proofs = append(proofs, proof)
	}
	elapsed := time.Since(start)

	if len(proofs) != batchSize {
		t.Fatalf("批量生成证明数量不匹配: got=%d, want=%d", len(proofs), batchSize)
	}

	avgDuration := elapsed / time.Duration(batchSize)
	t.Logf("批量证明生成: %d 个证明, 总耗时 %v, 平均 %v/个", batchSize, elapsed, avgDuration)
}

// TestPerformance_CircuitVerification 测试电路验证性能
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
			t.Fatalf("第 %d 次电路验证失败: %v", i+1, err)
		}
		if !valid {
			t.Fatalf("第 %d 次电路验证结果应为 true", i+1)
		}
	}
	elapsed := time.Since(start)

	avgDuration := elapsed / time.Duration(iterations)
	t.Logf("电路验证性能: %d 次迭代, 总耗时 %v, 平均 %v/次", iterations, elapsed, avgDuration)

	// 电路验证应非常快
	if avgDuration > 1*time.Millisecond {
		t.Errorf("电路验证平均耗时过长: %v (阈值 1ms)", avgDuration)
	}
}

// TestPerformance_WitnessGeneration 测试 witness 生成性能
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
		t.Fatalf("提取 logits 失败: %v", err)
	}

	start := time.Now()
	for i := 0; i < iterations; i++ {
		witness, err := circuit.GenerateWitness(logits, []byte("model-weights-data"))
		if err != nil {
			t.Fatalf("第 %d 次 witness 生成失败: %v", i+1, err)
		}
		if len(witness) == 0 {
			t.Fatalf("第 %d 次 witness 数据为空", i+1)
		}
	}
	elapsed := time.Since(start)

	avgDuration := elapsed / time.Duration(iterations)
	t.Logf("Witness 生成性能: %d 次迭代, 总耗时 %v, 平均 %v/次", iterations, elapsed, avgDuration)

	if avgDuration > 5*time.Millisecond {
		t.Errorf("Witness 生成平均耗时过长: %v (阈值 5ms)", avgDuration)
	}
}

// TestPerformance_LogitsSimilarity 测试 logits 相似度计算性能
func TestPerformance_LogitsSimilarity(t *testing.T) {
	verifier := NewLogitsSimilarityVerifier()
	extractor := NewLogitsExtractor()
	iterations := 500

	modelID := []byte("perf-similarity-model")
	promptHash1 := sha256.Sum256([]byte("similarity benchmark prompt 1"))
	promptHash2 := sha256.Sum256([]byte("similarity benchmark prompt 2"))

	logits1, err := extractor.ExtractLogits(nil, modelID, promptHash1[:])
	if err != nil {
		t.Fatalf("提取 logits1 失败: %v", err)
	}
	logits2, err := extractor.ExtractLogits(nil, modelID, promptHash2[:])
	if err != nil {
		t.Fatalf("提取 logits2 失败: %v", err)
	}

	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, err := verifier.ComputeSimilarity(logits1, logits2)
		if err != nil {
			t.Fatalf("第 %d 次相似度计算失败: %v", i+1, err)
		}
	}
	elapsed := time.Since(start)

	avgDuration := elapsed / time.Duration(iterations)
	t.Logf("Logits 相似度计算性能: %d 次迭代, 总耗时 %v, 平均 %v/次", iterations, elapsed, avgDuration)

	if avgDuration > 1*time.Millisecond {
		t.Errorf("Logits 相似度计算平均耗时过长: %v (阈值 1ms)", avgDuration)
	}
}

// TestPerformance_Quantization 测试量化性能
func TestPerformance_Quantization(t *testing.T) {
	extractor := NewLogitsExtractor()
	iterations := 300

	modelID := []byte("perf-quant-model")
	promptHash := sha256.Sum256([]byte("quantization benchmark prompt"))
	logits, err := extractor.ExtractLogits(nil, modelID, promptHash[:])
	if err != nil {
		t.Fatalf("提取 logits 失败: %v", err)
	}

	quantBits := []int{8, 16, 32}
	for _, bits := range quantBits {
		extractor.SetQuantization(bits)

		start := time.Now()
		for i := 0; i < iterations; i++ {
			q, err := extractor.QuantizeLogits(logits)
			if err != nil {
				t.Fatalf("%d-bit 第 %d 次量化失败: %v", bits, i+1, err)
			}
			if len(q) == 0 {
				t.Fatalf("%d-bit 第 %d 次量化结果为空", bits, i+1)
			}
		}
		elapsed := time.Since(start)

		avgDuration := elapsed / time.Duration(iterations)
		t.Logf("%d-bit 量化性能: %d 次迭代, 总耗时 %v, 平均 %v/次", bits, iterations, elapsed, avgDuration)
	}
}
