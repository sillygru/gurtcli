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
