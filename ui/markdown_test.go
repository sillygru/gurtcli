package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestMarkdownHeading1(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	out := RenderAssistantContent(theme, "# Hello World", 80, nil)
	if !strings.Contains(out, "Hello World") {
		t.Fatalf("expected heading text, got: %q", out)
	}
	// Bold/mauve styling produces ANSI codes
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected styled heading output, got: %q", out)
	}
}

func TestMarkdownBold(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	out := RenderAssistantContent(theme, "This is **bold** text", 80, nil)
	if !strings.Contains(out, "bold") {
		t.Fatalf("expected bold text, got: %q", out)
	}
}

func TestMarkdownCodeFence(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	content := "```go\nfmt.Println(\"hi\")\n```"
	out := RenderAssistantContent(theme, content, 80, nil)
	if !strings.Contains(out, "fmt.Println") {
		t.Fatalf("expected code block content, got: %q", out)
	}
}

func TestMarkdownBulletList(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	out := RenderAssistantContent(theme, "- first item\n- second item", 80, nil)
	if !strings.Contains(out, "first item") || !strings.Contains(out, "second item") {
		t.Fatalf("expected list items, got: %q", out)
	}
	if !strings.Contains(out, "•") {
		t.Fatalf("expected bullet marker, got: %q", out)
	}
}

func TestMarkdownWithTable(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	content := "# Results\n\n| A | B |\n|---|---|\n| 1 | 2 |"
	out := RenderAssistantContent(theme, content, 80, nil)
	if !strings.Contains(out, "Results") {
		t.Fatal("expected heading")
	}
	if !strings.Contains(out, "╭") {
		t.Fatal("expected table border")
	}
}

// renderWidths are the content widths the transcript is rendered at, from a
// phone over SSH up to a full-screen terminal.
var renderWidths = []int{16, 26, 36, 41, 56, 76, 116, 196}

// widestLine returns the widest rendered row in cells.
func widestLine(s string) (int, string) {
	widest, at := 0, ""
	for _, line := range strings.Split(s, "\n") {
		if w := lipgloss.Width(line); w > widest {
			widest, at = w, stripANSI(line)
		}
	}
	return widest, at
}

// Assistant markdown goes straight into a viewport that cuts anything wider
// than it and then lets the user pan across the cut. It has to fit on its own —
// the FitWidth backstop in the transcript builder is a safety net, not the
// mechanism.
func TestMarkdownFitsEveryWidth(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	content := "# A heading that is much too long to fit inside a narrow terminal window\n\n" +
		"A paragraph with a very long unbroken path /Volumes/KINGSTON/code/projects/gurtcli/ui/markdown_test.go in it.\n\n" +
		"- a bullet item whose text runs well past the right edge of a narrow terminal\n\n" +
		"1. a numbered item that also runs well past the right edge of a narrow terminal\n\n" +
		"这是一段中文文本，每个字符占据两个终端单元格。\n\n" +
		"```go\nfunc (m model) somethingWithAVeryLongSignature(ctx context.Context, opts ...RenderOption) (string, error)\n```\n"

	for _, width := range renderWidths {
		out := RenderAssistantContent(theme, content, width, nil)
		if got, line := widestLine(out); got > width {
			t.Errorf("markdown at width %d: widest row is %d cells: %q", width, got, line)
		}
	}
}

// Tables are the widest thing markdown can produce, and the only block whose
// natural width has nothing to do with the terminal's.
func TestTableFitsEveryWidth(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	block := "| Setting | Default | What it changes about the way the transcript is rendered |\n" +
		"|---------|---------|----------------------------------------------------------|\n" +
		"| wrap | on | every line is wrapped to the viewport width instead of being cut off |\n" +
		"| pan | off | horizontal scrolling is disabled entirely |"

	for _, width := range renderWidths {
		out := RenderAssistantTable(theme, block, width)
		if got, line := widestLine(out); got > width {
			t.Errorf("table at width %d: widest row is %d cells: %q", width, got, line)
		}
		// Nothing may be dropped on the way: every cell survives, wrapped.
		if !strings.Contains(stripANSI(out), "wrap") {
			t.Errorf("table at width %d lost content: %q", width, stripANSI(out))
		}
	}
}

// The read_file row is the one tool line drawn without a card around it.
func TestReadFileLineFitsEveryWidth(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	args := map[string]interface{}{
		"filePath": "/Volumes/KINGSTON/code/projects/gurtcli/internal/rendering/pipeline/transcript_cache.go",
	}
	for _, width := range renderWidths {
		out := renderReadFileLine(theme, args, "no such file or directory", true, width)
		if got, line := widestLine(out); got > width {
			t.Errorf("read_file line at width %d: widest row is %d cells: %q", width, got, line)
		}
	}
}
