package main

import (
	"testing"
)

// ============================================================
// §6.1A Token Estimator Tests
// ============================================================

func TestEstimateFromUpstream_Valid(t *testing.T) {
	usage := map[string]interface{}{
		"prompt_tokens":     float64(100),
		"completion_tokens": float64(50),
		"total_tokens":      float64(150),
	}
	est := EstimateFromUpstream(usage)
	if est == nil {
		t.Fatal("expected non-nil estimate")
	}
	if est.PromptTokens != 100 {
		t.Errorf("prompt_tokens = %d, want 100", est.PromptTokens)
	}
	if est.CompletionTokens != 50 {
		t.Errorf("completion_tokens = %d, want 50", est.CompletionTokens)
	}
	if est.TotalTokens != 150 {
		t.Errorf("total_tokens = %d, want 150", est.TotalTokens)
	}
	if est.Source != "upstream" {
		t.Errorf("source = %s, want upstream", est.Source)
	}
}

func TestEstimateFromUpstream_Nil(t *testing.T) {
	est := EstimateFromUpstream(nil)
	if est != nil {
		t.Error("expected nil for nil usage")
	}

	est = EstimateFromUpstream(map[string]interface{}{})
	if est != nil {
		t.Error("expected nil for empty usage")
	}

	// All zeros
	est = EstimateFromUpstream(map[string]interface{}{
		"prompt_tokens":     float64(0),
		"completion_tokens": float64(0),
		"total_tokens":      float64(0),
	})
	if est != nil {
		t.Error("expected nil for all-zero usage")
	}
}

func TestEstimateFromUpstream_DerivesTotal(t *testing.T) {
	usage := map[string]interface{}{
		"prompt_tokens":     float64(80),
		"completion_tokens": float64(20),
	}
	est := EstimateFromUpstream(usage)
	if est == nil {
		t.Fatal("expected non-nil estimate")
	}
	if est.TotalTokens != 100 {
		t.Errorf("total_tokens = %d, want 100 (derived)", est.TotalTokens)
	}
}

func TestEstimateFromUpstream_IntTypes(t *testing.T) {
	// Test with int type instead of float64
	usage := map[string]interface{}{
		"prompt_tokens":     200,
		"completion_tokens": int64(100),
	}
	est := EstimateFromUpstream(usage)
	if est == nil {
		t.Fatal("expected non-nil estimate")
	}
	if est.PromptTokens != 200 {
		t.Errorf("prompt_tokens = %d, want 200", est.PromptTokens)
	}
	if est.CompletionTokens != 100 {
		t.Errorf("completion_tokens = %d, want 100", est.CompletionTokens)
	}
}

func TestEstimateLocally(t *testing.T) {
	// English text: ~4 chars per token
	est := EstimateLocally("Hello world, this is a test of the token estimation system", "gpt-4o")
	if est == nil {
		t.Fatal("expected non-nil estimate")
	}
	if est.Source != "estimated" {
		t.Errorf("source = %s, want estimated", est.Source)
	}
	if est.PromptTokens <= 0 {
		t.Errorf("prompt_tokens = %d, want > 0", est.PromptTokens)
	}
	if est.CompletionTokens != 0 {
		t.Errorf("completion_tokens should be 0 for local estimate, got %d", est.CompletionTokens)
	}
}

func TestEstimateLocally_CJK(t *testing.T) {
	// CJK text: ~2 chars per token (should yield more tokens per character)
	est := EstimateLocally("你好世界这是一个测试", "gpt-4o")
	if est == nil {
		t.Fatal("expected non-nil estimate")
	}
	if est.PromptTokens <= 0 {
		t.Errorf("prompt_tokens = %d, want > 0", est.PromptTokens)
	}
	// CJK should yield roughly 2x more tokens than same-length ASCII
	asciiEst := EstimateLocally("Hello World Test!!", "gpt-4o")
	// Both strings have ~10 characters but CJK should estimate more tokens
	t.Logf("CJK tokens: %d, ASCII tokens: %d", est.PromptTokens, asciiEst.PromptTokens)
}

