package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/ui"
)

func retryModel() model {
	m := model{
		theme:        ui.ThemeRegistry[0].NewFunc(),
		width:        100,
		height:       40,
		chatViewport: viewport.New(),
		chatInput:    textarea.New(),
		toolExec:     &toolExecState{},
		streamState:  &streamState{},
		state:        stateChat,
		isStreaming:  true,
	}
	m.chatViewport.SetWidth(100)
	return m
}

// A retryable failure must arm a retry rather than dropping an error into the
// transcript, and must discard whatever the dead stream had already produced.
func TestChatStreamErrorSchedulesRetry(t *testing.T) {
	m := retryModel()
	m.streamingContent = new(strings.Builder)
	m.streamingContent.WriteString("half an answer")

	next, cmd := m.Update(chatStreamError{err: &llm.APIError{StatusCode: 503, Body: "bad gateway"}})
	got := next.(model)

	if !got.retry.active {
		t.Fatal("expected a retry to be armed")
	}
	if got.retry.attempt != 1 {
		t.Errorf("attempt = %d, want 1", got.retry.attempt)
	}
	if got.streamingContent != nil {
		t.Error("partial stream content should be discarded before retrying")
	}
	if !got.isStreaming {
		t.Error("isStreaming must stay true so esc/ctrl+c still cancel")
	}
	if len(got.messages) != 0 {
		t.Errorf("no error message should be appended yet, got %d messages", len(got.messages))
	}
	if cmd == nil {
		t.Error("expected a command scheduling the retry")
	}
	// First attempt sits in the 3s +/-20% band.
	if got.retry.delay < 2400*time.Millisecond || got.retry.delay > 3600*time.Millisecond {
		t.Errorf("first delay = %v, want ~3s", got.retry.delay)
	}
}

// The delay ladder climbs across consecutive failures and stops at the cap.
func TestRetryLadderAndExhaustion(t *testing.T) {
	m := retryModel()
	fail := chatStreamError{err: &llm.APIError{StatusCode: 500, Body: "boom"}}

	var prev time.Duration
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		next, _ := m.Update(fail)
		m = next.(model)
		if !m.retry.active {
			t.Fatalf("attempt %d: expected retry to stay armed", attempt)
		}
		if m.retry.attempt != attempt {
			t.Fatalf("attempt counter = %d, want %d", m.retry.attempt, attempt)
		}
		if attempt > 1 && attempt <= 5 && m.retry.delay <= prev {
			t.Errorf("attempt %d: delay %v did not grow past %v", attempt, m.retry.delay, prev)
		}
		if m.retry.delay > 72*time.Second {
			t.Errorf("attempt %d: delay %v exceeded the capped band", attempt, m.retry.delay)
		}
		prev = m.retry.delay
		// Simulate the retry firing so the next failure is a fresh attempt.
		m.retry.active = false
		m.isStreaming = true
	}

	// One more failure exhausts the budget and surfaces the error.
	next, _ := m.Update(fail)
	m = next.(model)
	if m.retry.active {
		t.Error("retry should be exhausted")
	}
	if len(m.messages) != 1 {
		t.Fatalf("expected the error in the transcript, got %d messages", len(m.messages))
	}
	if !strings.Contains(m.messages[0].Content, "after 8 retries") {
		t.Errorf("error should report the attempt count, got %q", m.messages[0].Content)
	}
	if m.isStreaming {
		t.Error("streaming state should be reset once retries are exhausted")
	}
}

// A provider that says when the limit resets overrides the backoff schedule.
func TestRetryHonorsProviderResetTime(t *testing.T) {
	m := retryModel()
	err := &llm.APIError{StatusCode: 429, Body: "slow down", RetryAfter: 45 * time.Second, HasHint: true}

	next, _ := m.Update(chatStreamError{err: err})
	got := next.(model)

	if want := 46 * time.Second; got.retry.delay != want {
		t.Errorf("delay = %v, want %v (reset + 1s margin)", got.retry.delay, want)
	}
	if !got.retry.rateLimit {
		t.Error("429 should be flagged as a rate limit")
	}
	if got.retry.needsOK {
		t.Error("a 46s wait is below the confirm threshold")
	}
}

