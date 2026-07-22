package main

import (
	"testing"
	"time"

	"github.com/sillygru/gurtcli/llm"
)

func TestLookupReasoningMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in     string
		want   reasoningMode
		wantOK bool
	}{
		{"expanded", reasoningModeExpanded, true},
		{"collapsed", reasoningModeCollapsed, true},
		{"auto", reasoningModeAuto, true},
		{"  AUTO  ", reasoningModeAuto, true},
		// The words the command took before modes existed.
		{"true", reasoningModeExpanded, true},
		{"yes", reasoningModeExpanded, true},
		{"false", reasoningModeCollapsed, true},
		{"no", reasoningModeCollapsed, true},
		// A typo must be rejected, not resolved to some default that silently
		// changes the user's setting.
		{"", "", false},
		{"expandedd", "", false},
		{"maybe", "", false},
	}
	for _, tt := range tests {
		got, ok := lookupReasoningMode(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("lookupReasoningMode(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

// An empty or unrecognized stored value predates modes, so the old boolean has
// to decide. This is how existing configs and session rows migrate.
func TestParseReasoningModeFallsBackToLegacyBool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in     string
		legacy bool
		want   reasoningMode
	}{
		{"", true, reasoningModeExpanded},
		{"", false, reasoningModeCollapsed},
		{"garbage", true, reasoningModeExpanded},
		{"garbage", false, reasoningModeCollapsed},
		// A real mode always wins over the legacy bool.
		{"auto", true, reasoningModeAuto},
		{"collapsed", true, reasoningModeCollapsed},
		{"expanded", false, reasoningModeExpanded},
	}
	for _, tt := range tests {
		if got := parseReasoningMode(tt.in, tt.legacy); got != tt.want {
			t.Errorf("parseReasoningMode(%q, %v) = %q; want %q", tt.in, tt.legacy, got, tt.want)
		}
	}
}

func TestReasoningModeVisibility(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode        reasoningMode
		whileActive bool
		whenStored  bool
	}{
		{reasoningModeExpanded, true, true},
		{reasoningModeCollapsed, false, false},
		// Auto is the whole point: open while thinking, closed once it is done.
		{reasoningModeAuto, true, false},
	}
	for _, tt := range tests {
		if got := tt.mode.visibleWhileActive(); got != tt.whileActive {
			t.Errorf("%s.visibleWhileActive() = %v; want %v", tt.mode, got, tt.whileActive)
		}
		if got := tt.mode.visibleWhenStored(); got != tt.whenStored {
			t.Errorf("%s.visibleWhenStored() = %v; want %v", tt.mode, got, tt.whenStored)
		}
	}
}

// Switching modes re-applies to blocks already on screen, which is what
// "always expanded" and "always collapsed" mean.
func TestApplyReasoningModeRewritesExistingBlocks(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		mode reasoningMode
		want bool
	}{
		{reasoningModeExpanded, true},
		{reasoningModeCollapsed, false},
		{reasoningModeAuto, false},
	} {
		m := model{messages: []llm.Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "a", Reasoning: "thought a", ReasoningVisible: false},
			{Role: "assistant", Content: "b", Reasoning: "thought b", ReasoningVisible: true},
			{Role: "assistant", Content: "c"},
		}}

		got := m.applyReasoningMode(tt.mode)

		if got.reasoning.mode != tt.mode {
			t.Fatalf("mode = %q; want %q", got.reasoning.mode, tt.mode)
		}
		for i, msg := range got.messages {
			if msg.Reasoning == "" {
				// A message with no reasoning must not sprout a visibility flag.
				if msg.ReasoningVisible {
					t.Errorf("%s: message %d has no reasoning but was marked visible", tt.mode, i)
				}
				continue
			}
			if msg.ReasoningVisible != tt.want {
				t.Errorf("%s: message %d visible = %v; want %v", tt.mode, i, msg.ReasoningVisible, tt.want)
			}
		}
	}
}

// The transcript caches finalized messages, so re-applying a mode without
// invalidating it would leave the old blocks rendered exactly as they were.
func TestApplyReasoningModeInvalidatesTranscriptCache(t *testing.T) {
	t.Parallel()
	m := model{
		messages:               []llm.Message{{Role: "assistant", Content: "a", Reasoning: "t"}},
		transcriptCacheContent: "stale render",
		transcriptCacheUpTo:    1,
		transcriptCachedKey:    "some-key",
	}

	got := m.applyReasoningMode(reasoningModeExpanded)

	if got.transcriptCacheContent != "" || got.transcriptCacheUpTo != 0 {
		t.Fatalf("cache survived: %q up to %d", got.transcriptCacheContent, got.transcriptCacheUpTo)
	}
}

// In auto mode the first answer token ends the thinking block: it stops being
// active, folds away, and keeps the elapsed time it had accumulated.
func TestAutoModeCollapsesWhenAnswerStarts(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		mode        reasoningMode
		wantActive  bool
		wantVisible bool
	}{
		{reasoningModeAuto, false, false},
		// The other two modes leave the live block exactly as it was.
		{reasoningModeExpanded, true, true},
		{reasoningModeCollapsed, true, false},
	} {
		m := model{}
		m.reasoning.mode = tt.mode
		m.reasoning.active = true
		m.reasoning.visible = tt.mode.visibleWhileActive()
		m.reasoning.startTime = time.Now().Add(-3 * time.Second)

		next, _ := m.Update(chatStreamChunk{content: "here is the answer"})
		got := next.(model)

		if got.reasoning.active != tt.wantActive {
			t.Errorf("%s: active = %v; want %v", tt.mode, got.reasoning.active, tt.wantActive)
		}
		if got.reasoning.visible != tt.wantVisible {
			t.Errorf("%s: visible = %v; want %v", tt.mode, got.reasoning.visible, tt.wantVisible)
		}
		if tt.mode == reasoningModeAuto && got.reasoning.duration < 2*time.Second {
			t.Errorf("%s: duration = %v; want the elapsed time to be kept", tt.mode, got.reasoning.duration)
		}
	}
}

// A block that resumes after interleaved answer text keeps counting from where
// it left off instead of restarting its clock at zero.
func TestReasoningClockResumesAfterInterleavedContent(t *testing.T) {
	t.Parallel()
	m := model{}
	m.reasoning.mode = reasoningModeAuto
	m.reasoning.duration = 5 * time.Second

	next, _ := m.Update(chatStreamReasoning{content: "more thinking"})
	got := next.(model)

	if !got.reasoning.active {
		t.Fatal("reasoning should be active again")
	}
	if elapsed := time.Since(got.reasoning.startTime); elapsed < 5*time.Second {
		t.Fatalf("elapsed = %v; want the earlier 5s to be carried forward", elapsed)
	}
}
