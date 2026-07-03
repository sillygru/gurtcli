package tools

import (
	"fmt"
	"os"
)

func DeleteFile(workspaceRoot, filePath string) (string, error) {
	safe, err := safePath(workspaceRoot, filePath)
	if err != nil {
		return "", err
	}

	if err := os.Remove(safe); err != nil {
		return "", fmt.Errorf("deleting file: %w", err)
	}

	return fmt.Sprintf("Successfully deleted %s", filePath), nil
}
