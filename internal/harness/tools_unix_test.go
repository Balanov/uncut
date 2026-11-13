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
