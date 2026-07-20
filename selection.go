package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"github.com/sillygru/gurtcli/ui"
)

// Selection geometry
//
// Everything below counts terminal cells. The transcript is styled text, so a
// line is a mix of ANSI escape sequences (zero cells), narrow glyphs (one cell)
// and wide glyphs — CJK, emoji — that cover two. Highlighting and extraction
// both go through the same cell walker and the same per-line span, so what is
// drawn in reverse video is exactly what lands on the clipboard.

// computeViewportStartRow returns the terminal row where the viewport
// content begins, accounting for header and banner wrapping.
func computeViewportStartRow(m model) int {
	brandWidth := lipgloss.Width(m.theme.Brand.Render("  " + m.modelDisplayName()))
	brandRows := 1
	if m.width > 0 && brandWidth > m.width {
		brandRows = (brandWidth + m.width - 1) / m.width
	}
	row := brandRows
	return row + 1 // divider
}

// computeContentPosition converts a terminal mouse position to viewport
// content coordinates. Returns ok=false if the position is outside the
// transcript viewport.
//
// Both axes are zero-based (bubbletea normalizes SGR's 1-based coordinates), so
// the column is the mouse column plus whatever the viewport is scrolled to
// horizontally — no fudge factor. The row is clamped to the visible window so
// that dragging down into the input area selects to the last visible line
// rather than running off into the rest of the transcript.
func computeContentPosition(m model, mouse tea.Mouse) (line, col int, ok bool) {
	startRow := computeViewportStartRow(m)
	height := m.chatViewport.Height()
	if height < 1 {
		height = 1
	}

	row := mouse.Y - startRow
	if row < 0 {
		row = 0
	}
	if row > height-1 {
		row = height - 1
	}

	col = mouse.X + m.chatViewport.XOffset()
	if col < 0 {
		col = 0
	}
	return m.chatViewport.YOffset() + row, col, true
}

// insideViewport reports whether a terminal row falls inside the transcript
// viewport. Rows outside it belong to the chrome and are handled as
// click-to-copy zones instead of text selection.
func insideViewport(m model, y int) bool {
	start := computeViewportStartRow(m)
	return y >= start && y < start+m.chatViewport.Height()
}

// normalizedSelection returns the selection as an ordered (startY, startX) to
// (endY, endX) range where the end cell is exclusive. The anchor and the focus
// cells are both part of the selection, matching terminal drag behaviour.
func (sel textSelection) normalized() (startY, startX, endY, endX int) {
	startY, startX = sel.anchorY, sel.anchorX
	endY, endX = sel.focusY, sel.focusX
	if startY > endY || (startY == endY && startX > endX) {
		startY, startX, endY, endX = endY, endX, startY, startX
	}
	if startY < 0 {
		startY, startX = 0, 0
	}
	if startX < 0 {
		startX = 0
	}
	return startY, startX, endY, endX + 1
}

// lineSpan returns the cell range [start, end) selected on content line i, and
// whether the line takes part in the selection at all. A line inside the range
// but with nothing on it still participates: it contributes the blank line that
// separates the paragraphs the user dragged across.
func (sel textSelection) lineSpan(i, lineCells int) (start, end int, ok bool) {
	startY, startX, endY, endX := sel.normalized()
	if i < startY || i > endY {
		return 0, 0, false
	}

	start, end = 0, lineCells
	if i == startY {
		start = startX
	}
	if i == endY {
		end = endX
	}
	if end > lineCells {
		end = lineCells
	}
	if start > lineCells {
		start = lineCells
	}
	if start > end {
		start = end
	}
	return start, end, true
}

// walkCells iterates the printable grapheme clusters of an ANSI string,
// reporting each cluster's byte range, starting column and width in cells.
// Escape sequences are skipped without advancing the column. Iteration stops
// early when fn returns false.
func walkCells(s string, fn func(byteStart, byteEnd, col, width int) bool) {
	col, i := 0, 0
	for i < len(s) {
		if s[i] == '\x1b' {
			i = skipANSIEscape(s, i)
			continue
		}
		cluster, w := ansi.FirstGraphemeCluster(s[i:], ansi.GraphemeWidth)
		n := len(cluster)
		if n == 0 {
			n = 1
		}
		if !fn(i, i+n, col, w) {
			return
		}
		col += w
		i += n
	}
}

// cellsIntersect reports whether a cluster starting at col and covering width
// cells overlaps the half-open range [start, end). Zero-width clusters
// (combining marks) ride along with the cell they attach to.
func cellsIntersect(col, width, start, end int) bool {
	if width < 1 {
		width = 1
	}
	return col < end && col+width > start
}

