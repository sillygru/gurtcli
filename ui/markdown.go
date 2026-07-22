package ui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	reHeading1 = regexp.MustCompile(`^#\s+(.+)$`)
	reHeading2 = regexp.MustCompile(`^##\s+(.+)$`)
	reHeading3 = regexp.MustCompile(`^###\s+(.+)$`)
	reBullet   = regexp.MustCompile(`^[-*+]\s+(.+)$`)
	reNumbered = regexp.MustCompile(`^(\d+)\.\s+(.+)$`)
)

// renderMarkdownContent renders assistant markdown with block and inline styling.
func renderMarkdownContent(t Theme, content string, width int, commands []string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	// Every branch below indents by two cells, so that indent comes out of the
	// width the text is wrapped to rather than being added on top of it.
	bodyWidth := LayoutForContent(width).ContentWidth() - mdIndent
	if bodyWidth < 1 {
		bodyWidth = 1
	}

	var b strings.Builder
	doHL := len(commands) > 0
	i := 0
	for i < len(lines) {
		line := lines[i]

		// Fenced code block
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			j := i + 1
			for j < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[j]), "```") {
				j++
			}
			if j < len(lines) {
				// The code style paints a background and pads inside it, and
				// that padding is drawn outside the text it wraps — measure it
				// rather than assume it.
				codeWidth := bodyWidth - lipgloss.Width(t.CodeBlock.Render(""))
				if codeWidth < 1 {
					codeWidth = 1
				}
				for k := i + 1; k < j; k++ {
					// Hard wrap, not word wrap: code reads by column, and a
					// long line is worth an extra row rather than a cut.
					for _, row := range strings.Split(ansi.Hardwrap(lines[k], codeWidth, true), "\n") {
						b.WriteString(t.CodeBlock.Render("  " + row))
						b.WriteString("\n")
					}
				}
				i = j + 1
				continue
			}
		}

		// Table block
		if isTableCandidate(lines, i) {
			j := i + 1
			for j < len(lines) && strings.Contains(lines[j], "|") {
				j++
			}
			b.WriteString(renderTable(t, lines[i:j], width))
			b.WriteString("\n")
			i = j
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			i++
			continue
		}

		// Headings
		if m := reHeading3.FindStringSubmatch(trimmed); m != nil {
			writeHeading(&b, t.Heading3, t, m[1], bodyWidth, commands, doHL)
			i++
			continue
		}
		if m := reHeading2.FindStringSubmatch(trimmed); m != nil {
			writeHeading(&b, t.Heading2, t, m[1], bodyWidth, commands, doHL)
			i++
			continue
		}
		if m := reHeading1.FindStringSubmatch(trimmed); m != nil {
			writeHeading(&b, t.Heading1, t, m[1], bodyWidth, commands, doHL)
			i++
			continue
		}

		// Bullet list
		if m := reBullet.FindStringSubmatch(trimmed); m != nil {
			writeListItem(&b, t.ListBullet, "• ", t, m[1], bodyWidth, commands, doHL)
			i++
			continue
		}

		// Numbered list
		if m := reNumbered.FindStringSubmatch(trimmed); m != nil {
			writeListItem(&b, t.ListNumber, m[1]+". ", t, m[2], bodyWidth, commands, doHL)
			i++
			continue
		}

		// Paragraph
		for _, subLine := range wrapRows(line, bodyWidth) {
			b.WriteString("  ")
			b.WriteString(renderInlineMarkdown(t, subLine, commands, doHL))
			b.WriteString("\n")
		}
		i++
	}
	return strings.TrimRight(b.String(), "\n")
}

// mdIndent is the two-cell indent every markdown block is drawn at.
const mdIndent = 2

// wrapRows wraps text to width, breaking inside a word only when the word is
// wider than the row it has to fit in. The text is still raw markdown here, so
// the rows come out a little narrower than the budget once the ** and ` markers
// are stripped — narrower is the safe direction.
func wrapRows(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	return strings.Split(ansi.Wrap(text, width, ""), "\n")
}