// Multi-hour resets park until the user confirms rather than silently hanging.
func TestLongResetWaitsForConfirmation(t *testing.T) {
	m := retryModel()
	err := &llm.APIError{StatusCode: 429, Body: "usage limit", RetryAfter: 5 * time.Hour, HasHint: true}

	next, _ := m.Update(chatStreamError{err: err})
	m = next.(model)

	if !m.retry.needsOK {
		t.Fatal("a 5h wait should require confirmation")
	}

	status := m.renderRetryStatus()
	for _, want := range []string{"Rate limited", "5h", "enter/r", "esc"} {
		if !strings.Contains(status, want) {
			t.Errorf("status line %q missing %q", status, want)
		}
	}

	// A stale fire must not start the request while it is still parked.
	next, _ = m.Update(retryFireMsg{token: m.retry.token})
	if !next.(model).retry.needsOK {
		t.Error("retry fired despite awaiting confirmation")
	}
}

// Cancelling invalidates any retry already scheduled with tea.Tick.
func TestCancelInvalidatesScheduledRetry(t *testing.T) {
	m := retryModel()
	next, _ := m.Update(chatStreamError{err: &llm.APIError{StatusCode: 503}})
	m = next.(model)
	staleToken := m.retry.token

	m = m.resetStreamingState()
	if m.retry.active {
		t.Error("resetStreamingState should clear the retry")
	}

	next, cmd := m.Update(retryFireMsg{token: staleToken})
	if cmd != nil {
		t.Error("a stale retry fire must not restart the request")
	}
	if next.(model).retry.active {
		t.Error("stale fire re-armed the retry")
	}
}

// Errors that cannot succeed on repetition fail immediately.
func TestNonRetryableFailsImmediately(t *testing.T) {
	for _, err := range []error{
		&llm.APIError{StatusCode: 401, Body: "invalid api key"},
		&llm.APIError{StatusCode: 400, Body: "bad request"},
	} {
		m := retryModel()
		next, _ := m.Update(chatStreamError{err: err})
		got := next.(model)
		if got.retry.active {
			t.Errorf("%v should not be retried", err)
		}
		if len(got.messages) != 1 || !strings.Contains(got.messages[0].Content, "_Error:") {
			t.Errorf("%v should surface immediately, got %+v", err, got.messages)
		}
	}
}

// A user-cancelled stream is never retried.
func TestCancelledStreamIsNotRetried(t *testing.T) {
	m := retryModel()
	m.cancelRequested = true
	next, _ := m.Update(chatStreamError{err: context.Canceled})
	got := next.(model)
	if got.retry.active {
		t.Error("a cancelled request must not be retried")
	}
	if len(got.messages) != 0 {
		t.Error("cancellation should not append an error message here")
	}
}

// A completed stream clears the ladder so the next failure starts at 3s.
func TestSuccessfulStreamResetsLadder(t *testing.T) {
	m := retryModel()
	next, _ := m.Update(chatStreamError{err: &llm.APIError{StatusCode: 503}})
	m = next.(model)
	m.retry.active = false
	m.isStreaming = true
	m.streamingContent = new(strings.Builder)
	m.streamingContent.WriteString("a complete answer")

	next, _ = m.Update(chatStreamDone{})
	m = next.(model)
	if m.retry.attempt != 0 {
		t.Errorf("attempt = %d, want 0 after a successful stream", m.retry.attempt)
	}
}

func TestRenderRetryStatusCountdown(t *testing.T) {
	m := retryModel()
	m.retry = retryState{
		active:  true,
		attempt: 2,
		delay:   6 * time.Second,
		until:   time.Now().Add(6 * time.Second),
	}
	status := m.renderRetryStatus()
	for _, want := range []string{"Failed", "retrying in", "6s", "attempt 2/8"} {
		if !strings.Contains(status, want) {
			t.Errorf("status line %q missing %q", status, want)
		}
	}
}

func TestFormatRetryWait(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{-time.Second, "0s"},
		{900 * time.Millisecond, "1s"},
		{45 * time.Second, "45s"},
		{time.Minute, "1m"},
		{4*time.Minute + 12*time.Second, "4m12s"},
		{5 * time.Hour, "5h"},
		{4*time.Hour + 58*time.Minute, "4h58m"},
	}
	for _, tt := range tests {
		if got := formatRetryWait(tt.in); got != tt.want {
			t.Errorf("formatRetryWait(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