// highlightCellRange wraps the cells in [start, end) of an ANSI string with the
// given SGR sequences, leaving every other byte — escape sequences included —
// untouched.
func highlightCellRange(s string, start, end int, before, after string) string {
	if start >= end {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(before) + len(after))
	last, inRange := 0, false

	walkCells(s, func(bs, be, col, w int) bool {
		selected := cellsIntersect(col, w, start, end)
		if selected != inRange {
			b.WriteString(s[last:bs])
			if selected {
				b.WriteString(before)
			} else {
				b.WriteString(after)
			}
			last, inRange = bs, selected
		}
		return true
	})

	b.WriteString(s[last:])
	if inRange {
		b.WriteString(after)
	}
	return b.String()
}

// cellSubstring returns the plain text covering cells [start, end) of an ANSI
// string, dropping the escape sequences.
func cellSubstring(s string, start, end int) string {
	if start >= end {
		return ""
	}
	var b strings.Builder
	walkCells(s, func(bs, be, col, w int) bool {
		if col >= end {
			return false
		}
		if cellsIntersect(col, w, start, end) {
			b.WriteString(s[bs:be])
		}
		return true
	})
	return b.String()
}

// applySelectionHighlight inserts ANSI reverse video markers into the
// viewport content string to highlight the selected cell range.
func applySelectionHighlight(content string, sel textSelection) string {
	if !sel.exists && !sel.active {
		return content
	}

	const (
		revStart = "\x1b[7m"
		revEnd   = "\x1b[27m"
	)

	lines := strings.Split(content, "\n")
	for i := range lines {
		start, end, ok := sel.lineSpan(i, ansi.StringWidth(lines[i]))
		if !ok || start >= end {
			continue
		}
		lines[i] = highlightCellRange(lines[i], start, end, revStart, revEnd)
	}
	return strings.Join(lines, "\n")
}

// extractSelectedText extracts plain text from the selection range. It walks
// the same lines and the same spans as applySelectionHighlight, so the copied
// text always matches the highlight, then tidies the result: trailing padding
// is dropped from every line (the renderer pads lines out to the viewport
// width) and blank lines at the two ends of the block go with it.
func extractSelectedText(content string, sel textSelection) string {
	if !sel.exists && !sel.active {
		return ""
	}

	lines := strings.Split(content, "\n")
	startY, _, endY, _ := sel.normalized()
	if endY >= len(lines) {
		endY = len(lines) - 1
	}
	if startY > endY {
		return ""
	}

	out := make([]string, 0, endY-startY+1)
	for i := startY; i <= endY; i++ {
		start, end, ok := sel.lineSpan(i, ansi.StringWidth(lines[i]))
		if !ok {
			continue
		}
		out = append(out, strings.TrimRight(cellSubstring(lines[i], start, end), " \t"))
	}
	return trimBlankEdges(out)
}

// trimBlankEdges joins lines, dropping empty ones at either end of the block.
func trimBlankEdges(lines []string) string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

// selectWordAt returns the selection covering the word under a cell, or ok=false
// when that cell is not on a word. Words are runs of letters, digits and the
// characters that hold identifiers and paths together, so a double click grabs
// `foo_bar`, `main.go` or `--flag` in one go.
func selectWordAt(content string, line, col int) (textSelection, bool) {
	lines := strings.Split(content, "\n")
	if line < 0 || line >= len(lines) {
		return textSelection{}, false
	}

	type cell struct {
		r          rune
		start, end int
	}
	var cells []cell
	walkCells(lines[line], func(bs, be, c, w int) bool {
		r := []rune(lines[line][bs:be])[0]
		width := w
		if width < 1 {
			width = 1
		}
		cells = append(cells, cell{r: r, start: c, end: c + width})
		return true
	})

	idx := -1
	for i, c := range cells {
		if col >= c.start && col < c.end {
			idx = i
			break
		}
	}
	if idx < 0 || !isWordRune(cells[idx].r) {
		return textSelection{}, false
	}

	first, last := idx, idx
	for first > 0 && isWordRune(cells[first-1].r) {
		first--
	}
	for last < len(cells)-1 && isWordRune(cells[last+1].r) {
		last++
	}

	return textSelection{
		anchorY: line, anchorX: cells[first].start,
		focusY: line, focusX: cells[last].end - 1,
		exists: true,
	}, true
}

// isWordRune reports whether a rune is part of a double-click word.
func isWordRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	return strings.ContainsRune("_-./\\@:+~", r)
}

