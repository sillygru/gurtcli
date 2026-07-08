package main

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

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
// content coordinates. Returns ok=false if the position is outside content.
func computeContentPosition(m model, mouse tea.Mouse) (line, col int, ok bool) {
	contentLine := m.chatViewport.YOffset() + mouse.Y - computeViewportStartRow(m)
	contentCol := mouse.X - 1
	if contentCol < 0 {
		contentCol = 0
	}
	if contentLine < 0 {
		return 0, 0, false
	}
	return contentLine, contentCol, true
}

// applySelectionHighlight inserts ANSI reverse video markers into the
// viewport content string to highlight the selected character range.
func applySelectionHighlight(content string, sel textSelection) string {
	if !sel.exists && !sel.active {
		return content
	}

	lines := strings.Split(content, "\n")

	// Normalize so anchor <= focus
	startY, endY := sel.anchorY, sel.focusY
	startX, endX := sel.anchorX, sel.focusX
	if startY > endY || (startY == endY && startX > endX) {
		startY, endY = endY, startY
		startX, endX = endX, startX
	}

	if startY < 0 {
		startY = 0
		startX = 0
	}
	if endY >= len(lines) {
		endY = len(lines) - 1
		endX = len(stripANSI(lines[endY]))
	}

	revStart := "\x1b[7m"
	revEnd := "\x1b[27m"

	for i := startY; i <= endY && i < len(lines); i++ {
		plain := stripANSI(lines[i])
		plainLen := utf8.RuneCountInString(plain)

		var selStart, selEnd int
		if i == startY && i == endY {
			selStart = startX
			selEnd = endX
		} else if i == startY {
			selStart = startX
			selEnd = plainLen
		} else if i == endY {
			selStart = 0
			selEnd = endX
		} else {
			selStart = 0
			selEnd = plainLen
		}

		if selStart < 0 {
			selStart = 0
		}
		if selEnd > plainLen {
			selEnd = plainLen
		}
		if selStart >= selEnd || selStart >= plainLen {
			continue
		}

		lines[i] = insertANSIAroundRange(lines[i], selStart, selEnd, revStart, revEnd)
	}

	return strings.Join(lines, "\n")
}

// insertANSIAroundRange injects `before` and `after` SGR sequences around
// the plain-text rune range [start, end) within an ANSI-escaped string.
func insertANSIAroundRange(s string, start, end int, before, after string) string {
	var out bytes.Buffer
	plainPos := 0
	i := 0
	inHighlight := false

	for i < len(s) {
		if s[i] == '\x1b' {
			seqStart := i
			i = skipANSIEscape(s, i)
			out.WriteString(s[seqStart:i])
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		selected := plainPos >= start && plainPos < end

		if selected && !inHighlight {
			out.WriteString(before)
			inHighlight = true
		}
		if !selected && inHighlight {
			out.WriteString(after)
			inHighlight = false
		}
		out.WriteString(s[i : i+size])
		plainPos++
		i += size
	}
	if inHighlight {
		out.WriteString(after)
	}

	return out.String()
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

// extractSelectedText extracts plain text from the selection range.
func extractSelectedText(content string, sel textSelection) string {
	if !sel.exists {
		return ""
	}

	lines := strings.Split(content, "\n")

	startY, endY := sel.anchorY, sel.focusY
	startX, endX := sel.anchorX, sel.focusX
	if startY > endY || (startY == endY && startX > endX) {
		startY, endY = endY, startY
		startX, endX = endX, startX
	}

	if startY < 0 {
		startY = 0
		startX = 0
	}
	if endY >= len(lines) {
		endY = len(lines) - 1
	}

	var result []string
	for i := startY; i <= endY && i < len(lines); i++ {
		plain := stripANSI(lines[i])
		plainLen := utf8.RuneCountInString(plain)

		var selStart, selEnd int
		if i == startY && i == endY {
			selStart = startX
			selEnd = endX
		} else if i == startY {
			selStart = startX
			selEnd = plainLen
		} else if i == endY {
			selStart = 0
			selEnd = endX
		} else {
			selStart = 0
			selEnd = plainLen
		}
		if selStart < 0 {
			selStart = 0
		}
		if selEnd > plainLen {
			selEnd = plainLen
		}
		if selStart < selEnd && selStart < plainLen {
			runes := []rune(plain)
			result = append(result, string(runes[selStart:selEnd]))
		}
	}
	return strings.Join(result, "\n")
}

// copyToClipboard copies text to the system clipboard using platform-specific
// commands. Silently fails if the clipboard tool is unavailable.
func copyToClipboard(text string) {
	if text == "" {
		return
	}
	var (
		cmd    *exec.Cmd
		bin    string
		params []string
	)
	switch runtime.GOOS {
	case "darwin":
		bin = "pbcopy"
	case "linux":
		bin = "xclip"
		params = []string{"-selection", "clipboard"}
	case "windows":
		bin = "clip"
	default:
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, bin, params...)
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run()
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
