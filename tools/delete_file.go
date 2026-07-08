package tools

import (
	"fmt"
	"os"
)

func DeleteFile(workspaceRoot, filePath string, allowedExternalDirs []string) (string, error) {
	safe, err := safePathWithExternals(workspaceRoot, filePath, allowedExternalDirs)
	if err != nil {
		return "", err
	}

	if err := os.Remove(safe); err != nil {
		return "", fmt.Errorf("deleting file: %w", err)
	}

	return fmt.Sprintf("Successfully deleted %s", filePath), nil
}
