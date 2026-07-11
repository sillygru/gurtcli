package ui

import (
	"os"
	"strings"
	"testing"
)

func init() {
	os.Setenv("CLICOLOR_FORCE", "1")
}

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

func TestAlignLinesReplace(t *testing.T) {
	t.Parallel()
	oldLines := []string{"a", "b", "c", "d"}
	newLines := []string{"a", "c", "d", "e"}
	rows := alignLines(oldLines, newLines)

	if len(rows) != 5 {
		t.Fatalf("expected 5 aligned rows, got %d", len(rows))
	}
	if rows[0].kind != diffEqual || rows[0].left != "a" {
		t.Fatalf("row 0: expected equal 'a', got %+v", rows[0])
	}
	if rows[1].kind != diffDelete || rows[1].left != "b" {
		t.Fatalf("row 1: expected delete 'b', got %+v", rows[1])
	}
	if rows[2].kind != diffEqual || rows[2].left != "c" {
		t.Fatalf("row 2: expected equal 'c', got %+v", rows[2])
	}
	if rows[3].kind != diffEqual || rows[3].left != "d" {
		t.Fatalf("row 3: expected equal 'd', got %+v", rows[3])
	}
	if rows[4].kind != diffInsert || rows[4].right != "e" {
		t.Fatalf("row 4: expected insert 'e', got %+v", rows[4])
	}
}

func TestRenderEditDiffWide(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	out := RenderEditDiff(theme, "a\nb\nc\nd", "a\nc\nd\ne", 120)
	if !strings.Contains(out, "Before") || !strings.Contains(out, "After") {
		t.Fatalf("expected panel labels, got: %q", out)
	}
	if !strings.Contains(stripANSI(out), "│") {
		t.Fatalf("expected side-by-side gutter, got: %q", out)
	}
}

func TestRenderEditDiffNarrow(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	out := RenderEditDiff(theme, "old\nline", "new\nline", 50)
	if !strings.Contains(out, "Before") || !strings.Contains(out, "After") {
		t.Fatalf("expected stacked sections, got: %q", out)
	}
}
