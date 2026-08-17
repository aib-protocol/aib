package inference

import (
	"context"
	"testing"
	"time"
)

func TestDefaultUnifiedConfig(t *testing.T) {
	config := DefaultUnifiedConfig()

	if config == nil {
		t.Fatal("expected non-nil config")
	}

	if config.Type != ProviderTypeOpenAI {
		t.Errorf("expected type OpenAI, got %s", config.Type)
	}

	if config.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected base URL https://api.openai.com/v1, got %s", config.BaseURL)
	}

	if config.Model != "gpt-3.5-turbo" {
		t.Errorf("expected model gpt-3.5-turbo, got %s", config.Model)
	}

	if config.Weight != 1.0 {
		t.Errorf("expected weight 1.0, got %f", config.Weight)
	}

	if config.Timeout != 120*time.Second {
		t.Errorf("expected timeout 120s, got %v", config.Timeout)
	}

	if config.MaxTokens != 2048 {
		t.Errorf("expected max tokens 2048, got %d", config.MaxTokens)
	}

	if config.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", config.Temperature)
	}

	if config.TopP != 0.9 {
		t.Errorf("expected top p 0.9, got %f", config.TopP)
	}
}

func TestNewUnifiedProvider(t *testing.T) {
	config := &UnifiedConfig{
		Type:        ProviderTypeOllama,
		BaseURL:     "http://localhost:11434",
		Model:       "llama2",
		Weight:      1.5,
		Timeout:     60 * time.Second,
		MaxTokens:   1024,
		Temperature: 0.5,
		TopP:        0.8,
	}

	provider, err := NewUnifiedProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if provider == nil {
		t.Fatal("expected non-nil provider")
	}

	if provider.config != config {
		t.Error("expected config to be set")
	}

	if provider.modelID == nil {
		t.Error("expected model ID to be set")
	}

	if provider.httpClient == nil {
		t.Error("expected HTTP client to be set")
	}

	if provider.modelRegistry == nil {
		t.Error("expected model registry to be set")
	}
}

