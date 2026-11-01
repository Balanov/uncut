//go:build !windows

package harness

import (
	"context"
	"os/exec"
)

func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", command)
}
