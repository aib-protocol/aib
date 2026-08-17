package consensus

import (
	"encoding/json"
	"testing"
)

func TestBlock_NewBlock(t *testing.T) {
	event := &BlockEvent{
		TaskID:           "test-task",
		FinalResult:      "final result",
		IsValid:          true,
		AgreementRate:    0.85,
		NodeResults:      map[string]string{"n1": "r1", "n2": "r2"},
		ConsensusNodes:   []string{"n1", "n2"},
		DisagreeingNodes: []string{"n3"},
		Metadata:         map[string]string{"key": "value"},
		Timestamp:        1234567890,
		BlockHeight:      5,
	}

	block := NewBlock(5, []byte("prevhash"), event)

	if block.Height != 5 {
		t.Errorf("expected height 5, got %d", block.Height)
	}
	if block.TaskID != "test-task" {
		t.Errorf("expected task ID test-task, got %s", block.TaskID)
	}
	if block.FinalResult != "final result" {
		t.Errorf("expected final result 'final result', got %s", block.FinalResult)
	}
	if !block.IsValid {
		t.Error("expected IsValid to be true")
	}
	if block.AgreementRate != 0.85 {
		t.Errorf("expected agreement rate 0.85, got %f", block.AgreementRate)
	}
}

func TestBlock_CalculateHash(t *testing.T) {
	block := &Block{
		Height:         1,
		PrevBlockHash:  []byte("prevhash"),
		Timestamp:      1234567890,
		TaskID:         "test-task",
		FinalResult:    "result",
		IsValid:        true,
		AgreementRate:  0.8,
		NodeResults:    map[string]string{"n1": "r1"},
		ConsensusNodes: []string{"n1"},
		Nonce:          42,
	}

	hash := block.CalculateHash()

	if len(hash) != 32 {
		t.Errorf("expected hash length 32, got %d", len(hash))
	}

	// Same block should produce same hash
	hash2 := block.CalculateHash()
	if !hashEqual(hash, hash2) {
		t.Error("same block should produce same hash")
	}
}

func TestBlock_Hash(t *testing.T) {
	block := &Block{
		Height:        1,
		PrevBlockHash: []byte("prevhash"),
		TaskID:        "task",
		FinalResult:   "result",
		IsValid:       true,
		AgreementRate: 1.0,
	}

	// First call should compute hash
	hash1 := block.Hash()

	// Second call should return cached hash
	hash2 := block.Hash()

	if !hashEqual(hash1, hash2) {
		t.Error("Hash() should return same value on subsequent calls")
	}

	if block.BlockHash == nil {
		t.Error("BlockHash should be set after Hash() call")
	}
}

func TestBlock_PreviousBlockHash(t *testing.T) {
	block := &Block{
		PrevBlockHash: []byte("test"),
	}

	hash := block.PreviousBlockHash()
	if !hashEqual(hash, []byte("test")) {
		t.Error("PreviousBlockHash should return the set value")
	}

	block2 := &Block{}
	hash2 := block2.PreviousBlockHash()
	if len(hash2) != 32 {
		t.Errorf("expected 32-byte zero hash for nil PrevBlockHash, got %d", len(hash2))
	}
}

func TestBlock_String(t *testing.T) {
	block := &Block{
		Height:        100,
		TaskID:        "test-task",
		FinalResult:   "result",
		IsValid:       true,
		AgreementRate: 0.85,
		Nonce:         123,
	}

	block.BlockHash = block.CalculateHash()

	str := block.String()

	if str == "" {
		t.Error("String() should not return empty string")
	}

	// Check for expected substrings
	expectedSubstrings := []string{
		"Block{",
		"100",       // height
		"test-task", // task ID
		"true",      // is valid
		"85.00",     // agreement rate percentage
	}

	for _, substr := range expectedSubstrings {
		if !contains(str, substr) {
			t.Errorf("String() should contain '%s', got: %s", substr, str)
		}
	}
}

func TestBlock_ToJSON(t *testing.T) {
	block := &Block{
		Height:        1,
		TaskID:        "test",
		FinalResult:   "result",
		IsValid:       true,
		AgreementRate: 0.9,
	}

	data, err := block.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("ToJSON should not return empty data")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed["task_id"] != "test" {
		t.Error("JSON should contain task_id")
	}
}