// writeHeading writes a heading wrapped to the body width, every row indented.
func writeHeading(b *strings.Builder, style lipgloss.Style, t Theme, text string, bodyWidth int, commands []string, doHL bool) {
	for _, row := range wrapRows(text, bodyWidth) {
		b.WriteString(style.Render("  " + renderInlineMarkdown(t, row, commands, doHL)))
		b.WriteString("\n")
	}
}

// writeListItem writes one list entry, aligning continuation rows under the
// text rather than under the bullet so the item still reads as one item.
func writeListItem(b *strings.Builder, markerStyle lipgloss.Style, marker string, t Theme, text string, bodyWidth int, commands []string, doHL bool) {
	markerWidth := ansi.StringWidth(marker)
	for k, row := range wrapRows(text, bodyWidth-markerWidth) {
		b.WriteString("  ")
		if k == 0 {
			b.WriteString(markerStyle.Render(marker))
		} else {
			b.WriteString(strings.Repeat(" ", markerWidth))
		}
		b.WriteString(renderInlineMarkdown(t, row, commands, doHL))
		b.WriteString("\n")
	}
}

// renderInlineMarkdown applies inline bold, italic, code, and file/cmd refs.
func renderInlineMarkdown(t Theme, line string, commands []string, doHL bool) string {
	if line == "" {
		return t.AssistantContent.Render("")
	}
	segments := parseInlineSegments(line)
	if len(segments) == 0 {
		if doHL {
			return HighlightInline(line, t.AssistantContent, t.FileRef, t.CmdRef, commands)
		}
		return t.AssistantContent.Render(line)
	}

	var b strings.Builder
	for _, seg := range segments {
		text := seg.text
		switch seg.kind {
		case "bold":
			if doHL {
				b.WriteString(HighlightInline(text, t.Bold, t.FileRef, t.CmdRef, commands))
			} else {
				b.WriteString(t.Bold.Render(text))
			}
		case "italic":
			if doHL {
				b.WriteString(HighlightInline(text, t.Italic, t.FileRef, t.CmdRef, commands))
			} else {
				b.WriteString(t.Italic.Render(text))
			}
		case "code":
			b.WriteString(t.InlineCode.Render(text))
		default:
			if doHL {
				b.WriteString(HighlightInline(text, t.AssistantContent, t.FileRef, t.CmdRef, commands))
			} else {
				b.WriteString(t.AssistantContent.Render(text))
			}
		}
	}
	return b.String()
}

type inlineSegment struct {
	kind string
	text string
}

func parseInlineSegments(line string) []inlineSegment {
	var segments []inlineSegment
	i := 0
	for i < len(line) {
		// Bold **text**
		if i+1 < len(line) && line[i] == '*' && line[i+1] == '*' {
			end := strings.Index(line[i+2:], "**")
			if end >= 0 {
				if i > 0 {
					segments = append(segments, inlineSegment{kind: "plain", text: line[:i]})
				}
				segments = append(segments, inlineSegment{kind: "bold", text: line[i+2 : i+2+end]})
				line = line[i+2+end+2:]
				i = 0
				continue
			}
		}
		// Inline code `text`
		if line[i] == '`' {
			end := strings.Index(line[i+1:], "`")
			if end >= 0 {
				if i > 0 {
					segments = append(segments, inlineSegment{kind: "plain", text: line[:i]})
				}
				segments = append(segments, inlineSegment{kind: "code", text: line[i+1 : i+1+end]})
				line = line[i+1+end+1:]
				i = 0
				continue
			}
		}
		// Italic *text* (single asterisk, not **)
		if line[i] == '*' && (i+1 >= len(line) || line[i+1] != '*') {
			end := strings.Index(line[i+1:], "*")
			if end >= 0 {
				if i > 0 {
					segments = append(segments, inlineSegment{kind: "plain", text: line[:i]})
				}
				segments = append(segments, inlineSegment{kind: "italic", text: line[i+1 : i+1+end]})
				line = line[i+1+end+1:]
				i = 0
				continue
			}
		}
		i++
	}
	if len(segments) == 0 {
		return nil
	}
	if line != "" {
		segments = append(segments, inlineSegment{kind: "plain", text: line})
	}
	return segments
}
