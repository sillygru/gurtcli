package ui

import (
	"strings"
	"testing"
)

// hasGuidePrefix checks that a line starts with the indent prefix.
func hasGuidePrefix(t Theme, line string) bool {
	cleaned := stripANSI(line)
	return strings.HasPrefix(cleaned, "  ")
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			for i++; i < len(s); i++ {
				if s[i] >= 'A' && s[i] <= 'Z' || s[i] >= 'a' && s[i] <= 'z' {
					break
				}
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestTableParseBasic(t *testing.T) {
	t.Parallel()
	input := `| Name | Age | City |
|------|-----|------|
| Alice | 30 | NYC |
| Bob | 25 | London |`
	tbl, ok := parseTable(strings.Split(input, "\n"))
	if !ok {
		t.Fatal("expected table to parse")
	}
	if len(tbl.header) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(tbl.header))
	}
	if tbl.header[0] != "Name" || tbl.header[1] != "Age" || tbl.header[2] != "City" {
		t.Fatalf("unexpected header: %v", tbl.header)
	}
	if len(tbl.rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(tbl.rows))
	}
	if tbl.rows[0][0] != "Alice" || tbl.rows[0][1] != "30" || tbl.rows[0][2] != "NYC" {
		t.Fatalf("unexpected row 0: %v", tbl.rows[0])
	}
	// Default alignment is left
	for i, a := range tbl.align {
		if a != "left" {
			t.Fatalf("expected left alignment for col %d, got %s", i, a)
		}
	}
}

func TestTableParseAlignment(t *testing.T) {
	t.Parallel()
	input := `| Left | Center | Right |
|:-----|:------:|------:|
| a | b | c |`
	tbl, ok := parseTable(strings.Split(input, "\n"))
	if !ok {
		t.Fatal("expected table to parse")
	}
	if tbl.align[0] != "left" {
		t.Fatalf("expected left alignment, got %s", tbl.align[0])
	}
	if tbl.align[1] != "center" {
		t.Fatalf("expected center alignment, got %s", tbl.align[1])
	}
	if tbl.align[2] != "right" {
		t.Fatalf("expected right alignment, got %s", tbl.align[2])
	}
}

func TestTableParseNoSeparator(t *testing.T) {
	t.Parallel()
	input := `| A | B |
| 1 | 2 |
| 3 | 4 |`
	tbl, ok := parseTable(strings.Split(input, "\n"))
	if !ok {
		t.Fatal("expected table to parse without separator")
	}
	if len(tbl.header) != 2 {
		t.Fatalf("expected 2 cols, got %d", len(tbl.header))
	}
	if len(tbl.rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(tbl.rows))
	}
}

func TestTableParseMalformed(t *testing.T) {
	t.Parallel()
	// Only 1 line — can't be a table
	_, ok := parseTable(strings.Split("just a line", "\n"))
	if ok {
		t.Fatal("expected malformed table to not parse")
	}
}

func TestTableParseEmptyHeader(t *testing.T) {
	t.Parallel()
	// Empty header cells — row is ["a", "b"] still parses but header is [""]
	tbl, ok := parseTable(strings.Split("||\n|---|---|\n|a|b|", "\n"))
	if ok {
		t.Logf("parsed: header=%v rows=%v", tbl.header, tbl.rows)
		// This is technically parseable; just check it doesn't panic
		if len(tbl.header) == 0 {
			t.Fatal("expected some header")
		}
	}
}

func TestTableRenderBasic(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	input := `| Name | Age |
|------|-----|
| Alice | 30 |
| Bob | 25 |`
	out := RenderAssistantTable(theme, input, 80)
	lines := strings.Split(out, "\n")
	// Should have: prefix + top border, header, separator, 2 data rows, bottom border = 6 lines
	if len(lines) < 6 {
		t.Fatalf("expected at least 6 lines, got %d:\n%s", len(lines), out)
	}
	// All lines should start with styled guide prefix
	for i, line := range lines {
		if !hasGuidePrefix(theme, line) {
			t.Fatalf("line %d missing guide prefix: %q", i, line)
		}
	}
	// Should contain box-drawing characters
	if !strings.Contains(out, "╭") || !strings.Contains(out, "╮") {
		t.Fatalf("missing top border: \n%s", out)
	}
	if !strings.Contains(out, "╰") || !strings.Contains(out, "╯") {
		t.Fatalf("missing bottom border: \n%s", out)
	}
	if !strings.Contains(out, "├") || !strings.Contains(out, "┤") {
		t.Fatalf("missing separator border: \n%s", out)
	}
	if !strings.Contains(out, "┼") {
		t.Fatalf("missing crossing: \n%s", out)
	}
	// Header content should be present
	if !strings.Contains(out, "Name") || !strings.Contains(out, "Age") {
		t.Fatalf("missing header content: \n%s", out)
	}
	// Data rows should be present
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") {
		t.Fatalf("missing data: \n%s", out)
	}
}