func TestBlock_ToDisplayMap(t *testing.T) {
	block := &Block{
		Height:           10,
		TaskID:           "test-task",
		FinalResult:      "final",
		IsValid:          true,
		AgreementRate:    0.75,
		ConsensusNodes:   []string{"n1", "n2"},
		DisagreeingNodes: []string{"n3"},
		Nonce:            999,
		Timestamp:        1234567890,
	}
	block.BlockHash = block.CalculateHash()
	block.PrevBlockHash = []byte("prev")

	display := block.ToDisplayMap()

	if display == nil {
		t.Fatal("ToDisplayMap should not return nil")
	}

	if display["height"] != uint64(10) {
		t.Error("height should be 10")
	}

	if display["task_id"] != "test-task" {
		t.Error("task_id should be test-task")
	}

	if display["final_result"] != "final" {
		t.Error("final_result should be 'final'")
	}

	if display["is_valid"] != true {
		t.Error("is_valid should be true")
	}

	agreementStr, ok := display["agreement_rate"].(string)
	if !ok || agreementStr != "75.00%" {
		t.Errorf("agreement_rate should be '75.00%%', got %v", display["agreement_rate"])
	}

	if display["consensus_nodes"] != 2 {
		t.Error("consensus_nodes count should be 2")
	}

	if display["disagreeing_nodes"] != 1 {
		t.Error("disagreeing_nodes count should be 1")
	}

	if display["nonce"] != uint64(999) {
		t.Error("nonce should be 999")
	}
}

func TestBlock_Serialize(t *testing.T) {
	block := &Block{
		Height:        5,
		TaskID:        "serialize-test",
		FinalResult:   "result",
		IsValid:       true,
		AgreementRate: 1.0,
		NodeResults:   map[string]string{"n1": "r1"},
	}

	data, err := block.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data should not be empty")
	}
}

func TestDeserializeBlock(t *testing.T) {
	original := &Block{
		Height:         7,
		TaskID:         "deserialize-test",
		FinalResult:    "final",
		IsValid:        false,
		AgreementRate:  0.5,
		NodeResults:    map[string]string{"node1": "result1"},
		ConsensusNodes: []string{"node1"},
		Nonce:          42,
	}

	data, err := original.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	deserialized, err := DeserializeBlock(data)
	if err != nil {
		t.Fatalf("DeserializeBlock failed: %v", err)
	}

	if deserialized.Height != original.Height {
		t.Errorf("expected height %d, got %d", original.Height, deserialized.Height)
	}

	if deserialized.TaskID != original.TaskID {
		t.Errorf("expected task ID %s, got %s", original.TaskID, deserialized.TaskID)
	}

	if deserialized.IsValid != original.IsValid {
		t.Errorf("expected IsValid %v, got %v", original.IsValid, deserialized.IsValid)
	}

	if deserialized.AgreementRate != original.AgreementRate {
		t.Errorf("expected agreement rate %f, got %f", original.AgreementRate, deserialized.AgreementRate)
	}

	if deserialized.Nonce != original.Nonce {
		t.Errorf("expected nonce %d, got %d", original.Nonce, deserialized.Nonce)
	}
}

func TestDeserializeBlock_InvalidData(t *testing.T) {
	_, err := DeserializeBlock([]byte("invalid data"))
	if err == nil {
		t.Error("expected error for invalid data")
	}
}

func TestBlock_DeterministicHash(t *testing.T) {
	// Two blocks with same data should have same hash
	nodeResults := map[string]string{
		"node1": "result1",
		"node2": "result2",
		"node3": "result3",
	}

	block1 := &Block{
		Height:        1,
		PrevBlockHash: []byte("prev"),
		Timestamp:     1234567890,
		TaskID:        "task",
		FinalResult:   "result",
		IsValid:       true,
		AgreementRate: 0.9,
		NodeResults:   nodeResults,
		Nonce:         100,
	}

	block2 := &Block{
		Height:        1,
		PrevBlockHash: []byte("prev"),
		Timestamp:     1234567890,
		TaskID:        "task",
		FinalResult:   "result",
		IsValid:       true,
		AgreementRate: 0.9,
		NodeResults:   nodeResults,
		Nonce:         100,
	}

	hash1 := block1.CalculateHash()
	hash2 := block2.CalculateHash()

	if !hashEqual(hash1, hash2) {
		t.Error("same block data should produce same hash")
	}
}

func TestBlock_HashChangesWithDifferentData(t *testing.T) {
	block1 := &Block{
		Height:      1,
		TaskID:      "task",
		FinalResult: "result1",
		Nonce:       1,
	}

	block2 := &Block{
		Height:      1,
		TaskID:      "task",
		FinalResult: "result2", // Different result
		Nonce:       1,
	}

	hash1 := block1.CalculateHash()
	hash2 := block2.CalculateHash()

	if hashEqual(hash1, hash2) {
		t.Error("different block data should produce different hash")
	}
}

func TestBlock_NonceAffectsHash(t *testing.T) {
	block1 := &Block{
		Height:      1,
		TaskID:      "task",
		FinalResult: "result",
		Nonce:       1,
	}

	block2 := &Block{
		Height:      1,
		TaskID:      "task",
		FinalResult: "result",
		Nonce:       2, // Different nonce
	}

	hash1 := block1.CalculateHash()
	hash2 := block2.CalculateHash()

	if hashEqual(hash1, hash2) {
		t.Error("different nonce should produce different hash")
	}
}

// Helper functions
func hashEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
