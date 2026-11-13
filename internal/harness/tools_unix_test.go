//go:build !windows

package harness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunCommandCancellationStopsChildProcesses(t *testing.T) {
	root := t.TempDir()
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	// The shell forks sleep and waits for it, like `cd dir && go test ./...`.
	// Killing only /bin/sh leaves sleep holding the output pipe.
	_, err = executor.runCommand(ctx, "sleep 20 & echo $! > child.pid; wait")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got error %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("run_command returned %v after cancellation; it waited for the orphaned child", elapsed.Round(time.Millisecond))
	}
	pidText, err := os.ReadFile(filepath.Join(root, "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("child process %d survived cancellation", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestKillProcessGroupAfterExitSignalsNothing(t *testing.T) {
	cmd := newShellCommand(context.Background(), "exit 0")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	// The shell has been reaped and its group is empty, as when Cancel races
	// with Wait. Nothing may be signalled; exec must keep the exit status.
	if err := killProcessGroup(cmd.Process.Pid); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("got error %v, want os.ErrProcessDone", err)
	}
}

func TestKillProcessGroupAfterShellExitStopsChild(t *testing.T) {
	root := t.TempDir()
	cmd := newShellCommand(context.Background(), "sleep 20 & echo $! > child.pid")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	pidText, err := os.ReadFile(filepath.Join(root, "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	// The shell is gone but the child it started is still in the group, so a
	// probe of the shell's pid alone would skip the kill and leave it running.
	if err := killProcessGroup(cmd.Process.Pid); err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("child process %d survived killProcessGroup", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