func TestTableRenderWide(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	input := `| A | B | C |
|---|---|---|
| 1 | 2 | 3 |`
	out := RenderAssistantTable(theme, input, 40)
	lines := strings.Split(out, "\n")
	// Table should fill width-4 = 36
	// Border lines should be ~36 chars (after the "  " prefix)
	if len(lines) < 4 {
		t.Fatalf("expected 4+ lines, got %d", len(lines))
	}
	prefixLen := len("  ")
	for i, line := range lines {
		content := line[prefixLen:]
		// Content should be longer than the minimum (just text)
		// to verify it stretches
		minLen := len("│ 1 │ 2 │ 3 │")
		if len(content) < minLen {
			t.Fatalf("line %d too narrow: %q (%d < %d)", i, content, len(content), minLen)
		}
	}
}

func TestTableRenderSingleRow(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	input := `| X | Y |
|---|---|
| 1 | 2 |`
	out := RenderAssistantTable(theme, input, 60)
	if !strings.Contains(out, "X") || !strings.Contains(out, "Y") {
		t.Fatalf("missing headers: \n%s", out)
	}
	if !strings.Contains(out, "1") || !strings.Contains(out, "2") {
		t.Fatalf("missing data: \n%s", out)
	}
}

func TestTableRenderEmptyCells(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	input := `| A | B | C |
|---|---|---|
| 1 || 3 |`
	out := RenderAssistantTable(theme, input, 60)
	if !strings.Contains(out, "1") || !strings.Contains(out, "3") {
		t.Fatalf("missing data: \n%s", out)
	}
	// Empty cell renders without error (presence of 1 and 3 confirms parsing)
	if strings.Contains(out, "panic") {
		t.Fatal("table render panicked")
	}
}

func TestIsTableCandidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		lines []string
		i     int
		want  bool
	}{
		{[]string{"| a | b |", "| c | d |"}, 0, true},
		{[]string{"hello", "world"}, 0, false},
		{[]string{"| a | b |"}, 0, false},         // no next line
		{[]string{"no pipe", "| c | d |"}, 0, false}, // current line no pipe
		{[]string{"a | b", "c | d"}, 0, true},     // pipes present
	}
	for _, tt := range tests {
		got := isTableCandidate(tt.lines, tt.i)
		if got != tt.want {
			t.Errorf("isTableCandidate(%v, %d) = %v, want %v", tt.lines, tt.i, got, tt.want)
		}
	}
}

func TestTableContentAvailable(t *testing.T) {
	t.Parallel()
	if !TableContentAvailable("foo | bar") {
		t.Fatal("expected pipe to be detected")
	}
	if TableContentAvailable("no pipes here") {
		t.Fatal("expected no pipes")
	}
}

func TestPadCell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		text  string
		w     int
		align string
		want  string
	}{
		{"hi", 5, "left", "hi   "},
		{"hi", 5, "right", "   hi"},
		{"hi", 5, "center", " hi  "},
		{"hello", 3, "left", "hello"}, // truncation not expected; n >= width
	}
	for _, tt := range tests {
		got := padCell(tt.text, tt.w, tt.align)
		if got != tt.want {
			t.Errorf("padCell(%q, %d, %q) = %q, want %q", tt.text, tt.w, tt.align, got, tt.want)
		}
	}
}

func TestTableInAssistantContent(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	content := `Here is a table:

| Name | Value |
|------|-------|
| Foo  | 42    |

And some text after.`
	out := RenderAssistantContent(theme, content, 80, nil)
	// Guide prefix should exist on all lines
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if !hasGuidePrefix(theme, line) {
			t.Fatalf("line %d missing guide: %q", i, line)
		}
	}
	// Table border characters should appear
	if !strings.Contains(out, "╭") || !strings.Contains(out, "╮") {
		t.Fatalf("table borders missing: \n%s", out)
	}
	// Regular text should still render
	if !strings.Contains(out, "Here is a table") || !strings.Contains(out, "And some text after") {
		t.Fatalf("non-table text missing: \n%s", out)
	}
}

func TestTableWithoutSeparatorInAssistantContent(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	// Two lines with pipes but no separator — still detected as table
	content := `| A | B |
| 1 | 2 |`
	out := RenderAssistantContent(theme, content, 60, nil)
	if !strings.Contains(out, "╭") {
		t.Fatalf("expected table borders, got: \n%s", out)
	}
}

func TestSinglePipeLineNotATable(t *testing.T) {
	t.Parallel()
	theme := DefaultTheme()
	// A single pipe line should not trigger table rendering
	content := `this is | not a table`
	out := RenderAssistantContent(theme, content, 60, nil)
	if strings.Contains(out, "╭") {
		t.Fatal("single pipe line should not render as table")
	}
	if !strings.Contains(out, "not a table") {
		t.Fatal("content should still be present")
	}
}

func TestBuildTopBorder(t *testing.T) {
	t.Parallel()
	got := buildTopBorder([]int{4, 3, 5})
	if got != "╭──────┬─────┬───────╮" {
		t.Fatalf("unexpected top border: %q", got)
	}
}

func TestBuildSepBorder(t *testing.T) {
	t.Parallel()
	got := buildSepBorder([]int{4, 3, 5})
	if got != "├──────┼─────┼───────┤" {
		t.Fatalf("unexpected sep border: %q", got)
	}
}

func TestBuildBottomBorder(t *testing.T) {
	t.Parallel()
	got := buildBottomBorder([]int{4, 3, 5})
	if got != "╰──────┴─────┴───────╯" {
		t.Fatalf("unexpected bottom border: %q", got)
	}
}
