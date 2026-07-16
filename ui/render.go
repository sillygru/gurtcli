package ui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sillygru/gurtcli/llm"
)

const maxBashResultLines = 10

// RenderUnifiedToolCard renders a tool call and its result together in one bordered card.
func RenderUnifiedToolCard(t Theme, tc llm.ToolCall, resultContent string, width int, isError bool) string {
	accent := t.ToolAccentFor(tc.Function.Name)
	args := parseToolArgs(tc.Function.Arguments)

	if tc.Function.Name == "read_file" {
		return renderReadFileLine(t, args, resultContent, isError)
	}

	var body strings.Builder
	switch tc.Function.Name {
	case "run_bash":
		renderBashArgs(&body, t, args)
	case "edit_file":
		renderEditArgs(&body, t, args, width)
	case "write_file":
		renderWriteArgs(&body, t, args)
	case "delete_file":
		renderDeleteArgs(&body, t, args)
	default:
		renderGenericArgs(&body, t, args)
	}

	preview := toolResultPreview(resultContent, tc.Function.Name)
	if preview != "" {
		if body.Len() > 0 {
			body.WriteString("\n")
		}
		for _, line := range strings.Split(preview, "\n") {
			style := t.ToolCode
			if isError {
				style = t.ToolResultErr
			}
			body.WriteString(style.Render("    " + line))
			body.WriteString("\n")
		}
	}

	header := renderToolHeader(t, accent)
	content := header
	if body.Len() > 0 {
		content += "\n" + body.String()
	}

	return wrapToolCard(t, accent, content, NewLayout(width, 0).CardWidth())
}

func renderReadFileLine(t Theme, args map[string]interface{}, resultContent string, isError bool) string {
	path, _ := args["filePath"].(string)
	if path == "" {
		path = "(unknown)"
	}

	icon := "◈"
	label := "Read"
	accentColor := t.Blue

	iconStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(accentColor)).Bold(true).Render(icon + " " + label)
	pathStyled := t.ToolPath.Render(" " + shortenPath(path))

	line := "  " + iconStyled + pathStyled

	if isError {
		errMsg := firstLineTrimmed(resultContent, 60)
		line += " " + t.ToolResultErr.Render("✕ "+errMsg)
	}

	return line
}

// RenderToolCall renders a tool invocation as a bordered card.
func RenderToolCall(t Theme, tc llm.ToolCall, width int) string {
	accent := t.ToolAccentFor(tc.Function.Name)
	args := parseToolArgs(tc.Function.Arguments)

	if tc.Function.Name == "read_file" {
		return renderReadFileLine(t, args, "", false)
	}

	var body strings.Builder
	switch tc.Function.Name {
	case "run_bash":
		renderBashArgs(&body, t, args)
	case "edit_file":
		renderEditArgs(&body, t, args, width)
	case "write_file":
		renderWriteArgs(&body, t, args)
	case "delete_file":
		renderDeleteArgs(&body, t, args)
	default:
		renderGenericArgs(&body, t, args)
	}

	header := renderToolHeader(t, accent)
	content := header
	if body.Len() > 0 {
		content += "\n" + body.String()
	}

	return wrapToolCard(t, accent, content, NewLayout(width, 0).CardWidth())
}

// RenderToolResult renders the output of a completed tool call in a bordered card.
func RenderToolResult(t Theme, toolName, content string, width int, isError bool) string {
	if content == "" {
		return ""
	}

	if toolName == "read_file" || toolName == "edit_file" {
		return ""
	}

	accent := t.ToolAccentFor(toolName)

	var body strings.Builder
	body.WriteString(renderToolHeader(t, accent))

	preview := toolResultPreview(content, toolName)
	if preview != "" {
		body.WriteString("\n")
		for _, line := range strings.Split(preview, "\n") {
			style := t.ToolCode
			if isError {
				style = t.ToolResultErr
			}
			body.WriteString(style.Render("    " + line))
			body.WriteString("\n")
		}
	}

	return wrapToolCard(t, accent, body.String(), NewLayout(width, 0).CardWidth())
}

// highlightInline finds @file and /command references in a single line of text
// and returns the line with those references styled using the provided styles.
func HighlightInline(line string, base, fileRef, cmdRef lipgloss.Style, commands []string) string {
	cmdSet := make(map[string]bool, len(commands))
	for _, c := range commands {
		cmdSet[c] = true
	}

	type span struct {
		start, end int
		style      lipgloss.Style
	}
	var spans []span

	// Find @file references
	for i := 0; i < len(line); i++ {
		if line[i] == '@' {
			j := i + 1
			for j < len(line) && line[j] != ' ' {
				j++
			}
			if j > i+1 {
				spans = append(spans, span{i, j, fileRef})
				i = j - 1
			}
		}
	}

	// Find /command references — only match at start of line (position 0)
	if len(line) > 0 && line[0] == '/' {
		for cmd := range cmdSet {
			if 1+len(cmd) <= len(line) && line[1:1+len(cmd)] == cmd {
				end := 1 + len(cmd)
				if end == len(line) || line[end] == ' ' {
					spans = append(spans, span{0, end, cmdRef})
					break
				}
			}
		}
	}

	if len(spans) == 0 {
		return base.Render(line)
	}

	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	// Merge overlapping spans
	merged := []span{spans[0]}
	for i := 1; i < len(spans); i++ {
		last := &merged[len(merged)-1]
		if spans[i].start <= last.end && spans[i].end > last.end {
			last.end = spans[i].end
		} else if spans[i].start > last.end {
			merged = append(merged, spans[i])
		}
	}

	var b strings.Builder
	pos := 0
	for _, s := range merged {
		if s.start > pos {
			b.WriteString(base.Render(line[pos:s.start]))
		}
		b.WriteString(s.style.Render(line[s.start:s.end]))
		pos = s.end
	}
	if pos < len(line) {
		b.WriteString(base.Render(line[pos:]))
	}
	return b.String()
}

