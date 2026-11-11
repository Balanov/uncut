//go:build !windows

package harness

import (
	"context"
	"os/exec"
	"syscall"
)

func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	// Start the shell in its own process group and cancel the whole group.
	// The default cancel kills only /bin/sh; a child it started keeps running
	// and keeps the output pipe open, so run_command would block until that
	// child exits on its own.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return cmd
}
