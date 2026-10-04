package tools

import (
	"fmt"
	"os"
	"strings"
)

func ReadFile(workspaceRoot, filePath string, offset, limit int, allowedExternalDirs []string) (string, error) {
	safe, err := safePathWithExternals(workspaceRoot, filePath, allowedExternalDirs)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(safe)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	// Remove trailing empty line from split
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	totalLines := len(lines)

	if offset == 0 {
		offset = 1
	}
	if offset < 1 {
		offset = 1
	}
	if limit <= 0 {
		limit = totalLines
	}

	start := offset - 1
	if start >= totalLines {
		return "", fmt.Errorf("offset %d exceeds file length (%d lines)", offset, totalLines)
	}

	end := start + limit
	if end > totalLines {
		end = totalLines
	}

	selected := lines[start:end]
	var b strings.Builder
	fmt.Fprintf(&b, "File: %s (%d lines total)", filePath, totalLines)
	if offset > 1 || limit < totalLines {
		fmt.Fprintf(&b, " [showing lines %d-%d]", offset, end)
	}
	b.WriteString("\n")
	for i, line := range selected {
		fmt.Fprintf(&b, "%d: %s\n", start+i+1, line)
	}

	// State the truncation explicitly. A silently shortened read is
	// indistinguishable from a complete one, so the model will quote a
	// fragment as if it were the whole file. Only annotate when a whole-file
	// read was actually narrowed, since that is the case that misleads.
	if end < totalLines {
		fmt.Fprintf(&b, "[showing %d of %d lines; the rest of the file was NOT read. Use offset=%d to continue.]",
			len(selected), totalLines, end+1)
	}

	return b.String(), nil
}
