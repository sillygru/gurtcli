package ui

import "testing"

func TestHighlightLineDiffChangedMiddle(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	got := highlightLineDiff("hello world", "hello there", theme.DiffDel, theme.DiffDelHighlight)
	if got == "" {
		t.Fatal("expected non-empty output")
	}
	if got == theme.DiffDel.Render("hello world") {
		t.Fatalf("expected middle to be highlighted, got plain: %q", got)
	}
}

func TestHighlightLineDiffNoCounterpart(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	got := highlightLineDiff("removed line", "", theme.DiffDel, theme.DiffDelHighlight)
	if got != theme.DiffDelHighlight.Render("removed line") {
		t.Fatalf("expected full line highlight, got: %q", got)
	}
}

func TestCommonPrefixLen(t *testing.T) {
	t.Parallel()
	if got := commonPrefixLen("abc", "abd"); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

func TestRenderDiffAddedLine(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	out := renderDiffAddedLine(theme, "+ ", "foo bar", "foo baz", 40)
	if out == "" {
		t.Fatal("expected output")
	}
}
