package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUtf8Tail(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "empty string", s: "", n: 10, want: ""},
		{name: "zero n", s: "hello", n: 0, want: ""},
		{name: "n larger than string", s: "hi", n: 10, want: "hi"},
		{name: "n equals string length", s: "abc", n: 3, want: "abc"},
		{name: "ascii tail", s: "0123456789", n: 4, want: "6789"},
		{name: "tail starts on rune boundary", s: "héllo wörld", n: 6, want: "wörld"},
		{name: "skips orphan continuation byte", s: "héllo wörld", n: 4, want: "rld"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := utf8Tail(tt.s, tt.n); got != tt.want {
				t.Errorf("utf8Tail(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

func TestRunBashTruncatesToTail(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// 20000 bytes of "a\n" + "ENDMARKER" = 20009 bytes > default limit.
	result, err := RunBash(ctx, "yes a | head -c 20000; printf 'ENDMARKER'", 30000, DefaultMaxOutputChars, "sess-1", dir)
	if err != nil {
		t.Fatalf("RunBash: %v", err)
	}

	if !strings.Contains(result, "output exceeded 20000 characters") {
		t.Errorf("expected truncation hint, got:\n%s", result)
	}
	if !strings.Contains(result, "showing the tail") {
		t.Errorf("expected the hint to say it is showing a tail, got:\n%s", result)
	}
	if !strings.HasSuffix(result, "ENDMARKER") {
		t.Errorf("expected the tail (with ENDMARKER) to be kept, got:\n%s", result)
	}
	if strings.Contains(result, strings.Repeat("a", 20000)) {
		t.Error("expected the head to be dropped, but a full 20000-char run is present")
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.out"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected exactly one saved output file, got %v (err %v)", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("reading saved output: %v", err)
	}
	if len(data) != 20009 {
		t.Errorf("saved output length = %d, want 20009", len(data))
	}
	if !strings.HasSuffix(string(data), "ENDMARKER") {
		t.Error("saved output should contain the full untruncated output")
	}
}

func TestRunBashCustomOutputLimit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// 300 bytes of "x\n" + "TAIL" = 304 bytes; limit of 100 keeps only the tail.
	result, err := RunBash(ctx, "yes x | head -c 300; printf 'TAIL'", 30000, 100, "sess-2", dir)
	if err != nil {
		t.Fatalf("RunBash: %v", err)
	}

	if !strings.Contains(result, "output exceeded 100 characters") {
		t.Fatalf("expected truncation hint, got:\n%s", result)
	}
	if !strings.HasSuffix(result, "TAIL") {
		t.Errorf("expected the tail (with TAIL marker) to be kept, got:\n%s", result)
	}
}

func TestRunBashUnderLimitNotTruncated(t *testing.T) {
	ctx := context.Background()
	result, err := RunBash(ctx, "printf 'short output'", 30000, 100, "", "")
	if err != nil {
		t.Fatalf("RunBash: %v", err)
	}
	if result != "short output" {
		t.Errorf("expected untruncated output, got %q", result)
	}
}

// A failing command used to return its entire output unbounded: truncation only
// ran on the success path, so a stack trace or a full test run came back in
// full and the model reasoned over it. The failure path must be bounded too.
func TestRunBashTruncatesOnFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// Emits well over the 100 char budget, then exits non-zero.
	_, err := RunBash(ctx, "yes e | head -c 5000; printf 'BOOM'; exit 3", 30000, 100, "sess-fail", dir)
	if err == nil {
		t.Fatal("expected a non-zero exit to return an error")
	}

	// The harness discards the separate return value when err != nil, so the
	// model only ever sees what is carried inside the error.
	msg := err.Error()
	if len(msg) > 400 {
		t.Errorf("failure error is %d chars; expected it to be bounded by the output limit, got:\n%s", len(msg), msg)
	}
	if !strings.Contains(msg, "output exceeded 100 characters") {
		t.Errorf("expected the failure to state that output was truncated, got:\n%s", msg)
	}
	if !strings.Contains(msg, "BOOM") {
		t.Errorf("expected the tail (with BOOM) to survive into the error, got:\n%s", msg)
	}

	files, globErr := filepath.Glob(filepath.Join(dir, "*.out"))
	if globErr != nil || len(files) != 1 {
		t.Fatalf("expected the full failing output to be saved to disk, got %v (err %v)", files, globErr)
	}
	data, readErr := os.ReadFile(files[0])
	if readErr != nil {
		t.Fatalf("reading saved output: %v", readErr)
	}
	if !strings.Contains(string(data), "BOOM") {
		t.Error("saved output should contain the full untruncated failing output")
	}
}

// A failed command that produced no output must not claim it truncated
// anything, and must still surface the underlying error.
func TestRunBashFailureWithNoOutputIsUnchanged(t *testing.T) {
	ctx := context.Background()

	_, err := RunBash(ctx, "exit 7", 30000, 100, "", "")
	if err == nil {
		t.Fatal("expected an error for a non-zero exit with no output")
	}
	if strings.Contains(err.Error(), "output exceeded") {
		t.Errorf("must not claim truncation when there was no output, got:\n%s", err.Error())
	}
	if !strings.Contains(err.Error(), "command failed") {
		t.Errorf("expected the command failure to be named, got:\n%s", err.Error())
	}
}

// Truncation must still happen when the output cannot be saved to disk; the
// model needs to know the output was cut either way.
func TestRunBashTruncatesWhenSaveFails(t *testing.T) {
	ctx := context.Background()

	// A sessionID with a path separator makes the save fail inside the
	// per-session output dir, which does not exist and cannot be created at
	// this path.
	_, err := RunBash(ctx, "yes z | head -c 5000; exit 1", 30000, 100, "bad/session", "/nonexistent-root-xyz/outputs")
	if err == nil {
		t.Fatal("expected an error from the non-zero exit")
	}
	if len(err.Error()) > 400 {
		t.Errorf("output should still be bounded when saving fails, got %d chars:\n%s", len(err.Error()), err.Error())
	}
	if !strings.Contains(err.Error(), "output exceeded 100 characters") {
		t.Errorf("expected a truncation notice even without a saved file, got:\n%s", err.Error())
	}
}

// Interrupting a run_bash must kill the whole process tree, not just the sh
// wrapper. The command sleeps before touching the marker, so the marker
// existing after cancel means an orphaned child outlived the interrupt.
func TestRunBashCancellationKillsProcessGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "interrupted-marker")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := RunBash(ctx, fmt.Sprintf("sleep 30 && touch %s", marker), 30000, 100, "", "")
		done <- err
	}()

	// Let the shell spawn before interrupting it.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("RunBash returned nil after the context was cancelled")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunBash did not return after cancel — the process is still running")
	}

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("marker exists — a child process survived the interrupt")
	}
}

// A timeout must also report the output produced before the kill, since that
// is usually the only clue about where the command hung.
func TestRunBashTimeoutCarriesOutput(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	_, err := RunBash(ctx, "printf 'BEFORE_HANG'; sleep 30", 300, DefaultMaxOutputChars, "sess-timeout", dir)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected a timeout message, got:\n%s", err.Error())
	}
	if !strings.Contains(err.Error(), "BEFORE_HANG") {
		t.Errorf("expected output emitted before the hang to be reported, got:\n%s", err.Error())
	}
}
