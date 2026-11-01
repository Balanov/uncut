//go:build windows

package harness

import (
	"context"
	"os/exec"
	"syscall"
)

func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd.exe")
	// /C receives the remainder of cmd.exe's raw command line, not a
	// Windows C-runtime quoted argument. Preserve the user's quoting and
	// wrap the complete command so quoted executable paths work as typed.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `cmd.exe /D /S /C "` + command + `"`,
	}
	return cmd
}
