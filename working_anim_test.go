package main

import (
	"testing"
	"time"
)

// The working spinner must advance glyphs at a fixed wall-clock rate,
// independent of how often ticks are delivered (a slow model, a chatty one, or
// a backlogged message queue may deliver ticks early or late — the frame shown
// after any delay must still land where real time says it should).
func TestWorkingFrameIdxFixedRate(t *testing.T) {
	if len(workingSpinnerFrames) == 0 {
		t.Fatal("workingSpinnerFrames must not be empty")
	}

	var zero model
	if got := zero.workingFrameIdx(); got != 0 {
		t.Errorf("zero anim start frame = %d, want 0", got)
	}

	tests := []struct {
		elapsed time.Duration
		want    int
	}{
		{0, 0},
		{249 * time.Millisecond, 0},
		{250 * time.Millisecond, 1},
		{500 * time.Millisecond, 2},
		{750 * time.Millisecond, 3},
		{1000 * time.Millisecond, 0}, // wraps around the cycle
		{1250 * time.Millisecond, 1},
	}
	for _, tt := range tests {
		m := model{workingAnimStart: time.Now().Add(-tt.elapsed)}
		if got := m.workingFrameIdx(); got != tt.want {
			t.Errorf("frame at %v = %d, want %d", tt.elapsed, got, tt.want)
		}
	}

	// A future anchor (clock skew) must not produce a negative index.
	m := model{workingAnimStart: time.Now().Add(time.Hour)}
	if got := m.workingFrameIdx(); got != 0 {
		t.Errorf("future animStart frame = %d, want 0", got)
	}
}

// The rotating status message must swap on fixed wall-clock slots since the
// animation started, not on a count of delivered ticks. A model streaming a
// burst of chunks (delaying ticks) or trickling tokens must not speed up or
// slow down how often the message changes.
func TestSyncWorkingMsgWallClock(t *testing.T) {
	t.Parallel()

	now := time.Now()
	anchor := func(elapsed time.Duration) model {
		return model{workingMsg: "Vibing", workingAnimStart: now.Add(-elapsed)}
	}

	// No animation: sync is a no-op.
	var zero model
	m := zero.syncWorkingMsg()
	if m.workingMsg != "" || m.workingMsgSlot != 0 {
		t.Errorf("zero anim: got msg=%q slot=%d, want unchanged", m.workingMsg, m.workingMsgSlot)
	}

	// Future anchor (clock skew): no-op.
	future := model{workingMsg: "Vibing", workingAnimStart: now.Add(time.Hour)}.syncWorkingMsg()
	if future.workingMsg != "Vibing" || future.workingMsgSlot != 0 {
		t.Errorf("future anchor: got msg=%q slot=%d, want unchanged", future.workingMsg, future.workingMsgSlot)
	}

	// Still inside the first 10s slot: message must not change yet.
	m = anchor(9 * time.Second).syncWorkingMsg()
	if m.workingMsg != "Vibing" || m.workingMsgSlot != 0 {
		t.Errorf("within first slot: got msg=%q slot=%d, want unchanged", m.workingMsg, m.workingMsgSlot)
	}

	// Crossing the 10s boundary picks a different message and moves to slot 1.
	m = anchor(10 * time.Second).syncWorkingMsg()
	if m.workingMsg == "Vibing" {
		t.Errorf("slot 1 must pick a fresh message, still got %q", m.workingMsg)
	}
	if m.workingMsgSlot != 1 {
		t.Errorf("slot = %d, want 1", m.workingMsgSlot)
	}
	first := m.workingMsg

	// The 20s boundary advances again.
	m = model{workingMsg: first, workingMsgSlot: 1, workingAnimStart: now.Add(-20 * time.Second)}.syncWorkingMsg()
	if m.workingMsg == first {
		t.Errorf("slot 2 must pick a fresh message, still got %q", m.workingMsg)
	}
	if m.workingMsgSlot != 2 {
		t.Errorf("slot = %d, want 2", m.workingMsgSlot)
	}

	// Repeated syncs within the same slot never re-pick the message: the two
	// render surfaces (spacer + copyzone) must read the same string at every
	// paint, so the rotation must be a pure function of the slot.
	m = anchor(25 * time.Second)
	m = m.syncWorkingMsg()
	same := m.workingMsg
	if same == "" {
		t.Fatal("slot 2 sync produced an empty message")
	}
	m.workingAnimStart = now.Add(-26 * time.Second) // still slot 2
	again := m.syncWorkingMsg()
	if again.workingMsg != same {
		t.Errorf("message changed within a slot: %q -> %q", same, again.workingMsg)
	}
	if again.workingMsgSlot != 2 {
		t.Errorf("slot = %d, want 2", again.workingMsgSlot)
	}
}

// randomWorkingMessage must not hand back the excluded message, but must do the
// only legal thing when the pool is a singleton.
func TestRandomWorkingMessage(t *testing.T) {
	t.Parallel()

	if len(workingMessages) == 0 {
		t.Fatal("workingMessages must not be empty")
	}
	for excludeIdx := range workingMessages {
		exclude := workingMessages[excludeIdx]
		for i := 0; i < 50; i++ {
			if got := randomWorkingMessage(exclude); got == exclude {
				t.Fatalf("randomWorkingMessage(%q) returned the excluded message", exclude)
			}
		}
	}
}
