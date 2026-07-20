package main

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/ui"
)

// highlightedRun returns the plain text between the reverse-video markers of a
// highlighted line — that is, exactly the cells the user sees selected.
func highlightedRun(t *testing.T, line string) string {
	t.Helper()
	var runs []string
	rest := line
	for {
		start := strings.Index(rest, "\x1b[7m")
		if start < 0 {
			break
		}
		rest = rest[start+len("\x1b[7m"):]
		end := strings.Index(rest, "\x1b[27m")
		if end < 0 {
			t.Fatalf("unterminated highlight in %q", line)
		}
		runs = append(runs, stripANSI(rest[:end]))
		rest = rest[end+len("\x1b[27m"):]
	}
	return strings.Join(runs, "")
}

func TestSelectionCountsCellsNotRunes(t *testing.T) {
	// 日 and 本 are two cells each, so cells 0..3 are the first two glyphs and
	// the ASCII text starts at cell 6.
	line := "日本語 text"
	sel := textSelection{anchorX: 0, focusX: 3, exists: true}

	start, end, ok := sel.lineSpan(0, ansi.StringWidth(line))
	if !ok {
		t.Fatal("line not selected")
	}
	if got := cellSubstring(line, start, end); got != "日本" {
		t.Errorf("cellSubstring = %q, want %q", got, "日本")
	}

	sel = textSelection{anchorX: 7, focusX: 10, exists: true}
	if got := extractSelectedText(line, sel); got != "text" {
		t.Errorf("extractSelectedText = %q, want %q", got, "text")
	}
}

func TestHighlightMatchesExtraction(t *testing.T) {
	theme := ui.DefaultTheme()
	content := strings.Join([]string{
		theme.Header.Render("  Heading with 日本語 in it"),
		theme.Dim.Render("  plain line"),
		"",
		theme.CodeBlock.Render("  func main() {}"),
	}, "\n")

	// Every span of every line must highlight the same cells it copies.
	lines := strings.Split(content, "\n")
	for y := range lines {
		width := ansi.StringWidth(lines[y])
		for start := 0; start < width; start += 3 {
			for end := start; end < width; end += 5 {
				sel := textSelection{
					anchorY: y, anchorX: start,
					focusY: y, focusX: end,
					exists: true,
				}
				highlighted := strings.Split(applySelectionHighlight(content, sel), "\n")[y]
				want := strings.TrimRight(extractSelectedText(content, sel), " ")
				if got := strings.TrimRight(highlightedRun(t, highlighted), " "); got != want {
					t.Errorf("line %d cells [%d,%d]: highlighted %q, copied %q", y, start, end, got, want)
				}
			}
		}
	}
}

func TestHighlightPreservesStyling(t *testing.T) {
	theme := ui.DefaultTheme()
	line := theme.Header.Render("styled text")
	sel := textSelection{anchorX: 2, focusX: 5, exists: true}

	out := strings.Split(applySelectionHighlight(line, sel), "\n")[0]
	if stripANSI(out) != stripANSI(line) {
		t.Errorf("highlight changed the text: %q vs %q", stripANSI(out), stripANSI(line))
	}
	if !strings.Contains(out, "\x1b[7m") || !strings.Contains(out, "\x1b[27m") {
		t.Error("highlight markers missing")
	}
}

func TestSelectionIncludesAnchorAndFocusCells(t *testing.T) {
	// Pressing on 'b' and releasing on 'd' selects "bcd", not "bc".
	sel := textSelection{anchorX: 1, focusX: 3, exists: true}
	if got := extractSelectedText("abcdef", sel); got != "bcd" {
		t.Errorf("extractSelectedText = %q, want %q", got, "bcd")
	}
}

func TestSelectionBackwardsDrag(t *testing.T) {
	sel := textSelection{anchorY: 1, anchorX: 2, focusY: 0, focusX: 1, exists: true}
	if got := extractSelectedText("abcdef\nghijkl", sel); got != "bcdef\nghi" {
		t.Errorf("extractSelectedText = %q, want %q", got, "bcdef\nghi")
	}
}

func TestExtractKeepsInteriorBlankLines(t *testing.T) {
	content := "first para\n\nsecond para"
	sel := textSelection{anchorY: 0, anchorX: 0, focusY: 2, focusX: 20, exists: true}
	if got := extractSelectedText(content, sel); got != content {
		t.Errorf("extractSelectedText = %q, want %q", got, content)
	}
}

func TestExtractTrimsPaddingAndBlankEdges(t *testing.T) {
	content := "   \nhello world     \n   "
	sel := textSelection{anchorY: 0, anchorX: 0, focusY: 2, focusX: 40, exists: true}
	if got := extractSelectedText(content, sel); got != "hello world" {
		t.Errorf("extractSelectedText = %q, want %q", got, "hello world")
	}
}

func TestSelectWordAt(t *testing.T) {
	line := ui.DefaultTheme().Dim.Render("  run go test ./update_test.go now")
	plain := stripANSI(line)

	tests := []struct {
		col  int
		want string
	}{
		{col: 2, want: "run"},
		{col: 7, want: "go"},
		{col: 10, want: "test"},
		{col: 14, want: "./update_test.go"},
		{col: 5, want: ""}, // the space between words
	}
	for _, tc := range tests {
		sel, ok := selectWordAt(line, 0, tc.col)
		if tc.want == "" {
			if ok {
				t.Errorf("col %d: expected no word, got %q", tc.col, extractSelectedText(line, sel))
			}
			continue
		}
		if !ok {
			t.Fatalf("col %d: no word found in %q", tc.col, plain)
		}
		if got := extractSelectedText(line, sel); got != tc.want {
			t.Errorf("col %d: word = %q, want %q", tc.col, got, tc.want)
		}
	}
}

