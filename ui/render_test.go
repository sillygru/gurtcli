package ui

import (
	"strings"
	"testing"

	"github.com/sillygru/gurtcli/llm"
)

func TestRenderToolCallReadFile(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	tc := llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "read_file",
			Arguments: `{"filePath":"src/main.go","offset":10,"limit":50}`,
		},
	}
	out := RenderToolCall(theme, tc, 80)
	if !strings.Contains(out, "Read") {
		t.Fatalf("expected Read label, got: %q", out)
	}
	if !strings.Contains(out, "main.go") {
		t.Fatalf("expected path in output, got: %q", out)
	}
}

func TestRenderToolCallRunBash(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	tc := llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "run_bash",
			Arguments: `{"command":"go test ./...","title":"Run tests"}`,
		},
	}
	out := RenderToolCall(theme, tc, 80)
	if !strings.Contains(out, "Shell") {
		t.Fatalf("expected Shell label, got: %q", out)
	}
	if !strings.Contains(out, "Run tests") {
		t.Fatalf("expected title, got: %q", out)
	}
	if !strings.Contains(out, "go test") {
		t.Fatalf("expected command, got: %q", out)
	}
}

func TestRenderToolCallEditFile(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	tc := llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name: "edit_file",
			Arguments: `{"filePath":"foo.go","oldString":"old\nline","newString":"new\nline"}`,
		},
	}
	out := RenderToolCall(theme, tc, 80)
	if !strings.Contains(out, "Edit") {
		t.Fatalf("expected Edit label, got: %q", out)
	}
	if !strings.Contains(out, "old") || !strings.Contains(out, "new") {
		t.Fatalf("expected diff content, got: %q", out)
	}
	if !strings.Contains(out, "foo.go") {
		t.Fatalf("expected path in output, got: %q", out)
	}
}

func TestRenderToolResultSuccess(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	out := RenderToolResult(theme, "write_file", "Successfully wrote 42 bytes to main.go", 80, false)
	if !strings.Contains(out, "Write") {
		t.Fatalf("expected tool label, got: %q", out)
	}
	if !strings.Contains(out, "Successfully wrote") {
		t.Fatalf("expected result body, got: %q", out)
	}
	if !strings.Contains(out, "╭") {
		t.Fatalf("expected card border, got: %q", out)
	}
}

func TestRenderToolResultError(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()

	// run_bash with "Error:" prefix (actual tool execution error)
	out := RenderToolResult(theme, "run_bash", "Error: exit status 1", 80, true)
	if !strings.Contains(out, "Shell") {
		t.Fatalf("expected tool label for Shell, got: %q", out)
	}
	if !strings.Contains(out, "Error: exit status 1") {
		t.Fatalf("expected error body for run_bash, got: %q", out)
	}
	if !strings.Contains(out, "╭") {
		t.Fatalf("expected card border, got: %q", out)
	}

	// run_bash with regular content (no error)
	out2 := RenderToolResult(theme, "run_bash", "command finished successfully", 80, false)
	if !strings.Contains(out2, "command finished successfully") {
		t.Fatalf("expected bash output body, got: %q", out2)
	}
}

func TestRenderUserMessageCard(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	out := RenderUserMessage(theme, "hello world", 80, nil)
	if !strings.Contains(out, "You") {
		t.Fatalf("expected You label, got: %q", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Fatalf("expected message content, got: %q", out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected styled output, got: %q", out)
	}
}

func TestShortenPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"main.go", "main.go"},
		{"src/main.go", "src/main.go"},
		{"a/b/c/d/e/file.go", "…/e/file.go"},
	}
	for _, tt := range tests {
		got := shortenPath(tt.in)
		if got != tt.want {
			t.Errorf("shortenPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestToolAccentForUnknown(t *testing.T) {
	t.Parallel()
	a := DefaultTheme().ToolAccentFor("unknown_tool")
	if a.Icon == "" || a.Label != "unknown_tool" {
		t.Fatalf("unexpected accent: %+v", a)
	}
}
