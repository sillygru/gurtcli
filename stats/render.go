package stats

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

const barChar = "━"

var (
	mauve    = "\033[38;2;203;166;247m"
	lavender = "\033[38;2;180;190;254m"
	blue     = "\033[38;2;137;180;250m"
	teal     = "\033[38;2;148;226;213m"
	surface2 = "\033[38;2;88;91;112m"
	overlay0 = "\033[38;2;108;112;134m"
	overlay1 = "\033[38;2;127;132;156m"
	green    = "\033[38;2;166;227;161m"
	bold     = "\033[1m"
	reset    = "\033[0m"
)

func init() {
	fi, _ := os.Stdout.Stat()
	if fi == nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		mauve = ""
		lavender = ""
		blue = ""
		teal = ""
		surface2 = ""
		overlay0 = ""
		overlay1 = ""
		green = ""
		bold = ""
		reset = ""
	}
}

func Render(w io.Writer, s *Stats) {
	termWidth := guessWidth()
	innerWidth := termWidth - 4
	if innerWidth < 20 {
		innerWidth = 20
	}

	renderOverview(w, s, innerWidth)
	fmt.Fprintln(w)
	renderTools(w, s, innerWidth)
}

func renderOverview(w io.Writer, s *Stats, inner int) {
	top(w, inner)
	header(w, "OVERVIEW", inner)
	sep(w, inner)

	rows := []struct {
		label string
		value int
	}{
		{"Sessions", s.Sessions},
		{"User Messages", s.UserMessages},
		{"API Calls", s.APICalls},
		{"Days", s.Days},
	}
	for _, r := range rows {
		label := r.label
		val := formatInt(r.value)
		fill := inner - runewidth.StringWidth(label) - runewidth.StringWidth(val) - 2
		if fill < 0 {
			fill = 0
		}
		fmt.Fprintf(w, "%s│ %s%s%s %s%s │%s\n",
			surface2, overlay1, label,
			strings.Repeat(" ", fill),
			teal, val, reset,
		)
	}

	bottom(w, inner)
}

func renderTools(w io.Writer, s *Stats, inner int) {
	if len(s.Tools) == 0 {
		return
	}

	maxName := 0
	maxCount := 0
	for _, t := range s.Tools {
		n := runewidth.StringWidth(t.Name)
		if n > maxName {
			maxName = n
		}
		c := runewidth.StringWidth(formatInt(t.Count))
		if c > maxCount {
			maxCount = c
		}
	}
	pctWidth := 8
	minBar := 5
	barArea := inner - maxName - maxCount - pctWidth - 4
	if barArea < minBar {
		barArea = minBar
	}

	maxCountVal := s.Tools[0].Count
	if maxCountVal == 0 {
		maxCountVal = 1
	}

	totalToolCalls := 0
	for _, t := range s.Tools {
		totalToolCalls += t.Count
	}
	if totalToolCalls == 0 {
		totalToolCalls = 1
	}

	top(w, inner)
	header(w, "TOOL USAGE", inner)
	sep(w, inner)

	for _, t := range s.Tools {
		pct := float64(t.Count) / float64(totalToolCalls) * 100
		barLen := 0
		if maxCountVal > 0 {
			barLen = t.Count * barArea / maxCountVal
		}
		if barLen == 0 && t.Count > 0 {
			barLen = 1
		}
		bar := strings.Repeat(barChar, barLen)

		name := t.Name + strings.Repeat(" ", maxName-runewidth.StringWidth(t.Name))
		count := formatInt(t.Count)
		countPad := strings.Repeat(" ", maxCount-runewidth.StringWidth(count))
		pctStr := fmt.Sprintf("(%5.1f%%)", pct)

		fmt.Fprintf(w, "%s│ %s%s %s%s%s%s%s %s%s %s│%s\n",
			surface2,
			blue, name,
			mauve, bar,
			strings.Repeat(" ", barArea-runewidth.StringWidth(bar)),
			teal, countPad+count,
			overlay0, pctStr,
			surface2, reset,
		)
	}

	bottom(w, inner)
}

func top(w io.Writer, inner int) {
	fmt.Fprintf(w, "%s┌%s┐%s\n", surface2, strings.Repeat("─", inner+2), reset)
}

func header(w io.Writer, title string, inner int) {
	titleW := runewidth.StringWidth(title)
	left := (inner - titleW) / 2
	right := inner - titleW - left
	fmt.Fprintf(w, "%s│%s%s%s%s%s%s%s│%s\n",
		surface2,
		strings.Repeat(" ", left+1),
		bold, mauve, title, reset,
		surface2, strings.Repeat(" ", right+1),
		reset,
	)
}

func sep(w io.Writer, inner int) {
	fmt.Fprintf(w, "%s├%s┤%s\n", surface2, strings.Repeat("─", inner+2), reset)
}

func bottom(w io.Writer, inner int) {
	fmt.Fprintf(w, "%s└%s┘%s\n", surface2, strings.Repeat("─", inner+2), reset)
}

func formatInt(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	s := fmt.Sprintf("%d", n)
	var parts []string
	for i := len(s); i > 0; i -= 3 {
		start := i - 3
		if start < 0 {
			start = 0
		}
		parts = append([]string{s[start:i]}, parts...)
	}
	return strings.Join(parts, ",")
}

func guessWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		return w
	}
	return 80
}
