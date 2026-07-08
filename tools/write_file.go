package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

func WriteFile(workspaceRoot, filePath, content string, allowedExternalDirs []string) (string, error) {
	safe, err := safePathWithExternals(workspaceRoot, filePath, allowedExternalDirs)
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(safe)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating parent directories: %w", err)
	}

	if err := os.WriteFile(safe, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), filePath), nil
}
