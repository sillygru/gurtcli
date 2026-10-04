package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTimeout = 30000
	MaxTimeout     = 300000 // 5 minutes

	DefaultMaxOutputChars = 20000
)

func RunBash(ctx context.Context, command string, timeout int, maxOutputChars int, sessionID, outputsDir string) (string, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	}
	if maxOutputChars <= 0 {
		maxOutputChars = DefaultMaxOutputChars
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := newBashCommand(ctx, command)
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
		out := b.String()
		if len(out) > maxOutputChars {
			out = fmt.Sprintf(
				"[output exceeded %d characters; showing the tail]\n\n%s",
				maxOutputChars, utf8Tail(out, maxOutputChars),
			)
		}
		if out == "" {
			return out, fmt.Errorf("command timed out after %dms", timeout)
		}
		return out, fmt.Errorf("command timed out after %dms\n\n%s", timeout, out)
	}

	result := b.String()

	// Bound the output on the failure path too. Previously only successful
	// commands were truncated, so a command that failed after emitting 5MB
	// (a stack trace, a full test run) returned all of it, and the model then
	// reasoned over a wall of text it never needed.
	if len(result) > maxOutputChars {
		savedPath, saveErr := saveLargeOutput(result, sessionID, outputsDir)
		if saveErr != nil {
			savedPath = ""
		}
		if savedPath != "" {
			result = fmt.Sprintf(
				"[output exceeded %d characters, full output saved to %s (use read_file to load it); showing the tail]\n\n%s",
				maxOutputChars, savedPath, utf8Tail(result, maxOutputChars),
			)
		} else {
			result = fmt.Sprintf(
				"[output exceeded %d characters and could not be saved to disk; showing the tail]\n\n%s",
				maxOutputChars, utf8Tail(result, maxOutputChars),
			)
		}
	}

	if err != nil {
		// The harness drops the separate return value when err != nil, so the
		// command's output must be carried inside the error or the model never
		// sees why it failed.
		if result == "" {
			return result, fmt.Errorf("command failed: %w", err)
		}
		return result, fmt.Errorf("command failed: %w\n\n%s", err, result)
	}

	return result, nil
}

// utf8Tail returns the last n bytes of s, shifted forward so the slice starts
// on a UTF-8 rune boundary (byte slicing can otherwise split a multi-byte
// rune, leaving invalid bytes in the model-visible output).
func utf8Tail(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if n >= len(s) {
		return s
	}
	start := len(s) - n
	for start < len(s) && s[start]&0xC0 == 0x80 {
		start++
	}
	return s[start:]
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
