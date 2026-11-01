package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Executor struct {
	root string
}

func NewExecutor(root string) (*Executor, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("open workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory: %s", absolute)
	}
	return &Executor{root: absolute}, nil
}

func (e *Executor) Execute(ctx context.Context, call ToolCall) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Old     string `json:"old"`
		New     string `json:"new"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("invalid tool arguments: %w", err)
	}

	switch call.Function.Name {
	case "read_file":
		return e.readFile(args.Path)
	case "write_file":
		return e.writeFile(args.Path, args.Content)
	case "replace":
		return e.replace(args.Path, args.Old, args.New)
	case "run_command":
		return e.runCommand(ctx, args.Command)
	default:
		return "", fmt.Errorf("unknown tool %q", call.Function.Name)
	}
}

func (e *Executor) resolve(path string) (string, error) {
	if path == "" {
		return "", errors.New("path is required")
	}
	var resolved string
	if filepath.IsAbs(path) {
		resolved = filepath.Clean(path)
	} else {
		resolved = filepath.Join(e.root, path)
	}
	relative, err := filepath.Rel(e.root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}
	return resolved, nil
}

func (e *Executor) readFile(path string) (string, error) {
	resolved, err := e.resolve(path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(content) {
		return "", fmt.Errorf("%s is not valid UTF-8; binary files are not supported", path)
	}
	return string(content), nil
}

func (e *Executor) writeFile(path, content string) (string, error) {
	resolved, err := e.resolve(path)
	if err != nil {
		return "", err
	}
	if !utf8.ValidString(content) {
		return "", errors.New("content is not valid UTF-8")
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), path), nil
}

func (e *Executor) replace(path, old, new string) (string, error) {
	if old == "" {
		return "", errors.New("old must not be empty")
	}
	resolved, err := e.resolve(path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(content) || !utf8.ValidString(old) || !utf8.ValidString(new) {
		return "", errors.New("replace supports valid UTF-8 text only")
	}
	count := bytes.Count(content, []byte(old))
	if count != 1 {
		return "", fmt.Errorf("old occurs %d times; exactly one occurrence is required", count)
	}
	updated := bytes.Replace(content, []byte(old), []byte(new), 1)
	if err := os.WriteFile(resolved, updated, 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("replaced %d bytes with %d bytes in %s", len(old), len(new), path), nil
}

func (e *Executor) runCommand(ctx context.Context, command string) (string, error) {
	if command == "" {
		return "", errors.New("command is required")
	}
	cmd := newShellCommand(ctx, command)
	cmd.Dir = e.root
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			return "", err
		}
		exitCode = exitError.ExitCode()
	}
	if !utf8.Valid(output) {
		return "", errors.New("command output is not valid UTF-8; refusing to replace bytes silently")
	}
	return fmt.Sprintf("exit_code=%d\n%s", exitCode, output), nil
}

func ToolDefinitions() []ToolDefinition {
	pathProperty := map[string]any{"type": "string", "description": "Workspace-relative path."}
	return []ToolDefinition{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "read_file",
				Description: "Read an entire UTF-8 text file. The result is never truncated.",
				Parameters: map[string]any{
					"type": "object", "properties": map[string]any{"path": pathProperty},
					"required": []string{"path"}, "additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "write_file",
				Description: "Write the complete contents of a UTF-8 text file, creating parent directories.",
				Parameters: map[string]any{
					"type": "object", "properties": map[string]any{"path": pathProperty, "content": map[string]any{"type": "string"}},
					"required": []string{"path", "content"}, "additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "replace",
				Description: "Replace exactly one occurrence of old with new in a UTF-8 text file.",
				Parameters: map[string]any{
					"type": "object", "properties": map[string]any{"path": pathProperty, "old": map[string]any{"type": "string"}, "new": map[string]any{"type": "string"}},
					"required": []string{"path", "old", "new"}, "additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "run_command",
				Description: "Run a shell command in the workspace. Return the complete combined stdout/stderr and exit code without truncation.",
				Parameters: map[string]any{
					"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}},
					"required": []string{"command"}, "additionalProperties": false,
				},
			},
		},
	}
}
