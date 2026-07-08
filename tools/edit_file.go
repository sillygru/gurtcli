package tools

import (
	"fmt"
	"os"
	"strings"
)

func EditFile(workspaceRoot, filePath, oldString, newString string, allowedExternalDirs []string) (string, error) {
	safe, err := safePathWithExternals(workspaceRoot, filePath, allowedExternalDirs)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(safe)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	content := string(data)
	count := strings.Count(content, oldString)
	if count == 0 {
		return "", fmt.Errorf("oldString not found in %s", filePath)
	}
	if count > 1 {
		return "", fmt.Errorf("found %d matches for oldString in %s. Provide more surrounding context to identify the correct match", count, filePath)
	}

	newContent := strings.Replace(content, oldString, newString, 1)
	if err := os.WriteFile(safe, []byte(newContent), 0o644); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	return fmt.Sprintf("Successfully edited %s (%d bytes written)", filePath, len(newContent)), nil
}
