package ui

import (
	"strings"
	"testing"
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
