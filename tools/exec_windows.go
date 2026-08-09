//go:build windows

package tools

import (
	"context"
	"os/exec"
)

// newBashCommand on Windows runs sh -c like any other platform. There are no
// POSIX process groups to signal here, so interruption relies on
// exec.CommandContext's default of killing the direct process.
func newBashCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", command)
}
