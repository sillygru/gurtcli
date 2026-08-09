//go:build !windows

package tools

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// interruptKillWait is how long exec waits after a command was interrupted
// before force-killing the process group. SIGTERM on cancel gives the group a
// chance to shut down cleanly; this escalates for a group that ignores it.
const interruptKillWait = 3 * time.Second

// newBashCommand runs sh -c with process-group control so an interrupted run
// kills the whole tree (the shell and everything it spawned), not just the
// shell. exec.CommandContext alone only SIGKILLs the direct process, orphaning
// children like `node` or `make`.
func newBashCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Signaling the negative PID targets the whole process group sh
		// started, matching the interrupt a user typing Ctrl+C would expect.
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
			return err
		}
		return nil
	}
	cmd.WaitDelay = interruptKillWait
	return cmd
}
