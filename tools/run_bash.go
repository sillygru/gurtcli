package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	DefaultTimeout = 30000
	MaxTimeout     = 300000 // 5 minutes
)

func RunBash(ctx context.Context, command string, timeout int) (string, error) {
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

	return b.String(), nil
}