func TestSelectBlockAtGrabsWholeCodeBlock(t *testing.T) {
	theme := ui.DefaultTheme()
	prefix := codeBlockStylePrefix(theme)
	if prefix == "" {
		t.Skip("theme renders code blocks unstyled")
	}

	content := strings.Join([]string{
		theme.Dim.Render("  prose above"),
		theme.CodeBlock.Render("  func main() {"),
		theme.CodeBlock.Render("  \tprintln(\"hi\")"),
		theme.CodeBlock.Render("  }"),
		theme.Dim.Render("  prose below"),
	}, "\n")

	sel, ok := selectBlockAt(content, 2, prefix)
	if !ok {
		t.Fatal("no block found on a code line")
	}
	got := extractSelectedText(content, sel)
	if strings.Count(got, "\n") != 2 || !strings.Contains(got, "func main() {") || strings.Contains(got, "prose") {
		t.Errorf("block = %q", got)
	}

	if _, ok := selectBlockAt(content, 0, prefix); ok {
		t.Error("prose line reported as a block")
	}
}

func TestSelectLineAt(t *testing.T) {
	content := "  hello there   \n\n  bye"
	sel, ok := selectLineAt(content, 0)
	if !ok {
		t.Fatal("no line selected")
	}
	if got := extractSelectedText(content, sel); got != "  hello there" {
		t.Errorf("line = %q", got)
	}
	if _, ok := selectLineAt(content, 1); ok {
		t.Error("blank line reported as selectable")
	}
}

// testChatModel returns a chat model with a known viewport geometry.
func testChatModel() model {
	vp := viewport.New()
	vp.SetWidth(76)
	vp.SetHeight(10)
	return model{
		state:        stateChat,
		width:        80,
		height:       20,
		theme:        ui.DefaultTheme(),
		chatViewport: vp,
		modelName:    "test-model",
	}
}

func TestComputeContentPositionMapsCellsOneToOne(t *testing.T) {
	m := testChatModel()
	start := computeViewportStartRow(m)

	line, col, ok := computeContentPosition(m, tea.Mouse{X: 0, Y: start})
	if !ok || line != 0 || col != 0 {
		t.Errorf("top-left = (%d,%d,%v), want (0,0,true)", line, col, ok)
	}

	line, col, _ = computeContentPosition(m, tea.Mouse{X: 12, Y: start + 3})
	if line != 3 || col != 12 {
		t.Errorf("mid = (%d,%d), want (3,12)", line, col)
	}

	// Scrolled transcripts offset the line, never the column.
	m.chatViewport.SetYOffset(5)
	line, col, _ = computeContentPosition(m, tea.Mouse{X: 4, Y: start + 1})
	if line != m.chatViewport.YOffset()+1 || col != 4 {
		t.Errorf("scrolled = (%d,%d), want (%d,4)", line, col, m.chatViewport.YOffset()+1)
	}

	// Dragging below the viewport clamps to its last visible row.
	line, _, _ = computeContentPosition(m, tea.Mouse{X: 0, Y: m.height - 1})
	if want := m.chatViewport.YOffset() + m.chatViewport.Height() - 1; line != want {
		t.Errorf("below viewport line = %d, want %d", line, want)
	}
}

func TestInsideViewport(t *testing.T) {
	m := testChatModel()
	start := computeViewportStartRow(m)
	for _, tc := range []struct {
		y    int
		want bool
	}{
		{y: 0, want: false},
		{y: start - 1, want: false},
		{y: start, want: true},
		{y: start + m.chatViewport.Height() - 1, want: true},
		{y: start + m.chatViewport.Height(), want: false},
		{y: m.height - 1, want: false},
	} {
		if got := insideViewport(m, tc.y); got != tc.want {
			t.Errorf("insideViewport(%d) = %v, want %v", tc.y, got, tc.want)
		}
	}
}

// TestDragRoundTrip drives the mouse handler the way a terminal does — press,
// motion, release — and checks that the text pulled out of the transcript is
// the text the pointer was actually over.
func TestDragRoundTrip(t *testing.T) {
	m := testChatModel()
	m.messages = []llm.Message{{Role: "user", Content: "the quick brown fox"}}

	content := buildChatContent(m)
	line, col := findInContent(t, content, "quick brown")

	startRow := computeViewportStartRow(m)
	press := tea.MouseClickMsg{X: col, Y: startRow + line, Button: tea.MouseLeft}
	drag := tea.MouseMotionMsg{X: col + len("quick brown") - 1, Y: startRow + line, Button: tea.MouseLeft}

	updated, _ := m.updateMouse(press)
	m = updated.(model)
	updated, _ = m.updateMouse(drag)
	m = updated.(model)

	if !m.selection.active {
		t.Fatal("drag did not start a selection")
	}
	if got := extractSelectedText(buildChatContent(m), m.selection); got != "quick brown" {
		t.Errorf("dragged text = %q, want %q", got, "quick brown")
	}

	// What the viewport shows must carry the same run in reverse video.
	highlighted := strings.Split(applySelectionHighlight(content, m.selection), "\n")[line]
	if got := highlightedRun(t, highlighted); got != "quick brown" {
		t.Errorf("highlighted text = %q, want %q", got, "quick brown")
	}
}

// findInContent returns the content line and starting cell of a substring.
func findInContent(t *testing.T, content, want string) (line, col int) {
	t.Helper()
	for i, l := range strings.Split(content, "\n") {
		plain := stripANSI(l)
		if idx := strings.Index(plain, want); idx >= 0 {
			return i, ansi.StringWidth(plain[:idx])
		}
	}
	t.Fatalf("%q not found in transcript", want)
	return 0, 0
}
