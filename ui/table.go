package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
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
	if avail < 1 {
		avail = 1
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

	// The grid itself costs "│ " plus " " per column and a closing "│".
	chrome := 3*ncols + 1
	budget := avail - chrome
	if budget < ncols*minTableColWidth {
		// Not even a stub of every column fits — a grid here would be borders
		// and single letters. Render the rows as prose instead.
		return renderStackedTable(t, tbl, avail)
	}

	natural := 0
	for _, c := range cols {
		natural += c
	}

	switch {
	case natural > budget:
		shrinkColumns(cols, budget)
	case natural < budget && natural > 0:
		surplus := budget - natural
		allocated := 0
		for i := range cols {
			add := surplus * cols[i] / natural
			cols[i] += add
			allocated += add
		}
		for i := 0; i < surplus-allocated; i++ {
			cols[i]++
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

	writeTableRow(&b, t, prefix, tbl.header, cols, tbl.align, headerStyle)

	b.WriteString(prefix)
	b.WriteString(borderStyle.Render(buildSepBorder(cols)))
	b.WriteString("\n")

	for _, row := range tbl.rows {
		writeTableRow(&b, t, prefix, row, cols, tbl.align, cellStyle)
	}

	b.WriteString(prefix)
	b.WriteString(borderStyle.Render(buildBottomBorder(cols)))

	return strings.TrimRight(b.String(), "\n")
}

// minTableColWidth is the narrowest a column is squeezed to before the grid is
// abandoned in favour of the stacked layout.
const minTableColWidth = 3

// shrinkColumns scales columns down to budget, taking the cells from the widest
// columns first so a table of one long prose column and three short ones loses
// its width where there is width to lose. Nothing is dropped: whatever no
// longer fits on a cell's row wraps onto the next one.
func shrinkColumns(cols []int, budget int) {
	total := 0
	for _, c := range cols {
		total += c
	}
	if total <= budget || total == 0 {
		return
	}

	for i := range cols {
		scaled := cols[i] * budget / total
		if scaled < minTableColWidth {
			scaled = minTableColWidth
		}
		cols[i] = scaled
	}

	// Rounding and the floor can leave the row a few cells over or under.
	for {
		total = 0
		widest := 0
		for i, c := range cols {
			total += c
			if c > cols[widest] {
				widest = i
			}
		}
		if total <= budget {
			break
		}
		if cols[widest] <= minTableColWidth {
			return // every column is at the floor; the caller's fallback applies
		}
		cols[widest]--
	}
	for i := 0; total < budget; i = (i + 1) % len(cols) {
		cols[i]++
		total++
	}
}

// writeTableRow writes one logical row, wrapping each cell over as many
// physical rows as it needs and padding the cells that ran out of text.
func writeTableRow(b *strings.Builder, t Theme, prefix string, cells []string, cols []int, align []string, style lipgloss.Style) {
	ncols := len(cols)
	wrapped := make([][]string, ncols)
	height := 1
	for i := range wrapped {
		text := ""
		if i < len(cells) {
			text = cells[i]
		}
		wrapped[i] = wrapRows(text, cols[i])
		if len(wrapped[i]) > height {
			height = len(wrapped[i])
		}
	}

	borderStyle := t.TableBorder
	for line := 0; line < height; line++ {
		b.WriteString(prefix)
		b.WriteString(borderStyle.Render("│"))
		for i := 0; i < ncols; i++ {
			text := ""
			if line < len(wrapped[i]) {
				text = wrapped[i][line]
			}
			b.WriteString(" ")
			b.WriteString(style.Render(padCell(text, cols[i], align[i])))
			b.WriteString(" ")
			if i < ncols-1 {
				b.WriteString(borderStyle.Render("│"))
			}
		}
		b.WriteString(borderStyle.Render("│"))
		b.WriteString("\n")
	}
}

// renderStackedTable is the phone-width fallback: one "header: value" line per
// cell, wrapped, with a blank row between records. Borders would eat more of
// the screen than the data at these widths.
func renderStackedTable(t Theme, tbl tableBlock, avail int) string {
	var b strings.Builder
	for r, row := range tbl.rows {
		if r > 0 {
			b.WriteString("\n")
		}
		for i, head := range tbl.header {
			value := ""
			if i < len(row) {
				value = row[i]
			}
			if value == "" {
				continue
			}
			label := head
			if label == "" {
				label = "—"
			}
			// Wrapped to the deepest indent the block uses, so a continuation
			// row is no wider than the first one.
			for k, line := range wrapRows(label+": "+value, avail-4) {
				if k == 0 {
					b.WriteString("  ")
				} else {
					b.WriteString("    ")
				}
				b.WriteString(t.TableCell.Render(line))
				b.WriteString("\n")
			}
		}
	}
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
