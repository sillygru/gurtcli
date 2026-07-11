package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

type tableBlock struct {
	align  []string
	header []string
	rows   [][]string
}

func isTableCandidate(lines []string, i int) bool {
	if i >= len(lines) {
		return false
	}
	if !strings.Contains(lines[i], "|") {
		return false
	}
	if i+1 >= len(lines) {
		return false
	}
	return strings.Contains(lines[i+1], "|")
}

func parseTable(block []string) (tableBlock, bool) {
	if len(block) < 2 {
		return tableBlock{}, false
	}

	parseRow := func(line string) []string {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "|") {
			line = line[1:]
		}
		if strings.HasSuffix(line, "|") {
			line = line[:len(line)-1]
		}
		raw := strings.Split(line, "|")
		cells := make([]string, len(raw))
		for i, p := range raw {
			cells[i] = strings.TrimSpace(p)
		}
		return cells
	}

	sepLine := strings.TrimSpace(block[1])
	hasSep := strings.Contains(sepLine, "-")

	if hasSep {
		header := parseRow(block[0])
		if len(header) == 0 {
			return tableBlock{}, false
		}
		ncols := len(header)
		align := parseAlignment(sepLine, ncols)
		var rows [][]string
		for _, line := range block[2:] {
			if strings.TrimSpace(line) == "" {
				continue
			}
			cells := parseRow(line)
			if len(cells) < ncols {
				pad := make([]string, ncols-len(cells))
				cells = append(cells, pad...)
			}
			rows = append(rows, cells[:ncols])
		}
		return tableBlock{align: align, header: header, rows: rows}, true
	}

	header := parseRow(block[0])
	ncols := len(header)
	if ncols == 0 {
		return tableBlock{}, false
	}
	align := make([]string, ncols)
	for i := range align {
		align[i] = "left"
	}
	var rows [][]string
	for _, line := range block[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cells := parseRow(line)
		if len(cells) < ncols {
			pad := make([]string, ncols-len(cells))
			cells = append(cells, pad...)
		}
		rows = append(rows, cells[:ncols])
	}
	return tableBlock{align: align, header: header, rows: rows}, true
}

func parseAlignment(sepLine string, ncols int) []string {
	align := make([]string, ncols)
	s := strings.TrimSpace(sepLine)
	if strings.HasPrefix(s, "|") {
		s = s[1:]
	}
	if strings.HasSuffix(s, "|") {
		s = s[:len(s)-1]
	}
	parts := strings.Split(s, "|")
	for i := 0; i < ncols && i < len(parts); i++ {
		p := strings.TrimSpace(parts[i])
		switch {
		case strings.HasPrefix(p, ":") && strings.HasSuffix(p, ":"):
			align[i] = "center"
		case strings.HasSuffix(p, ":"):
			align[i] = "right"
		default:
			align[i] = "left"
		}
	}
	for i := range align {
		if align[i] == "" {
			align[i] = "left"
		}
	}
	return align
}

func renderTable(t Theme, block []string, width int) string {
	tbl, ok := parseTable(block)
	if !ok || len(tbl.header) == 0 {
		return ""
	}

	ncols := len(tbl.header)
	avail := width - 2
	if avail < 10 {
		avail = 10
	}

	cols := make([]int, ncols)
	for i, h := range tbl.header {
		cols[i] = runewidth.StringWidth(h)
	}
	for _, row := range tbl.rows {
		for i := 0; i < ncols && i < len(row); i++ {
			w := runewidth.StringWidth(row[i])
			if w > cols[i] {
				cols[i] = w
			}
		}
	}

	minTotal := 0
	for _, c := range cols {
		minTotal += c
	}
	minTotal += 3*ncols + 1

	surplus := avail - minTotal
	if surplus > 0 {
		totalNatural := 0
		for _, c := range cols {
			totalNatural += c
		}
		if totalNatural > 0 {
			allocated := 0
			for i := range cols {
				add := surplus * cols[i] / totalNatural
				cols[i] += add
				allocated += add
			}
			rem := surplus - allocated
			for i := 0; i < rem; i++ {
				cols[i]++
			}
		}
	}

	prefix := "  "
	borderStyle := t.TableBorder
	headerStyle := t.TableHeader
	cellStyle := t.TableCell

	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString(borderStyle.Render(buildTopBorder(cols)))
	b.WriteString("\n")

	b.WriteString(prefix)
	b.WriteString(borderStyle.Render("│"))
	for i, h := range tbl.header {
		b.WriteString(" ")
		b.WriteString(headerStyle.Render(padCell(h, cols[i], tbl.align[i])))
		b.WriteString(" ")
		if i < ncols-1 {
			b.WriteString(borderStyle.Render("│"))
		}
	}
	b.WriteString(borderStyle.Render("│"))
	b.WriteString("\n")

	b.WriteString(prefix)
	b.WriteString(borderStyle.Render(buildSepBorder(cols)))
	b.WriteString("\n")

	for _, row := range tbl.rows {
		b.WriteString(prefix)
		b.WriteString(borderStyle.Render("│"))
		for i := 0; i < ncols; i++ {
			cellText := ""
			if i < len(row) {
				cellText = row[i]
			}
			b.WriteString(" ")
			b.WriteString(cellStyle.Render(padCell(cellText, cols[i], tbl.align[i])))
			b.WriteString(" ")
			if i < ncols-1 {
				b.WriteString(borderStyle.Render("│"))
			}
		}
		b.WriteString(borderStyle.Render("│"))
		b.WriteString("\n")
	}

	b.WriteString(prefix)
	b.WriteString(borderStyle.Render(buildBottomBorder(cols)))

	return strings.TrimRight(b.String(), "\n")
}

func buildTopBorder(cols []int) string {
	var b strings.Builder
	b.WriteString("╭")
	for i, w := range cols {
		b.WriteString(strings.Repeat("─", w+2))
		if i < len(cols)-1 {
			b.WriteString("┬")
		}
	}
	b.WriteString("╮")
	return b.String()
}

func buildSepBorder(cols []int) string {
	var b strings.Builder
	b.WriteString("├")
	for i, w := range cols {
		b.WriteString(strings.Repeat("─", w+2))
		if i < len(cols)-1 {
			b.WriteString("┼")
		}
	}
	b.WriteString("┤")
	return b.String()
}

func buildBottomBorder(cols []int) string {
	var b strings.Builder
	b.WriteString("╰")
	for i, w := range cols {
		b.WriteString(strings.Repeat("─", w+2))
		if i < len(cols)-1 {
			b.WriteString("┴")
		}
	}
	b.WriteString("╯")
	return b.String()
}

func padCell(text string, width int, align string) string {
	n := runewidth.StringWidth(text)
	if n >= width {
		return text
	}
	switch align {
	case "right":
		return strings.Repeat(" ", width-n) + text
	case "center":
		left := (width - n) / 2
		right := width - n - left
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
	default:
		return text + strings.Repeat(" ", width-n)
	}
}

// RenderAssistantTable is exported for testing — renders a pipe-delimited
// table block as a box-drawing grid within the assistant content area.
func RenderAssistantTable(t Theme, block string, width int) string {
	lines := strings.Split(block, "\n")
	return renderTable(t, lines, width)
}

// TableContentAvailable returns true when content may contain renderable
// tables. Used to skip table detection overhead when no pipes are present.
func TableContentAvailable(content string) bool {
	return strings.Contains(content, "|")
}
