package llm

import (
	"reflect"
	"testing"
)

func TestEnrichModelsAPIMissingFieldsFilled(t *testing.T) {
	apiModels := []ModelInfo{
		{
			ID:             "gpt-5.5",
			DisplayName:    "GPT-5.5 live",
			MaxInputTokens: 500000,
			MaxTokens:      64000,
		},
	}
	details := map[string]ModelInfo{
		"gpt-5.5": {
			ID:             "gpt-5.5",
			DisplayName:    "GPT 5.5",
			MaxInputTokens: 900000,
			MaxTokens:      128000,
			Capabilities: ModelCapabilities{
				CodeExecution:  SimpleCapability{Supported: true},
				ThinkingLevels: []string{"none", "low", "medium", "high", "max"},
				Effort: EffortCapabilities{
					Medium: SimpleCapability{Supported: true},
					High:   SimpleCapability{Supported: true},
				},
				EffortLevels: []string{"low", "medium", "high"},
			},
		},
	}

	got := EnrichModels(apiModels, details, ProviderOpenAI)
	if len(got) != 1 {
		t.Fatalf("EnrichModels() returned %d models, want 1", len(got))
	}
	m := got[0]

	// API values win where present.
	if m.DisplayName != "GPT-5.5 live" {
		t.Errorf("DisplayName = %q, want API value", m.DisplayName)
	}
	if m.MaxInputTokens != 500000 {
		t.Errorf("MaxInputTokens = %d, want API value 500000", m.MaxInputTokens)
	}
	if m.MaxTokens != 64000 {
		t.Errorf("MaxTokens = %d, want API value 64000", m.MaxTokens)
	}

	// Capabilities the API left empty come from llmdetails.json.
	if !reflect.DeepEqual(m.Capabilities.ThinkingLevels, []string{"none", "low", "medium", "high", "max"}) {
		t.Errorf("ThinkingLevels = %v, want details from llmdetails.json", m.Capabilities.ThinkingLevels)
	}
	if !m.Capabilities.Effort.High.Supported {
		t.Error("Effort.High should be filled from llmdetails.json")
	}
	if !m.ThinkingHasNone() {
		t.Error("ThinkingHasNone() should be true when 'none' comes from llmdetails.json")
	}
}

func TestEnrichModelsAPICapabilitiesTakePriority(t *testing.T) {
	apiModels := []ModelInfo{
		{
			ID:   "provider-custom-model",
			Slug: "provider-custom-model",
			Capabilities: ModelCapabilities{
				ThinkingLevels: []string{"enabled", "adaptive"},
				Effort: EffortCapabilities{
					Medium: SimpleCapability{Supported: true},
				},
				EffortLevels: []string{"low", "medium"},
			},
		},
	}
	details := map[string]ModelInfo{
		"provider-custom-model": {
			Capabilities: ModelCapabilities{
				CodeExecution:  SimpleCapability{Supported: true},
				ThinkingLevels: []string{"none", "high"},
				Effort: EffortCapabilities{
					High: SimpleCapability{Supported: true},
				},
				EffortLevels: []string{"max"},
			},
		},
	}

	got := EnrichModels(apiModels, details, ProviderCustom)
	if len(got) != 1 {
		t.Fatalf("EnrichModels() returned %d models, want 1", len(got))
	}
	m := got[0]

	// The API's own levels are kept instead of llmdetails.json's.
	if !reflect.DeepEqual(m.Capabilities.ThinkingLevels, []string{"enabled", "adaptive"}) {
		t.Errorf("ThinkingLevels = %v, want API levels", m.Capabilities.ThinkingLevels)
	}
	if !reflect.DeepEqual(m.Capabilities.EffortLevels, []string{"low", "medium"}) {
		t.Errorf("EffortLevels = %v, want API levels", m.Capabilities.EffortLevels)
	}
	if m.Capabilities.Effort.Medium.Supported != true {
		t.Error("Effort.Medium should stay true from the API response")
	}
	if m.Capabilities.Effort.High.Supported {
		t.Error("Effort.High must not leak in from llmdetails.json when the API defines effort levels")
	}
	if !m.Capabilities.Thinking.Types.Enabled.Supported || !m.Capabilities.Thinking.Types.Adaptive.Supported {
		t.Error("Thinking types enabled/adaptive should stay from the API response")
	}

	// Simple capabilities the API does not report still fill from llmdetails.json.
	if !m.Capabilities.CodeExecution.Supported {
		t.Error("CodeExecution should be filled from llmdetails.json")
	}
}

func TestEnrichModelsUnmatchedModelStays(t *testing.T) {
	apiModels := []ModelInfo{
		{ID: "brand-new-model", DisplayName: "Brand New", MaxInputTokens: 424242},
	}
	details := map[string]ModelInfo{
		"unrelated": {ID: "unrelated"},
	}

	got := EnrichModels(apiModels, details, ProviderOpenAI)
	if len(got) != 1 {
		t.Fatalf("EnrichModels() returned %d models, want 1", len(got))
	}
	if got[0].ID != "brand-new-model" || got[0].MaxInputTokens != 424242 {
		t.Errorf("unmatched API model was altered: %+v", got[0])
	}
}

func TestEnrichModelsMatchesBySlug(t *testing.T) {
	// The details map is keyed only by the platform slug, not the API ID.
	details := map[string]ModelInfo{
		"deepseek-ai/deepseek-v4-pro": {
			ID:             "deepseek-v4-pro",
			Slug:           "deepseek-ai/deepseek-v4-pro",
			DisplayName:    "DeepSeek V4 Pro",
			MaxInputTokens: 1000000,
			Capabilities: ModelCapabilities{
				ThinkingLevels: []string{"none", "high", "max"},
			},
		},
	}
	apiModels := []ModelInfo{
		{ID: "deepseek-v4-pro", Slug: "deepseek-ai/deepseek-v4-pro"},
	}

	got := EnrichModels(apiModels, details, ProviderCustom)
	if len(got) != 1 {
		t.Fatalf("EnrichModels() returned %d models, want 1", len(got))
	}
	m := got[0]
	if m.DisplayName != "DeepSeek V4 Pro" {
		t.Errorf("DisplayName = %q, want filled via slug match", m.DisplayName)
	}
	if m.MaxInputTokens != 1000000 {
		t.Errorf("MaxInputTokens = %d, want filled via slug match", m.MaxInputTokens)
	}
	if !reflect.DeepEqual(m.Capabilities.ThinkingLevels, []string{"none", "high", "max"}) {
		t.Errorf("ThinkingLevels = %v, want filled via slug match", m.Capabilities.ThinkingLevels)
	}
}

func TestEnrichModelsNoDetailsNoChange(t *testing.T) {
	apiModels := []ModelInfo{{ID: "some-model", MaxInputTokens: 12345}}
	got := EnrichModels(apiModels, nil, ProviderOpenAI)
	if len(got) != 1 {
		t.Fatalf("EnrichModels() returned %d models, want 1", len(got))
	}
	if got[0].MaxInputTokens != 12345 {
		t.Errorf("MaxInputTokens = %d, want unchanged 12345", got[0].MaxInputTokens)
	}
}
