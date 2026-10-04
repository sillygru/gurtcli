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
	out := RenderToolCall(theme, tc, 80, "")
	if !strings.Contains(out, "Read") {
		t.Fatalf("expected Read label, got: %q", out)
	}
	if !strings.Contains(out, "main.go") {
		t.Fatalf("expected path in output, got: %q", out)
	}
	if !strings.Contains(out, "(lines 10-59)") {
		t.Fatalf("expected line range in output, got: %q", out)
	}
}

func TestRenderReadFileRangeSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args string
		want string
	}{
		{"offset and limit", `{"filePath":"a.go","offset":10,"limit":50}`, "(lines 10-59)"},
		{"offset only", `{"filePath":"a.go","offset":10}`, "(from line 10)"},
		{"limit only", `{"filePath":"a.go","limit":50}`, "(first 50 lines)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := RenderToolCall(DefaultTheme(), llm.ToolCall{
				Function: llm.ToolCallFunction{Name: "read_file", Arguments: tt.args},
			}, 120, "")
			if !strings.Contains(out, tt.want) {
				t.Fatalf("expected %q in output, got: %q", tt.want, out)
			}
		})
	}

	full := RenderToolCall(DefaultTheme(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "read_file", Arguments: `{"filePath":"a.go"}`},
	}, 80, "")
	if strings.Contains(full, "lines") {
		t.Fatalf("expected no range without offset/limit, got: %q", full)
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
	out := RenderToolCall(theme, tc, 80, "")
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

func TestRenderToolCallRunBashBusyGlyph(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	tc := llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "run_bash",
			Arguments: `{"command":"run-tests --watch","title":"Watch tests"}`,
		},
	}

	// Quiet render uses the static shell icon.
	quiet := RenderToolCall(theme, tc, 80, "")
	if !strings.Contains(quiet, "$") {
		t.Fatalf("expected static $ icon, got: %q", quiet)
	}

	// Busy render swaps the header icon for the spinner frame.
	busy := RenderToolCall(theme, tc, 80, "◓")
	if strings.Contains(busy, "$") {
		t.Fatalf("busy render must not keep the static $ icon, got: %q", busy)
	}
	if !strings.Contains(busy, "◓") {
		t.Fatalf("expected the spinner frame in the busy card, got: %q", busy)
	}
	if !strings.Contains(busy, "Watch tests") {
		t.Fatalf("expected title retained, got: %q", busy)
	}
}

func TestRenderToolCallEditFile(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	tc := llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "edit_file",
			Arguments: `{"filePath":"foo.go","oldString":"old\nline","newString":"new\nline"}`,
		},
	}
	out := RenderToolCall(theme, tc, 80, "")
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

// The status line is a protocol marker for the model, not content for the
// user. It must be stripped from the rendered preview along with the blank
// line that follows it.
func TestToolResultPreviewStripsStatusHeader(t *testing.T) {
	got := toolResultPreview("status: OK\n\nhello world", "run_bash")
	if got != "hello world" {
		t.Errorf("status header not stripped, got %q", got)
	}

	got = toolResultPreview("status: FAILED\n\ncommand failed: exit 1", "run_bash")
	if got != "command failed: exit 1" {
		t.Errorf("failed status header not stripped, got %q", got)
	}
}

// A result that is nothing but the status line must render as empty rather than
// as a lone marker.
func TestToolResultPreviewStripsHeaderOnlyResult(t *testing.T) {
	if got := toolResultPreview("status: OK\n\n", "run_bash"); got != "" {
		t.Errorf("expected empty preview for a bodyless result, got %q", got)
	}
}

// Content that merely starts with the word "status" but is not the header must
// survive untouched.
func TestToolResultPreviewKeepsNonHeaderStatusLines(t *testing.T) {
	if got := toolResultPreview("status of the build: ok", "run_bash"); got != "status of the build: ok" {
		t.Errorf("non-header line was stripped, got %q", got)
	}
}

// Truncation and trailing-line caps must still apply after the header is
// stripped, and the "more lines" notice must remain the last thing shown.
func TestToolResultPreviewStillTruncatesAfterStripping(t *testing.T) {
	body := make([]string, 0, maxBashResultLines+5)
	body = append(body, "status: OK", "")
	for i := 0; i < maxBashResultLines+5; i++ {
		body = append(body, "line")
	}
	got := toolResultPreview(strings.Join(body, "\n"), "run_bash")
	if !strings.Contains(got, "more lines") {
		t.Errorf("expected the more-lines notice, got %q", got)
	}
	if strings.Contains(got, ToolResultStatusHeader) {
		t.Errorf("status header leaked into a truncated preview: %q", got)
	}
}