// RenderUserMessage renders a user message in a bordered card with inline highlighting.
func RenderUserMessage(t Theme, content string, width int, commands []string) string {
	wrapWidth := width - 2
	if wrapWidth < 10 {
		wrapWidth = 10
	}

	var body strings.Builder
	doHL := len(commands) > 0
	for _, line := range strings.Split(content, "\n") {
		wrapped := ansi.Hardwrap(line, wrapWidth, true)
		for _, subLine := range strings.Split(wrapped, "\n") {
			if doHL {
				body.WriteString(HighlightInline(subLine, t.UserContent, t.FileRef, t.CmdRef, commands))
			} else {
				body.WriteString(t.UserContent.Render(subLine))
			}
			body.WriteString("\n")
		}
	}

	label := t.UserBoxLabel.Render("You")
	inner := label + "\n" + strings.TrimRight(body.String(), "\n")
	return t.UserBox.Width(width).Render(inner)
}

// RenderAssistantLabel renders the assistant label with the model name.
func RenderAssistantLabel(t Theme, name string) string {
	return t.AssistantLabel.Render("  " + name)
}

// RenderAssistantContent renders assistant text with markdown styling.
// Table blocks (pipe-delimited) are detected and rendered as box-drawing grids.
// Inline @file and /command references are highlighted when commands is non-nil.
func RenderAssistantContent(t Theme, content string, width int, commands []string) string {
	return renderMarkdownContent(t, content, width, commands)
}

// PermissionOptions returns the permission option labels for a tool.
// This is the single source of truth for option count and order.
// bashPrefix is embedded for run_bash labels.
// externalPath is non-empty when the tool targets a path outside the workspace.
// sudo is true when the command starts with "sudo".
func PermissionOptions(toolName, bashPrefix, externalPath string, sudo bool) []string {
	if sudo {
		return []string{
			"Yes, enter sudo password",
			"No",
		}
	}
	if externalPath != "" {
		return []string{
			"Yes",
			"Allow this directory for this session",
			"Allow every directory for this session",
			"Always allow every directory outside working space (forever)",
			"No",
		}
	}
	switch toolName {
	case "run_bash":
		return []string{
			"Yes",
			"Allow " + bashPrefix + " for this session",
			"Always allow " + bashPrefix,
			"Allow everything for this session",
			"No",
		}
	case "edit_file", "write_file":
		return []string{
			"Yes",
			"Allow every edit for this session",
			"Allow everything for this session",
			"Always allow edits",
			"No",
		}
	case "delete_file":
		return []string{
			"Yes",
			"Allow deletion of files for this session",
			"Allow everything for this session",
			"No",
		}
	default:
		return []string{"Yes", "Allow everything for this session", "No"}
	}
}

// RenderPermissionPrompt renders the permission confirmation box content.
// cursor is the index of the currently selected option.
// bashPrefix is the command prefix to display in bash-related options, if any.
// externalPath is the external file path being accessed, if any (triggers external access prompt).
// sudo is true when the command starts with "sudo" (shows a simplified prompt).
func RenderPermissionPrompt(t Theme, tc llm.ToolCall, width int, cursor int, bashPrefix string, externalPath string, sudo bool) string {
	content := RenderToolCall(t, tc, width) + "\n\n"
	if sudo {
		content += t.PermPrompt.Render("  This command requires sudo (administrator privileges).")
		content += "\n"
		content += t.PermPrompt.Render("  You will be asked to enter your password if you approve.")
		content += "\n\n"
	} else if externalPath != "" {
		content += t.PermPrompt.Render("  External path: " + externalPath)
		content += "\n\n"
	}

	options := PermissionOptions(tc.Function.Name, bashPrefix, externalPath, sudo)

	b := new(strings.Builder)
	b.WriteString(content)
	b.WriteString(t.PermPrompt.Render("  Allow this action?"))
	b.WriteString("\n\n")
	for i, opt := range options {
		prefix := "  "
		style := t.Dim
		if i == cursor {
			prefix = "> "
			style = t.Header
		}
		b.WriteString(style.Render(prefix + opt))
		b.WriteString("\n")
	}
	return b.String()
}