func TestNewUnifiedProviderNilConfig(t *testing.T) {
	// nil config should use defaults
	provider, err := NewUnifiedProvider(nil)
	if err != nil {
		t.Fatalf("failed to create provider with nil config: %v", err)
	}

	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewUnifiedProviderEmptyBaseURL(t *testing.T) {
	config := &UnifiedConfig{
		Model: "test-model",
		// BaseURL is empty
	}

	_, err := NewUnifiedProvider(config)
	if err == nil {
		t.Error("expected error for empty base URL")
	}
}

func TestNewUnifiedProviderEmptyModel(t *testing.T) {
	config := &UnifiedConfig{
		BaseURL: "http://localhost:11434",
		// Model is empty
	}

	_, err := NewUnifiedProvider(config)
	if err == nil {
		t.Error("expected error for empty model")
	}
}

func TestUnifiedProvider_ModelID(t *testing.T) {
	config := &UnifiedConfig{
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
	}

	provider, err := NewUnifiedProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	modelID := provider.ModelID()
	if modelID == nil {
		t.Error("expected non-nil model ID")
	}

	// Model ID should be deterministic (SHA-256 hash)
	modelID2 := provider.ModelID()
	if len(modelID) != len(modelID2) {
		t.Error("model ID should have consistent length")
	}
}

func TestUnifiedProvider_GetWeight(t *testing.T) {
	config := &UnifiedConfig{
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
		Weight:  2.0,
	}

	provider, err := NewUnifiedProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	weight := provider.GetWeight()
	// Without performance data, should return config weight
	if weight != 2.0 {
		t.Errorf("expected weight 2.0, got %f", weight)
	}
}

func TestUnifiedProvider_GetWeightWithPerformance(t *testing.T) {
	registry := NewModelRegistry()

	// Register a model
	modelID := "test-model"
	registry.RegisterModel(&ModelInfo{
		ModelID: modelID,
		Performance: &ModelPerformance{
			TaskCompletionRate: 0.95,
			UserSatisfaction:   4.5,
			CostPerTask:        0.005,
		},
	})

	config := &UnifiedConfig{
		BaseURL: "http://localhost:11434",
		Model:   modelID,
		Weight:  1.0,
	}

	provider, err := NewUnifiedProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Override the registry to use our test registry
	provider.modelRegistry = registry

	weight := provider.GetWeight()
	// Should be adjusted based on performance
	if weight <= 0 {
		t.Error("expected positive weight")
	}
}

func TestUnifiedProvider_WeightedInfer(t *testing.T) {
	config := &UnifiedConfig{
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
		Weight:  1.5,
	}

	provider, err := NewUnifiedProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// This will fail since there's no actual server, but we can test the call
	ctx := context.Background()
	_, _, err = provider.WeightedInfer(ctx, "test prompt")

	// We expect an error since there's no actual server
	// But the method should at least be callable
	if err == nil {
		t.Log("Note: test passed without server (unexpected in production)")
	}
}

func TestUnifiedProvider_ProposeWeightAdjustment(t *testing.T) {
	config := &UnifiedConfig{
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
	}

	provider, err := NewUnifiedProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Valid proposal
	proposal, err := provider.ProposeWeightAdjustment("model1", 1.0, 2.0, map[string]interface{}{
		"model_id":        "model1",
		"proposed_weight": 2.0,
	})
	if err != nil {
		t.Fatalf("failed to create proposal: %v", err)
	}

	if proposal == nil {
		t.Fatal("expected non-nil proposal")
	}

	if proposal.Type != ProposalTypeWeightAdjustment {
		t.Errorf("expected type weight_adjustment, got %s", proposal.Type)
	}

	// Invalid proposal - zero weight
	_, err = provider.ProposeWeightAdjustment("model1", 1.0, 0, nil)
	if err == nil {
		t.Error("expected error for zero weight")
	}

	// Invalid proposal - negative weight
	_, err = provider.ProposeWeightAdjustment("model1", 1.0, -1.0, nil)
	if err == nil {
		t.Error("expected error for negative weight")
	}
}

func TestUnifiedProvider_ExecuteProposal(t *testing.T) {
	config := &UnifiedConfig{
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
	}

	provider, err := NewUnifiedProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Execute non-passed proposal
	proposal := &GovernanceProposal{
		Type:   ProposalTypeWeightAdjustment,
		Status: ProposalStatusActive,
	}

	err = provider.ExecuteProposal(proposal)
	if err == nil {
		t.Error("expected error for non-passed proposal")
	}

	// Execute passed proposal
	proposal2 := &GovernanceProposal{
		ID:     "prop-1",
		Type:   ProposalTypeWeightAdjustment,
		Status: ProposalStatusPassed,
		Evidence: map[string]interface{}{
			"model_id":        "model1",
			"proposed_weight": 2.5,
		},
	}

	// Without a registry, this should still work (no-op)
	err = provider.ExecuteProposal(proposal2)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Execute unknown proposal type
	proposal3 := &GovernanceProposal{
		Type:   "unknown_type",
		Status: ProposalStatusPassed,
	}

	err = provider.ExecuteProposal(proposal3)
	if err == nil {
		t.Error("expected error for unknown proposal type")
	}
}

func TestUnifiedProvider_Infer(t *testing.T) {
	config := &UnifiedConfig{
		Type:    ProviderTypeOllama,
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
	}

	_, err := NewUnifiedProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	ctx := context.Background()

	// Test default fallback (unknown type)
	configUnknown := &UnifiedConfig{
		Type:    "unknown",
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
	}
	providerUnknown, _ := NewUnifiedProvider(configUnknown)
	_, _ = providerUnknown.Infer(ctx, "test")

	// Test custom type (should error)
	configCustom := &UnifiedConfig{
		Type:    ProviderTypeCustom,
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
	}
	providerCustom, _ := NewUnifiedProvider(configCustom)
	_, err = providerCustom.Infer(ctx, "test")
	if err == nil {
		t.Error("expected error for custom type")
	}

	// Test plugin type (should error)
	configPlugin := &UnifiedConfig{
		Type:    ProviderTypePlugin,
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
	}
	providerPlugin, _ := NewUnifiedProvider(configPlugin)
	_, err = providerPlugin.Infer(ctx, "test")
	if err == nil {
		t.Error("expected error for plugin type")
	}
}

func TestInferenceProviderTypeConstants(t *testing.T) {
	tests := []struct {
		name  string
		ptype InferenceProviderType
		value string
	}{
		{"OpenAI", ProviderTypeOpenAI, "openai-compatible"},
		{"Ollama", ProviderTypeOllama, "ollama"},
		{"Anthropic", ProviderTypeAnthropic, "anthropic"},
		{"Custom", ProviderTypeCustom, "custom"},
		{"Plugin", ProviderTypePlugin, "plugin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.ptype) != tt.value {
				t.Errorf("expected %s, got %s", tt.value, tt.ptype)
			}
		})
	}
}

func TestModelInfo(t *testing.T) {
	info := &ModelInfo{
		ModelID: "model-1",
		Name:    "Test Model",
		Type:    ProviderTypeOllama,
		BaseURL: "http://localhost:11434",
		Weight:  1.5,
		Performance: &ModelPerformance{
			TaskCompletionRate: 0.9,
			UserSatisfaction:   4.2,
			AvgResponseTime:    150 * time.Millisecond,
			CostPerTask:        0.008,
			ReliabilityScore:   0.95,
		},
		RegisteredAt: time.Now(),
	}

	if info.ModelID != "model-1" {
		t.Errorf("expected model ID model-1, got %s", info.ModelID)
	}

	if info.Performance.TaskCompletionRate != 0.9 {
		t.Errorf("expected 0.9, got %f", info.Performance.TaskCompletionRate)
	}
}

func TestGovernanceProposalTypes(t *testing.T) {
	if ProposalTypeWeightAdjustment != "weight_adjustment" {
		t.Errorf("expected weight_adjustment, got %s", ProposalTypeWeightAdjustment)
	}
	if ProposalTypeModelRegistration != "model_registration" {
		t.Errorf("expected model_registration, got %s", ProposalTypeModelRegistration)
	}
	if ProposalTypeModelRemoval != "model_removal" {
		t.Errorf("expected model_removal, got %s", ProposalTypeModelRemoval)
	}
}

func TestGovernanceProposalStatus(t *testing.T) {
	if ProposalStatusPending != "pending" {
		t.Errorf("expected pending, got %s", ProposalStatusPending)
	}
	if ProposalStatusActive != "active" {
		t.Errorf("expected active, got %s", ProposalStatusActive)
	}
	if ProposalStatusPassed != "passed" {
		t.Errorf("expected passed, got %s", ProposalStatusPassed)
	}
	if ProposalStatusRejected != "rejected" {
		t.Errorf("expected rejected, got %s", ProposalStatusRejected)
	}
}
