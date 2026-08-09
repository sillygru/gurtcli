package main

import "testing"

// Usage events accumulate into session-lifetime sums, and the input total
// always uses the normalized full-prompt count (cached portions included), so
// no endpoint convention can drop or double-count input.
func TestChatStreamUsageAccumulatesTokens(t *testing.T) {
	m := retryModel()

	// Anthropic-style first request: base input + cache read + cache write.
	next, _ := m.Update(chatStreamUsage{
		outputTokens:      200,
		reasoningTokens:   50,
		cacheHitTokens:    8000,
		cacheWriteTokens:  1000,
		promptTotalTokens: 10000,
	})
	m = next.(model)

	if m.inputTokens != 10000 {
		t.Errorf("inputTokens = %d, want 10000 (full prompt incl. cache)", m.inputTokens)
	}
	if m.cacheHitTokens != 8000 {
		t.Errorf("cacheHitTokens = %d, want 8000", m.cacheHitTokens)
	}
	if m.cacheWriteTokens != 1000 {
		t.Errorf("cacheWriteTokens = %d, want 1000", m.cacheWriteTokens)
	}
	if m.outputTokens != 200 || m.reasoningOutputTokens != 50 {
		t.Errorf("output = %d, reasoning = %d, want 200/50", m.outputTokens, m.reasoningOutputTokens)
	}
	if m.contextInputTokens != 10000 {
		t.Errorf("contextInputTokens = %d, want 10000", m.contextInputTokens)
	}

	// A warm-cache request: the raw input is only the uncached delta, but the
	// lifetime input sum grows by the full prompt again.
	next, _ = m.Update(chatStreamUsage{
		cacheHitTokens:    9500,
		promptTotalTokens: 10000,
	})
	m = next.(model)
	if m.inputTokens != 20000 {
		t.Errorf("inputTokens after second request = %d, want 20000", m.inputTokens)
	}
	if m.cacheHitTokens != 17500 {
		t.Errorf("cacheHitTokens after second request = %d, want 17500", m.cacheHitTokens)
	}
	if m.cacheWriteTokens != 1000 {
		t.Errorf("cacheWriteTokens should not grow on a cache-hit request, got %d", m.cacheWriteTokens)
	}

	// Output-only events (Anthropic message_delta) must not disturb input sums.
	next, _ = m.Update(chatStreamUsage{outputTokens: 75})
	m = next.(model)
	if m.inputTokens != 20000 || m.outputTokens != 275 {
		t.Errorf("after output-only event: input = %d, output = %d, want 20000/275",
			m.inputTokens, m.outputTokens)
	}
}

// toSession persists the cache counters and applySession restores them, so a
// resumed session keeps its full lifetime sums instead of starting over.
func TestSessionRoundTripsCacheWriteTokens(t *testing.T) {
	m := retryModel()
	next, _ := m.Update(chatStreamUsage{
		outputTokens:      200,
		cacheHitTokens:    8000,
		cacheWriteTokens:  1000,
		promptTotalTokens: 10000,
	})
	m = next.(model)

	sess := m.toSession()
	if sess.InputTokens != 10000 {
		t.Errorf("toSession.InputTokens = %d, want 10000", sess.InputTokens)
	}
	if sess.CacheHitTokens != 8000 {
		t.Errorf("toSession.CacheHitTokens = %d, want 8000", sess.CacheHitTokens)
	}
	if sess.CacheWriteTokens != 1000 {
		t.Errorf("toSession.CacheWriteTokens = %d, want 1000", sess.CacheWriteTokens)
	}

	restored := retryModel().applySession(sess)
	if restored.inputTokens != 10000 {
		t.Errorf("applySession.inputTokens = %d, want 10000", restored.inputTokens)
	}
	if restored.cacheHitTokens != 8000 {
		t.Errorf("applySession.cacheHitTokens = %d, want 8000", restored.cacheHitTokens)
	}
	if restored.cacheWriteTokens != 1000 {
		t.Errorf("applySession.cacheWriteTokens = %d, want 1000", restored.cacheWriteTokens)
	}
}

// The OpenAI convention where prompt_tokens excludes cached tokens must still
// land the whole prompt in the lifetime input sum (normalized upstream), never
// the raw remainder alone.
func TestChatStreamUsageUsesNormalizedPromptTotal(t *testing.T) {
	m := retryModel()

	next, _ := m.Update(chatStreamUsage{
		cacheHitTokens:    8000,
		promptTotalTokens: 9000,
	})
	m = next.(model)

	if m.inputTokens != 9000 {
		t.Errorf("inputTokens = %d, want 9000 (cached 8000 folded into the total)", m.inputTokens)
	}
	if m.cacheHitTokens != 8000 {
		t.Errorf("cacheHitTokens = %d, want 8000", m.cacheHitTokens)
	}
}