func renderToolHeader(t Theme, accent ToolAccent) string {
	badge := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Base)).
		Bold(true).
		Foreground(lipgloss.Color(accent.Color))

	icon := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Base)).
		Foreground(lipgloss.Color(accent.Color)).
		Bold(true).
		Render(accent.Icon)

	return "  " + icon + " " + badge.Render(accent.Label)
}

func wrapToolCard(t Theme, accent ToolAccent, content string, width int) string {
	card := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Base)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(accent.Color)).
		Padding(0, 1).
		Margin(0, 0, 1, 0).
		Width(width)

	return card.Render(content)
}

func parseToolArgs(raw string) map[string]interface{} {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil
	}
	return args
}

func renderBashArgs(b *strings.Builder, t Theme, args map[string]interface{}) {
	if title, ok := args["title"].(string); ok && title != "" {
		b.WriteString(t.ToolTitle.Render("    " + title))
		b.WriteString("\n")
	}
	if cmd, ok := args["command"].(string); ok && cmd != "" {
		lines := strings.Split(strings.TrimSpace(cmd), "\n")
		for _, line := range lines {
			b.WriteString(t.ToolCode.Render("    " + line))
			b.WriteString("\n")
		}
	}
}

func renderEditArgs(b *strings.Builder, t Theme, args map[string]interface{}, width int) {
	if path, ok := args["filePath"].(string); ok && path != "" {
		b.WriteString(t.ToolPath.Render("    " + shortenPath(path)))
		b.WriteString("\n")
	}

	oldStr, _ := args["oldString"].(string)
	newStr, _ := args["newString"].(string)
	if oldStr == "" && newStr == "" {
		return
	}

	diff := RenderEditDiff(t, oldStr, newStr, width)
	if diff != "" {
		b.WriteString(diff)
		b.WriteString("\n")
	}
}

func renderWriteArgs(b *strings.Builder, t Theme, args map[string]interface{}) {
	path, _ := args["filePath"].(string)
	content, _ := args["content"].(string)

	if path != "" {
		b.WriteString(t.ToolPath.Render("    " + shortenPath(path)))
		b.WriteString("\n")
	}

	if content == "" {
		return
	}

	lines := splitLines(content)
	lineCount := len(lines)
	byteCount := len(content)

	b.WriteString(t.ToolMeta.Render(fmt.Sprintf("    %d lines · %s", lineCount, formatBytes(byteCount))))
	b.WriteString("\n")

	for _, line := range lines {
		b.WriteString(t.ToolCode.Render("    " + line))
		b.WriteString("\n")
	}
}

func renderReadArgs(b *strings.Builder, t Theme, args map[string]interface{}) {
	if path, ok := args["filePath"].(string); ok && path != "" {
		b.WriteString(t.ToolPath.Render("    " + shortenPath(path)))
		b.WriteString("\n")
	}

	var meta []string
	if offset, ok := args["offset"].(float64); ok && offset > 0 {
		meta = append(meta, fmt.Sprintf("from line %d", int(offset)))
	}
	if limit, ok := args["limit"].(float64); ok && limit > 0 {
		meta = append(meta, fmt.Sprintf("%d lines", int(limit)))
	}
	if len(meta) > 0 {
		b.WriteString(t.ToolMeta.Render("    " + strings.Join(meta, " · ")))
		b.WriteString("\n")
	}
}

func renderDeleteArgs(b *strings.Builder, t Theme, args map[string]interface{}) {
	if path, ok := args["filePath"].(string); ok && path != "" {
		b.WriteString(t.ToolPath.Render("    " + shortenPath(path)))
		b.WriteString("\n")
	}
}

func renderGenericArgs(b *strings.Builder, t Theme, args map[string]interface{}) {
	if path, ok := args["filePath"].(string); ok && path != "" {
		b.WriteString(t.ToolPath.Render("    " + shortenPath(path)))
		b.WriteString("\n")
	}
}

func firstLineTrimmed(content string, maxLen int) string {
	first := strings.SplitN(strings.TrimSpace(content), "\n", 2)[0]
	if len(first) > maxLen {
		first = first[:maxLen-3] + "…"
	}
	return first
}

func toolResultPreview(content, toolName string) string {
	lines := splitLines(content)
	if len(lines) == 0 {
		return ""
	}

	switch toolName {
	case "read_file", "edit_file":
		return ""
	case "run_bash":
		return joinPreviewLines(lines, maxBashResultLines)
	default:
		return joinPreviewLines(lines, maxBashResultLines)
	}
}

func joinPreviewLines(lines []string, max int) string {
	if len(lines) == 0 {
		return ""
	}
	end := len(lines)
	if end > max {
		end = max
	}
	out := strings.Join(lines[:end], "\n")
	if len(lines) > max {
		out += fmt.Sprintf("\n… %d more lines", len(lines)-max)
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func shortenPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "/" {
		return path
	}
	// Keep last two path segments when long
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) <= 3 {
		return path
	}
	return "…/" + strings.Join(parts[len(parts)-2:], "/")
}

func truncateLine(s string, max int) string {
	if max <= 3 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func formatBytes(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1f KB", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