func TestEstimateLocally_Empty(t *testing.T) {
	est := EstimateLocally("", "gpt-4o")
	if est == nil {
		t.Fatal("expected non-nil estimate")
	}
	if est.PromptTokens != 0 {
		t.Errorf("prompt_tokens = %d, want 0 for empty string", est.PromptTokens)
	}
}

func TestShouldCountTowardContribution(t *testing.T) {
	tests := []struct {
		name        string
		estimate    *TokenEstimate
		shouldCount bool
	}{
		{
			name: "upstream with request-id",
			estimate: &TokenEstimate{
				TotalTokens:  100,
				Source:       "upstream",
				HasRequestID: true,
			},
			shouldCount: true,
		},
		{
			name: "upstream without request-id",
			estimate: &TokenEstimate{
				TotalTokens:  100,
				Source:       "upstream",
				HasRequestID: false,
			},
			shouldCount: false,
		},
		{
			name: "estimated without request-id",
			estimate: &TokenEstimate{
				TotalTokens:  100,
				Source:       "estimated",
				HasRequestID: false,
			},
			shouldCount: false,
		},
		{
			name: "zero tokens",
			estimate: &TokenEstimate{
				TotalTokens:  0,
				Source:       "upstream",
				HasRequestID: true,
			},
			shouldCount: false,
		},
		{
			name:        "nil estimate",
			estimate:    nil,
			shouldCount: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.estimate.ShouldCountTowardContribution()
			if got != tt.shouldCount {
				t.Errorf("ShouldCountTowardContribution() = %v, want %v", got, tt.shouldCount)
			}
		})
	}
}

func TestResolveTokenEstimate_UpstreamPreferred(t *testing.T) {
	usage := map[string]interface{}{
		"prompt_tokens":     float64(100),
		"completion_tokens": float64(50),
		"total_tokens":      float64(150),
	}
	est := ResolveTokenEstimate(usage, "some prompt", "gpt-4o", "req-123")
	if est.Source != "upstream" {
		t.Errorf("source = %s, want upstream (upstream preferred)", est.Source)
	}
	if !est.HasRequestID {
		t.Error("HasRequestID should be true")
	}
}

func TestResolveTokenEstimate_FallbackToLocal(t *testing.T) {
	est := ResolveTokenEstimate(nil, "Hello world test prompt", "gpt-4o", "")
	if est.Source != "estimated" {
		t.Errorf("source = %s, want estimated (fallback)", est.Source)
	}
	if est.HasRequestID {
		t.Error("HasRequestID should be false when no request-id")
	}
	if est.PromptTokens <= 0 {
		t.Errorf("prompt_tokens = %d, want > 0", est.PromptTokens)
	}
}

func TestExtractPromptFromMessages(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hello"},
	}
	prompt := ExtractPromptFromMessages(messages)
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if len(prompt) < 10 {
		t.Errorf("prompt too short: %q", prompt)
	}

	// Empty messages
	prompt = ExtractPromptFromMessages(nil)
	if prompt != "" {
		t.Errorf("expected empty prompt for nil messages, got %q", prompt)
	}
}

func TestJsonIntField(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]interface{}
		key  string
		want int
	}{
		{"float64", map[string]interface{}{"x": float64(42)}, "x", 42},
		{"int", map[string]interface{}{"x": 42}, "x", 42},
		{"int64", map[string]interface{}{"x": int64(42)}, "x", 42},
		{"string", map[string]interface{}{"x": "42"}, "x", 42},
		{"missing", map[string]interface{}{"x": 42}, "y", 0},
		{"invalid_string", map[string]interface{}{"x": "abc"}, "x", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonIntField(tt.m, tt.key)
			if got != tt.want {
				t.Errorf("jsonIntField() = %d, want %d", got, tt.want)
			}
		})
	}
}
