package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// GitHub-inspired diff colors on dark background
const (
	ColorDiffDelBG        = "#3d2028"
	ColorDiffDelHighlight = "#6e3040"
	ColorDiffDelText      = "#cdd6f4"
	ColorDiffDelChange    = "#f38ba8"
	ColorDiffAddBG        = "#1a2e28"
	ColorDiffAddHighlight = "#2a4538"
	ColorDiffAddText      = "#cdd6f4"
	ColorDiffAddChange    = "#a6e3a1"
)

// WrapScreen is a no-op passthrough; kept for API stability.
func WrapScreen(content string, _, _ int) string {
	return content
}

// RenderReasoning renders the reasoning toggle and optional expanded content.
func RenderReasoning(t Theme, active, visible bool, elapsed time.Duration, content string, width int) string {
	header := renderReasoningHeader(t, active, visible, elapsed)

	if !visible || content == "" {
		return header
	}

	boxW := cardWidth(width) - 2
	if boxW < 28 {
		boxW = 28
	}

	var body strings.Builder
	for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		body.WriteString("  ")
		body.WriteString(t.ReasoningText.Render(line))
		body.WriteString("\n")
	}

	inner := header + "\n" + strings.TrimRight(body.String(), "\n")
	return t.ReasoningBox.Width(boxW).Render(inner)
}

// RenderReasoningStored renders a collapsed reasoning indicator for finalized messages.
func RenderReasoningStored(t Theme) string {
	icon := t.ReasoningHeader.Render("◷")
	label := t.ReasoningToggle.Render(" reasoning available")
	return "  " + icon + label
}

func renderReasoningHeader(t Theme, active, visible bool, elapsed time.Duration) string {
	icon := "◷"
	status := "thought for"
	chevron := "▸"

	if active {
		icon = "◌"
		status = "thinking"
		chevron = "▾"
	} else if visible {
		chevron = "▾"
	}

	elapsedStr := formatDuration(elapsed)
	iconStyled := t.ReasoningHeader.Render(icon)
	statusStyled := t.ReasoningToggle.Render(fmt.Sprintf(" %s %s", status, elapsedStr))
	chevronStyled := t.Muted.Render("  " + chevron)

	return "  " + iconStyled + statusStyled + chevronStyled
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	secs := d.Seconds()
	if secs < 10 {
		return fmt.Sprintf("%.1fs", secs)
	}
	return fmt.Sprintf("%.0fs", secs)
}

// ScreenStyle is unused; retained so callers compile without background fill.
func ScreenStyle() lipgloss.Style {
	return lipgloss.NewStyle()
}

// renderDiffRemovedLine renders a removed line with red fill and change highlights.
func renderDiffRemovedLine(t Theme, prefix, line, counterpart string, maxLen int) string {
	line = truncateLine(line, maxLen)
	body := highlightLineDiff(line, counterpart, t.DiffDel, t.DiffDelHighlight)
	return t.DiffDel.Render(prefix) + body
}

// renderDiffAddedLine renders an added line with green fill and change highlights.
func renderDiffAddedLine(t Theme, prefix, line, counterpart string, maxLen int) string {
	line = truncateLine(line, maxLen)
	body := highlightLineDiff(line, counterpart, t.DiffAdd, t.DiffAddHighlight)
	return t.DiffAdd.Render(prefix) + body
}

// highlightLineDiff applies GitHub-style intra-line change highlighting.
func highlightLineDiff(line, counterpart string, base, highlight lipgloss.Style) string {
	if line == "" {
		return base.Render("")
	}
	if counterpart == "" {
		return highlight.Render(line)
	}

	prefixLen := commonPrefixLen(line, counterpart)
	maxSuffix := min(len(line)-prefixLen, len(counterpart)-prefixLen)
	suffixLen := commonSuffixLen(line[prefixLen:], counterpart[prefixLen:], maxSuffix)

	midEnd := len(line) - suffixLen
	if prefixLen >= midEnd {
		return base.Render(line)
	}

	var b strings.Builder
	if prefixLen > 0 {
		b.WriteString(base.Render(line[:prefixLen]))
	}
	b.WriteString(highlight.Render(line[prefixLen:midEnd]))
	if suffixLen > 0 {
		b.WriteString(base.Render(line[midEnd:]))
	}
	return b.String()
}

func commonPrefixLen(a, b string) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func commonSuffixLen(a, b string, max int) int {
	count := 0
	for count < max && a[len(a)-1-count] == b[len(b)-1-count] {
		count++
	}
	return count
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
