package llm

import "testing"

func TestDisplayName(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{ProviderOpenAI, "OpenAI"},
		{ProviderAnthropic, "Anthropic"},
		{ProviderCustom, "Custom (OpenAI-compatible)"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		got := DisplayName(tt.provider)
		if got != tt.want {
			t.Errorf("DisplayName(%q) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}

func TestDefaultBaseURL(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{ProviderOpenAI, "https://api.openai.com/v1"},
		{ProviderAnthropic, "https://api.anthropic.com/v1"},
		{ProviderCustom, ""},
		{"unknown", ""},
	}

	for _, tt := range tests {
		got := DefaultBaseURL(tt.provider)
		if got != tt.want {
			t.Errorf("DefaultBaseURL(%q) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}

func TestProvidersList(t *testing.T) {
	if len(Providers) != 3 {
		t.Errorf("len(Providers) = %d, want 3", len(Providers))
	}
	want := []string{"openai", "anthropic", "custom"}
	for i, p := range Providers {
		if p != want[i] {
			t.Errorf("Providers[%d] = %q, want %q", i, p, want[i])
		}
	}
}
