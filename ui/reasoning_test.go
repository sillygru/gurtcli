package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// The block used to be drawn inside a rounded border. Nothing may reintroduce
// one: the gutter rule is the only structure an expanded block gets.
func TestRenderReasoningHasNoBox(t *testing.T) {
	t.Parallel()
	out := RenderReasoning(DefaultTheme(), false, true, 2*time.Second, "one\ntwo", 80)
	for _, glyph := range []string{"╭", "╮", "╰", "╯", "─"} {
		if strings.Contains(out, glyph) {
			t.Fatalf("expanded block still draws border glyph %q:\n%s", glyph, out)
		}
	}
}

// Every body line carries the gutter, including the continuation lines a wrap
// produces — the transcript's FitWidth backstop would not put them back.
func TestRenderReasoningGuttersEveryWrappedLine(t *testing.T) {
	t.Parallel()
	const width = 30
	content := "a short line\n" + strings.Repeat("wrapme ", 20) + "\n" + strings.Repeat("x", 90)

	out := RenderReasoning(DefaultTheme(), false, true, time.Second, content, width)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected the long lines to wrap, got %d lines:\n%s", len(lines), out)
	}

	for i, line := range lines {
		plain := ansi.Strip(line)
		if i == 0 {
			// The header row is the toggle, not body.
			if !strings.Contains(plain, "thought for") {
				t.Fatalf("line 0 should be the header, got %q", plain)
			}
			continue
		}
		if !strings.HasPrefix(plain, "  │ ") {
			t.Fatalf("body line %d lost its gutter: %q", i, plain)
		}
		if got := ansi.StringWidth(plain); got > width {
			t.Fatalf("body line %d is %d cells wide, over the %d budget: %q", i, got, width, plain)
		}
	}
}

// Collapsed is collapsed regardless of how much content is buffered.
func TestRenderReasoningCollapsedIsHeaderOnly(t *testing.T) {
	t.Parallel()
	out := RenderReasoning(DefaultTheme(), true, false, time.Second, "secret thoughts", 80)
	if strings.Contains(ansi.Strip(out), "secret thoughts") {
		t.Fatalf("collapsed block leaked its content:\n%s", out)
	}
	if strings.Contains(out, "\n") {
		t.Fatalf("collapsed block should be a single row, got:\n%s", out)
	}
}

// A terminal narrow enough that the gutter alone fills it must still produce
// rows rather than dividing the remaining width down to zero.
func TestRenderReasoningSurvivesTinyWidth(t *testing.T) {
	t.Parallel()
	for _, width := range []int{1, 2, 4, 5} {
		out := RenderReasoning(DefaultTheme(), false, true, time.Second, "hello there", width)
		if out == "" {
			t.Fatalf("width %d rendered nothing", width)
		}
	}
}
