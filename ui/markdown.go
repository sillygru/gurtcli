package ui

import (
	"regexp"
	"strings"

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
	layout := NewLayout(width+contentMargin, 0)
	wrapWidth := layout.ContentWidth()
	if wrapWidth < 10 {
		wrapWidth = 10
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
				for k := i + 1; k < j; k++ {
					b.WriteString(t.CodeBlock.Render("  " + lines[k]))
					b.WriteString("\n")
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
			b.WriteString(t.Heading3.Render("  " + renderInlineMarkdown(t, m[1], commands, doHL)))
			b.WriteString("\n")
			i++
			continue
		}
		if m := reHeading2.FindStringSubmatch(trimmed); m != nil {
			b.WriteString(t.Heading2.Render("  " + renderInlineMarkdown(t, m[1], commands, doHL)))
			b.WriteString("\n")
			i++
			continue
		}
		if m := reHeading1.FindStringSubmatch(trimmed); m != nil {
			b.WriteString(t.Heading1.Render("  " + renderInlineMarkdown(t, m[1], commands, doHL)))
			b.WriteString("\n")
			i++
			continue
		}

		// Bullet list
		if m := reBullet.FindStringSubmatch(trimmed); m != nil {
			b.WriteString("  ")
			b.WriteString(t.ListBullet.Render("• "))
			b.WriteString(renderInlineMarkdown(t, m[1], commands, doHL))
			b.WriteString("\n")
			i++
			continue
		}

		// Numbered list
		if m := reNumbered.FindStringSubmatch(trimmed); m != nil {
			b.WriteString("  ")
			b.WriteString(t.ListNumber.Render(m[1] + ". "))
			b.WriteString(renderInlineMarkdown(t, m[2], commands, doHL))
			b.WriteString("\n")
			i++
			continue
		}

		// Paragraph
		wrapped := ansi.Hardwrap(line, wrapWidth, true)
		for _, subLine := range strings.Split(wrapped, "\n") {
			b.WriteString("  ")
			b.WriteString(renderInlineMarkdown(t, subLine, commands, doHL))
			b.WriteString("\n")
		}
		i++
	}
	return strings.TrimRight(b.String(), "\n")
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
