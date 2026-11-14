package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadFileReturnsEveryByte(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("0123456789 abcdefghijklmnopqrstuvwxyz — полный контекст\n", 20000)
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := executor.readFile("large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != content {
		t.Fatalf("read_file changed or truncated content: got %d bytes, want %d", len(got), len(content))
	}
}

func TestRunCommandReturnsCompleteOutput(t *testing.T) {
	root := t.TempDir()
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	line := "12345678901234567890123456789012345678901234567890"
	const repetitions = 5000
	var command, expected string
	if runtime.GOOS == "windows" {
		command = fmt.Sprintf("for /L %%i in (1,1,%d) do @echo %s", repetitions, line)
		expected = strings.Repeat(line+"\r\n", repetitions)
	} else {
		command = fmt.Sprintf("i=0; while [ $i -lt %d ]; do echo %s; i=$((i+1)); done", repetitions, line)
		expected = strings.Repeat(line+"\n", repetitions)
	}
	got, err := executor.runCommand(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if got != "exit_code=0\n"+expected {
		t.Fatalf("run_command changed or truncated output: got %d bytes, want %d", len(got), len("exit_code=0\n"+expected))
	}
}

func TestReplaceRequiresExactlyOneMatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("before old after old tail"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.replace("file.txt", "old", "new"); err == nil {
		t.Fatal("expected ambiguous replacement to fail")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "before old after old tail" {
		t.Fatal("failed replacement modified the file")
	}
	if _, err := executor.replace("file.txt", "after old", "new"); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "before old new tail" {
		t.Fatalf("unexpected replacement result: %q", content)
	}
}

func TestFileToolsRejectWorkspaceEscape(t *testing.T) {
	executor, err := NewExecutor(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.readFile("../secret.txt"); err == nil {
		t.Fatal("expected workspace escape to fail")
	}
}

func TestRunCommandSupportsQuotedExecutable(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows cmd.exe quoting")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "directory with spaces")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(directory, "check.cmd")
	if err := os.WriteFile(script, []byte("@echo off\r\n@echo script-ok %1\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := executor.runCommand(context.Background(), `"`+script+`" "hello world"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "exit_code=0\nscript-ok \"hello world\"\r\n" {
		t.Fatalf("unexpected quoted command result: %q", got)
	}
}

func TestReadFileRejectsInvalidUTF8(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "binary.dat"), []byte{0xff, 0xfe, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.readFile("binary.dat"); err == nil {
		t.Fatal("expected invalid UTF-8 to fail explicitly")
	}
}

func TestWriteFileRequiresContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, arguments := range []string{`{"path":"notes.md"}`, `{"path":"notes.md","content":null}`, `{"path":"fresh.md"}`} {
		call := ToolCall{Function: FunctionCall{Name: "write_file", Arguments: arguments}}
		if _, err := executor.Execute(context.Background(), call); err == nil || err.Error() != "content is required" {
			t.Fatalf("%s: expected content is required, got %v", arguments, err)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "keep me" {
		t.Fatalf("refused write_file modified the file: %q", content)
	}
	if _, err := os.Stat(filepath.Join(root, "fresh.md")); !os.IsNotExist(err) {
		t.Fatalf("refused write_file created a file: %v", err)
	}
	call := ToolCall{Function: FunctionCall{Name: "write_file", Arguments: `{"path":"notes.md","content":""}`}}
	if _, err := executor.Execute(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 0 {
		t.Fatalf("explicit empty content was not written: %q", content)
	}
}

func TestReplaceRequiresNew(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("before old after"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, arguments := range []string{`{"path":"file.txt","old":"old "}`, `{"path":"file.txt","old":"old ","new":null}`} {
		call := ToolCall{Function: FunctionCall{Name: "replace", Arguments: arguments}}
		if _, err := executor.Execute(context.Background(), call); err == nil || err.Error() != "new is required" {
			t.Fatalf("%s: expected new is required, got %v", arguments, err)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "before old after" {
		t.Fatalf("refused replace modified the file: %q", content)
	}
	call := ToolCall{Function: FunctionCall{Name: "replace", Arguments: `{"path":"file.txt","old":"old ","new":""}`}}
	if _, err := executor.Execute(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "before after" {
		t.Fatalf("explicit empty new did not delete the match: %q", content)
	}
}
