package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// ansiBackground returns the ANSI escape sequence to set the background
// to the given hex color (e.g. "#1e1e2e").
func ansiBackground(hex string) string {
	r, _ := strconv.ParseUint(hex[1:3], 16, 8)
	g, _ := strconv.ParseUint(hex[3:5], 16, 8)
	b, _ := strconv.ParseUint(hex[5:7], 16, 8)
	return fmt.Sprintf("\033[48;2;%d;%d;%dm", r, g, b)
}

// WrapScreen fills the terminal area with the given base background color and
// places content at the top. Remaining vertical space is padded with
// background-colored blank lines; content taller than the screen is trimmed so
// an over-long view can never scroll the terminal.
func WrapScreen(content string, width, height int, baseColor string) string {
	if width <= 0 || height <= 0 {
		return content
	}
	bgSeq := ansiBackground(baseColor)
	reset := "\033[0m"
	reset2 := "\033[m" // short form used by charm v2 libs

	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	var b strings.Builder
	for i, line := range lines {
		rawWidth := lipgloss.Width(line)
		// After every ANSI reset inside the line, re-set the background
		// so styled text always has the base background behind it.
		line = strings.ReplaceAll(line, reset2, reset2+bgSeq)
		line = strings.ReplaceAll(line, reset, reset+bgSeq)

		b.WriteString(bgSeq)
		b.WriteString(line)
		pad := width - rawWidth
		if pad > 0 {
			b.WriteString(bgSeq)
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(reset)
		if i < height-1 {
			b.WriteString("\n")
		}
	}
	emptyLine := bgSeq + strings.Repeat(" ", width) + reset
	for i := len(lines); i < height; i++ {
		b.WriteString(emptyLine)
		if i < height-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// RenderReasoning renders the reasoning toggle and optional expanded content.
func RenderReasoning(t Theme, active, visible bool, elapsed time.Duration, content string, width int) string {
	header := renderReasoningHeader(t, active, visible, elapsed)

	if !visible || content == "" {
		return header
	}

	boxW := width
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
func RenderReasoningStored(t Theme, duration time.Duration) string {
	icon := t.ReasoningHeader.Render("◷")
	label := " reasoning available"
	if duration > 0 {
		label = " thought for " + formatDuration(duration)
	}
	labelStyled := t.ReasoningToggle.Render(label)
	return "  " + icon + labelStyled
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
	totalSecs := int(d.Seconds())
	if totalSecs >= 60 {
		mins := totalSecs / 60
		rem := totalSecs % 60
		if rem == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm %ds", mins, rem)
	}
	if totalSecs < 10 {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%ds", totalSecs)
}

// ScreenStyle returns a style with the given background color for screen-level use.
func ScreenStyle(baseColor string) lipgloss.Style {
	return lipgloss.NewStyle().Background(lipgloss.Color(baseColor))
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
	if a == "" || b == "" {
		return 0
	}
	count := 0
	for count < max && a[len(a)-1-count] == b[len(b)-1-count] {
		count++
	}
	return count
}

// diffRowKind describes how a line pair relates in an aligned diff.
type diffRowKind int

const (
	diffEqual diffRowKind = iota
	diffDelete
	diffInsert
	diffReplace
)

// diffRow is one aligned row in a line diff.
type diffRow struct {
	left  string
	right string
	kind  diffRowKind
}

// alignLines produces LCS-aligned rows from old and new line slices.
func alignLines(oldLines, newLines []string) []diffRow {
	if len(oldLines) == 0 && len(newLines) == 0 {
		return nil
	}
	n, m := len(oldLines), len(newLines)
	// LCS table
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = 1 + dp[i+1][j+1]
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var rows []diffRow
	i, j := 0, 0
	for i < n && j < m {
		if oldLines[i] == newLines[j] {
			rows = append(rows, diffRow{left: oldLines[i], right: newLines[j], kind: diffEqual})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			rows = append(rows, diffRow{left: oldLines[i], right: "", kind: diffDelete})
			i++
		} else {
			rows = append(rows, diffRow{left: "", right: newLines[j], kind: diffInsert})
			j++
		}
	}
	for i < n {
		rows = append(rows, diffRow{left: oldLines[i], right: "", kind: diffDelete})
		i++
	}
	for j < m {
		rows = append(rows, diffRow{left: "", right: newLines[j], kind: diffInsert})
		j++
	}

	// Merge adjacent delete+insert into replace
	var merged []diffRow
	for k := 0; k < len(rows); k++ {
		if k+1 < len(rows) && rows[k].kind == diffDelete && rows[k+1].kind == diffInsert {
			merged = append(merged, diffRow{
				left:  rows[k].left,
				right: rows[k+1].right,
				kind:  diffReplace,
			})
			k++
			continue
		}
		merged = append(merged, rows[k])
	}
	return merged
}

// RenderEditDiff renders an aligned before/after diff for edit_file.
func RenderEditDiff(t Theme, oldStr, newStr string, width int) string {
	oldLines := splitLines(oldStr)
	newLines := splitLines(newStr)
	if len(oldLines) == 0 && len(newLines) == 0 {
		return ""
	}

	layout := NewLayout(width+contentMargin, 0)
	rows := alignLines(oldLines, newLines)

	if layout.DiffMode() == DiffSideBySide {
		return renderEditDiffSideBySide(t, rows, layout)
	}
	return renderEditDiffStacked(t, rows, layout)
}

func renderEditDiffSideBySide(t Theme, rows []diffRow, layout Layout) string {
	panelW := layout.DiffPanelWidth()
	var leftB, rightB strings.Builder

	leftB.WriteString("\n")
	rightB.WriteString("\n")

	for _, row := range rows {
		leftB.WriteString(renderDiffPanelLine(t, row.left, row.right, true, panelW, row.kind))
		leftB.WriteString("\n")
		rightB.WriteString(renderDiffPanelLine(t, row.right, row.left, false, panelW, row.kind))
		rightB.WriteString("\n")
	}

	leftStyle := lipgloss.NewStyle().Width(panelW).Padding(0, 1)
	rightStyle := lipgloss.NewStyle().Width(panelW).Padding(0, 1)
	gutter := t.Muted.Render(" │ ")

	leftPanel := leftStyle.Render(strings.TrimRight(leftB.String(), "\n"))
	rightPanel := rightStyle.Render(strings.TrimRight(rightB.String(), "\n"))

	return "    " + lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, gutter, rightPanel)
}

func renderEditDiffStacked(t Theme, rows []diffRow, layout Layout) string {
	panelW := layout.CardWidth() - 8
	if panelW < 20 {
		panelW = 20
	}
	var b strings.Builder

	b.WriteString("\n")
	for _, row := range rows {
		b.WriteString(renderDiffPanelLine(t, row.left, row.right, true, panelW, row.kind))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString("\n")
	for _, row := range rows {
		b.WriteString(renderDiffPanelLine(t, row.right, row.left, false, panelW, row.kind))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderDiffPanelLine(t Theme, line, counterpart string, isLeft bool, maxLen int, kind diffRowKind) string {
	prefix := "    "
	if line == "" {
		return prefix + t.DiffEmptyLine.Render(" ")
	}

	line = truncateLine(line, maxLen)

	switch {
	case kind == diffEqual:
		return prefix + t.DiffContext.Render(line)
	case isLeft && (kind == diffDelete || kind == diffReplace):
		return prefix + renderDiffRemovedLine(t, "", line, counterpart, maxLen)
	case !isLeft && (kind == diffInsert || kind == diffReplace):
		return prefix + renderDiffAddedLine(t, "", line, counterpart, maxLen)
	default:
		return prefix + t.DiffEmptyLine.Render(" ")
	}
}
