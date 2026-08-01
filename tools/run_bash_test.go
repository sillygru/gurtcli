package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	if !strings.Contains(result, "Showing the tail:") {
		t.Errorf("expected truncation hint, got:\n%s", result)
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

	if !strings.Contains(result, "Showing the tail:") {
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