// selectLineAt returns the selection covering a whole content line, or ok=false
// when the line is blank.
func selectLineAt(content string, line int) (textSelection, bool) {
	lines := strings.Split(content, "\n")
	if line < 0 || line >= len(lines) {
		return textSelection{}, false
	}
	width := ansi.StringWidth(lines[line])
	if strings.TrimSpace(stripANSI(lines[line])) == "" {
		return textSelection{}, false
	}
	return textSelection{
		anchorY: line, anchorX: 0,
		focusY: line, focusX: width - 1,
		exists: true,
	}, true
}

// selectBlockAt returns the selection covering the contiguous run of lines
// sharing the styling of the line under the cursor — a fenced code block, a
// diff, a table — or ok=false when the line is not part of such a block.
// Clicking a code block and getting the whole snippet is the copy people
// actually want; dragging out a twenty-line span by hand is the fallback.
func selectBlockAt(content string, line int, stylePrefix string) (textSelection, bool) {
	if stylePrefix == "" {
		return textSelection{}, false
	}
	lines := strings.Split(content, "\n")
	if line < 0 || line >= len(lines) || !strings.HasPrefix(lines[line], stylePrefix) {
		return textSelection{}, false
	}

	first, last := line, line
	for first > 0 && strings.HasPrefix(lines[first-1], stylePrefix) {
		first--
	}
	for last < len(lines)-1 && strings.HasPrefix(lines[last+1], stylePrefix) {
		last++
	}

	return textSelection{
		anchorY: first, anchorX: 0,
		focusY: last, focusX: ansi.StringWidth(lines[last]) - 1,
		exists: true,
	}, true
}

// codeBlockStylePrefix returns the escape sequence every rendered code block
// line starts with, which is what marks a run of lines as one block. An empty
// string means the theme draws code blocks unstyled and blocks cannot be
// detected, in which case callers fall back to single-line selection.
func codeBlockStylePrefix(t ui.Theme) string {
	const probe = "\x00"
	rendered := t.CodeBlock.Render(probe)
	if i := strings.Index(rendered, probe); i > 0 {
		return rendered[:i]
	}
	return ""
}

// skipANSIEscape advances i past one complete ANSI escape sequence
// starting at position i (s[i] must be \x1b). Returns the index after
// the sequence.
func skipANSIEscape(s string, i int) int {
	i++ // skip \x1b
	if i >= len(s) {
		return i
	}

	switch {
	case s[i] == '[': // CSI: \x1b[... + final byte (0x40–0x7E)
		i++
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3F {
			i++
		}
		if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7E {
			i++
		}
	case s[i] == ']': // OSC: \x1b]...\x07 or \x1b]...\x1b\\
		i++
		for i < len(s) && s[i] != '\x07' {
			if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
				i += 2
				return i
			}
			i++
		}
		if i < len(s) { // skip BEL
			i++
		}
	default: // Simple 2-byte: \x1b + final byte
		if s[i] >= 0x40 && s[i] <= 0x7E {
			i++
		}
	}
	return i
}

// stripANSI removes all ANSI escape sequences from a string.
func stripANSI(s string) string {
	var out bytes.Buffer
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			i = skipANSIEscape(s, i)
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// copyToClipboard copies text to the system clipboard, reporting whether it
// got there. The cross-platform helper is tried first; when it is missing a
// backend (a bare Linux box without xclip, say) the known CLI tools are tried
// in turn so that one missing binary is not the end of the feature.
func copyToClipboard(text string) bool {
	if text == "" {
		return false
	}
	if err := clipboard.WriteAll(text); err == nil {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, c := range clipboardCommands() {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.CommandContext(ctx, c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

// clipboardCommands lists the clipboard writers to try for this platform, in
// preference order.
func clipboardCommands() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbcopy"}}
	case "windows":
		return [][]string{{"clip"}}
	default:
		return [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
			{"clip.exe"}, // WSL
		}
	}
}

// copyCmd puts text on the clipboard and returns the command, if any, that
// finishes the job. Over ssh the local helpers would fill the clipboard of the
// machine gurt runs on, which is not where the user is looking, so OSC 52 is
// used instead — and it is also the last resort when no clipboard tool exists.
func copyCmd(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	if !isRemoteSession() && copyToClipboard(text) {
		return nil
	}
	return tea.SetClipboard(text)
}

// isRemoteSession reports whether gurt is running over ssh.
func isRemoteSession() bool {
	return os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != ""
}

// buildChatContentHighlighted returns the chat content with selection
// highlighting applied when a selection is active.
func buildChatContentHighlighted(m model) string {
	content := buildChatContent(m)
	if m.selection.active || m.selection.exists {
		content = applySelectionHighlight(content, m.selection)
	}
	return content
}
