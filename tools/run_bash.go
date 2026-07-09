package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTimeout = 30000
	MaxTimeout     = 300000 // 5 minutes

	maxOutputLen = 5000
)

func RunBash(ctx context.Context, command string, timeout int, sessionID, outputsDir string) (string, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var b strings.Builder
	if stdout.Len() > 0 {
		b.WriteString(strings.TrimSpace(stdout.String()))
	}
	if stderr.Len() > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimSpace(stderr.String()))
	}

	if ctx.Err() == context.DeadlineExceeded {
		return b.String(), fmt.Errorf("command timed out after %dms", timeout)
	}

	if err != nil {
		exitErr := ""
		if b.Len() > 0 {
			exitErr = fmt.Sprintf("\n%s", b.String())
		}
		return b.String(), fmt.Errorf("command failed: %w%s", err, exitErr)
	}

	result := b.String()

	if len(result) > maxOutputLen && sessionID != "" && outputsDir != "" {
		savedPath, saveErr := saveLargeOutput(result, sessionID, outputsDir)
		if saveErr == nil {
			truncated := result[:maxOutputLen]
			result = fmt.Sprintf(
				"Output > %d characters, saved to %s (use read_file to load it)\n\n%s",
				maxOutputLen, savedPath, truncated,
			)
		}
	}

	return result, nil
}

func saveLargeOutput(content, sessionID, outputsDir string) (string, error) {
	if err := os.MkdirAll(outputsDir, 0700); err != nil {
		return "", fmt.Errorf("creating outputs dir: %w", err)
	}

	seq := nextOutputSeq(outputsDir, sessionID)
	filename := fmt.Sprintf("%s_%d.out", sessionID, seq)
	path := filepath.Join(outputsDir, filename)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing output file: %w", err)
	}

	return path, nil
}

func nextOutputSeq(dir, sessionID string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	prefix := sessionID + "_"
	maxSeq := -1
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".out") {
			seqStr := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".out")
			if seq, err := strconv.Atoi(seqStr); err == nil && seq > maxSeq {
				maxSeq = seq
			}
		}
	}
	return maxSeq + 1
}