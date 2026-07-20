package llm

import "testing"

func TestNormalizePromptTokens(t *testing.T) {
	tests := []struct {
		name       string
		prompt     int
		cached     int
		wantTotal  int
		wantCached int
	}{
		{
			// Spec behaviour: cached_tokens is a subset of prompt_tokens.
			name: "cached is subset", prompt: 9000, cached: 8000,
			wantTotal: 9000, wantCached: 8000,
		},
		{
			// Endpoints that report only the uncached remainder: the counts
			// are additive, and cached exceeding prompt is the giveaway.
			name: "cached exceeds prompt", prompt: 1000, cached: 8000,
			wantTotal: 9000, wantCached: 8000,
		},
		{
			// The pathological case from the field report: a small uncached
			// delta against a large warm cache must not shrink the total.
			name: "tiny delta large cache", prompt: 340, cached: 37000,
			wantTotal: 37340, wantCached: 37000,
		},
		{
			name: "no caching", prompt: 1200, cached: 0,
			wantTotal: 1200, wantCached: 0,
		},
		{
			name: "fully cached", prompt: 5000, cached: 5000,
			wantTotal: 5000, wantCached: 5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, cached := normalizePromptTokens(tt.prompt, tt.cached)
			if total != tt.wantTotal || cached != tt.wantCached {
				t.Errorf("normalizePromptTokens(%d, %d) = (%d, %d), want (%d, %d)",
					tt.prompt, tt.cached, total, cached, tt.wantTotal, tt.wantCached)
			}
			if cached > total {
				t.Errorf("cached %d exceeds total %d", cached, total)
			}
		})
	}
}
