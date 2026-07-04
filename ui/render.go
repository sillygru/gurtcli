package ui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sillygru/gurtcli/llm"
)

const (
	defaultCardWidth = 68
	maxPreviewLines  = 8
	maxResultLines   = 6
)

// RenderToolCall renders a tool invocation as a bordered card.
func RenderToolCall(t Theme, tc llm.ToolCall, width int) string {
	accent := ToolAccentFor(tc.Function.Name)
	args := parseToolArgs(tc.Function.Arguments)

	var body strings.Builder
	switch tc.Function.Name {
	case "run_bash":
		renderBashArgs(&body, t, args)
	case "edit_file":
		renderEditArgs(&body, t, args, width)
	case "write_file":
		renderWriteArgs(&body, t, args)
	case "read_file":
		renderReadArgs(&body, t, args)
	case "delete_file":
		renderDeleteArgs(&body, t, args)
	default:
		renderGenericArgs(&body, t, args)
	}

	header := renderToolHeader(accent)
	content := header
	if body.Len() > 0 {
		content += "\n" + body.String()
	}

	return wrapToolCard(accent.Color, content, cardWidth(width))
}

// RenderToolResult renders the output of a completed tool call.
func RenderToolResult(t Theme, toolName, content string, width int) string {
	if content == "" {
		return ""
	}

	lower := strings.ToLower(content)
	isErr := strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "timed out")

	icon := "✓"
	resultStyle := t.ToolResultOK
	if isErr {
		icon = "✕"
		resultStyle = t.ToolResultErr
	}

	summary := summarizeToolResult(toolName, content)
	header := resultStyle.Render(fmt.Sprintf("  %s %s", icon, summary))

	var body strings.Builder
	preview := toolResultPreview(content, toolName)
	if preview != "" {
		body.WriteString(t.ToolResultBody.Render(preview))
	}

	if body.Len() == 0 {
		return header
	}
	return header + "\n" + body.String()
}

// RenderUserMessage renders a user message block.
func RenderUserMessage(t Theme, content string) string {
	var b strings.Builder
	b.WriteString(t.UserLabel.Render("  you"))
	b.WriteString("\n")
	for _, line := range strings.Split(content, "\n") {
		b.WriteString(t.UserContent.Render("  " + line))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderAssistantLabel renders the assistant brand label.
func RenderAssistantLabel(t Theme) string {
	return t.AssistantLabel.Render("  gurt")
}

// RenderAssistantContent renders assistant text with a subtle left guide.
func RenderAssistantContent(t Theme, content string) string {
	if content == "" {
		return ""
	}
	guide := t.Divider.Render("│")
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		b.WriteString("  ")
		b.WriteString(guide)
		b.WriteString(" ")
		b.WriteString(t.AssistantContent.Render(line))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderPermissionPrompt renders the permission confirmation box content.
func RenderPermissionPrompt(t Theme, tc llm.ToolCall, width int) string {
	return RenderToolCall(t, tc, width) + "\n\n" +
		t.PermPrompt.Render("  Allow this action?") + " " +
		t.PermKey.Render("y") + t.Dim.Render("es  ") +
		t.PermKey.Render("n") + t.Dim.Render("o  ") +
		t.PermKey.Render("a") + t.Dim.Render("ll")
}

func renderToolHeader(accent ToolAccent) string {
	badge := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(accent.Color))

	icon := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accent.Color)).
		Bold(true).
		Render(accent.Icon)

	return "  " + icon + " " + badge.Render(accent.Label)
}

func wrapToolCard(accentColor, content string, width int) string {
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(accentColor)).
		Padding(0, 1).
		Margin(0, 0, 1, 0).
		Width(width)

	return card.Render(content)
}

func cardWidth(terminalWidth int) int {
	if terminalWidth <= 0 {
		return defaultCardWidth
	}
	w := terminalWidth - 6
	if w < 36 {
		w = 36
	}
	if w > defaultCardWidth {
		w = defaultCardWidth
	}
	return w
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
		b.WriteString(t.ToolMeta.Render("    " + title))
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

	oldLines := splitLines(oldStr)
	newLines := splitLines(newStr)

	if len(oldLines) > 0 {
		b.WriteString(t.DiffContext.Render("    removed"))
		b.WriteString("\n")
		for i, line := range oldLines {
			if i >= maxPreviewLines {
				b.WriteString(t.Muted.Render(fmt.Sprintf("    … %d more lines", len(oldLines)-maxPreviewLines)))
				b.WriteString("\n")
				break
			}
			counterpart := ""
			if i < len(newLines) {
				counterpart = newLines[i]
			}
			b.WriteString(renderDiffRemovedLine(t, "    − ", line, counterpart, width-8))
			b.WriteString("\n")
		}
	}

	if len(newLines) > 0 {
		b.WriteString(t.DiffContext.Render("    added"))
		b.WriteString("\n")
		for i, line := range newLines {
			if i >= maxPreviewLines {
				b.WriteString(t.Muted.Render(fmt.Sprintf("    … %d more lines", len(newLines)-maxPreviewLines)))
				b.WriteString("\n")
				break
			}
			counterpart := ""
			if i < len(oldLines) {
				counterpart = oldLines[i]
			}
			b.WriteString(renderDiffAddedLine(t, "    + ", line, counterpart, width-8))
			b.WriteString("\n")
		}
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

	previewCount := maxPreviewLines
	if previewCount > lineCount {
		previewCount = lineCount
	}
	for i := 0; i < previewCount; i++ {
		b.WriteString(t.ToolCode.Render("    " + truncateLine(lines[i], 60)))
		b.WriteString("\n")
	}
	if lineCount > maxPreviewLines {
		b.WriteString(t.Muted.Render(fmt.Sprintf("    … %d more lines", lineCount-maxPreviewLines)))
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

func summarizeToolResult(toolName, content string) string {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
		first := strings.SplitN(strings.TrimSpace(content), "\n", 2)[0]
		if len(first) > 72 {
			first = first[:69] + "…"
		}
		return first
	}

	switch toolName {
	case "read_file":
		if idx := strings.Index(content, "("); idx > 0 {
			return strings.TrimSpace(content[:idx])
		}
		return "Read complete"
	case "write_file":
		if strings.HasPrefix(content, "Successfully wrote") {
			return content
		}
		return "Write complete"
	case "edit_file":
		return "Edit applied"
	case "delete_file":
		return "File deleted"
	case "run_bash":
		lines := strings.Split(strings.TrimSpace(content), "\n")
		if len(lines) == 0 {
			return "Command finished"
		}
		if len(lines) == 1 {
			line := lines[0]
			if len(line) > 60 {
				line = line[:57] + "…"
			}
			return line
		}
		return fmt.Sprintf("%d lines of output", len(lines))
	default:
		first := strings.SplitN(strings.TrimSpace(content), "\n", 2)[0]
		if len(first) > 72 {
			first = first[:69] + "…"
		}
		return first
	}
}

func toolResultPreview(content, toolName string) string {
	lines := splitLines(content)
	if len(lines) == 0 {
		return ""
	}

	switch toolName {
	case "read_file":
		// Skip the "File: ..." header line
		start := 0
		if strings.HasPrefix(lines[0], "File:") {
			start = 1
		}
		return joinPreviewLines(lines[start:], maxResultLines)
	case "run_bash":
		return joinPreviewLines(lines, maxResultLines)
	default:
		if len(lines) == 1 && len(lines[0]) < 80 {
			return ""
		}
		return joinPreviewLines(lines, maxResultLines)
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
