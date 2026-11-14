//go:build !windows

package harness

import (
	"context"
	"errors"
	"os"
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
	cmd.Cancel = func() error { return killProcessGroup(cmd.Process.Pid) }
	return cmd
}

// killProcessGroup kills the process group that Setpgid created for the shell.
// Its id is the shell's pid, which Start sets once; Cancel runs only after a
// successful Start, so pgid is always that value.
//
// Cancel can race with Wait and run after the shell has been reaped. The group
// is probed first, not the shell: the shell may be gone while a child it
// started is still in the group and holding the output pipe, and that child
// must still be killed. When no process is left in the group the probe
// returns ESRCH and nothing is signalled, so a pid the kernel has since
// recycled is never hit. os.ErrProcessDone tells exec that the command had
// already finished, so Wait keeps the command's own exit status.
func killProcessGroup(pgid int) error {
	if err := syscall.Kill(-pgid, 0); errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
