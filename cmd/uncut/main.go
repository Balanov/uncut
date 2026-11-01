package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/Balanov/uncut/internal/harness"
)

const version = "0.1.1"
const defaultSystemPrompt = "You are a coding agent working in a local workspace. Use the available tools to complete the user's request. Inspect before editing. Run relevant checks. Do not claim success without evidence. File tools are scoped to the workspace; shell commands run from its root."

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	invocationDir, err := os.Getwd()
	if err != nil {
		return fail(stderr, nil, err, 1)
	}
	flags := flag.NewFlagSet("uncut", flag.ContinueOnError)
	flags.SetOutput(stderr)
	model := flags.String("model", os.Getenv("UNCUT_MODEL"), "model id (or UNCUT_MODEL)")
	baseURL := flags.String("base-url", envOr("UNCUT_BASE_URL", "OPENAI_BASE_URL", "https://api.openai.com/v1"), "OpenAI-compatible base URL")
	workspace := flags.String("cwd", invocationDir, "workspace directory")
	sessionPath := flags.String("session", "", "append-only JSONL session; resume it when it exists")
	maxSteps := flags.Int("max-steps", 32, "maximum model calls per run")
	maxTokens := flags.Int("max-tokens", 0, "provider output limit; 0 uses the provider default")
	requestTimeout := flags.Duration("request-timeout", 10*time.Minute, "timeout for each provider request; 0 disables it")
	jsonOutput := flags.Bool("json", false, "write newline-delimited JSON events to stdout")
	noAgents := flags.Bool("no-agents", false, "do not read AGENTS.md")
	showVersion := flags.Bool("version", false, "print version and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "UNCUT — the lossless, headless coding-agent harness")
		fmt.Fprintln(stderr, "\nUsage:")
		fmt.Fprintln(stderr, "  uncut [flags] <prompt>")
		fmt.Fprintln(stderr, "  git diff | uncut [flags] \"review this\"")
		fmt.Fprintln(stderr, "\nFlags:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "uncut %s\n", version)
		return 0
	}
	if *model == "" || *maxSteps <= 0 || *maxTokens < 0 || *requestTimeout < 0 {
		flags.Usage()
		return 2
	}

	prompt, err := readPrompt(flags.Args(), stdin)
	if err != nil || strings.TrimSpace(prompt) == "" || !utf8.ValidString(prompt) {
		if err != nil {
			fmt.Fprintln(stderr, "uncut:", err)
		} else if !utf8.ValidString(prompt) {
			fmt.Fprintln(stderr, "uncut: prompt is not valid UTF-8")
		}
		flags.Usage()
		return 2
	}

	var emit func(harness.Event)
	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		emit = func(event harness.Event) {
			if err := encoder.Encode(event); err != nil {
				fmt.Fprintln(stderr, "uncut: encode output event:", err)
			}
		}
	}

	var session *harness.Session
	if *sessionPath != "" {
		if !filepath.IsAbs(*sessionPath) {
			*sessionPath = filepath.Join(invocationDir, *sessionPath)
		}
		session, err = harness.OpenSession(*sessionPath)
		if err != nil {
			return fail(stderr, emit, err, 1)
		}
		defer session.Close()
	}

	system := defaultSystemPrompt
	if !*noAgents {
		agentsPath := filepath.Join(*workspace, "AGENTS.md")
		content, readErr := os.ReadFile(agentsPath)
		if readErr == nil {
			if !utf8.Valid(content) {
				return fail(stderr, emit, errors.New("AGENTS.md is not valid UTF-8"), 1)
			}
			system += "\n\nProject instructions (AGENTS.md):\n" + string(content)
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return fail(stderr, emit, fmt.Errorf("read AGENTS.md: %w", readErr), 1)
		}
	}

	executor, err := harness.NewExecutor(*workspace)
	if err != nil {
		return fail(stderr, emit, err, 1)
	}
	provider := &harness.OpenAIProvider{
		BaseURL: *baseURL,
		APIKey:  envOr("UNCUT_API_KEY", "OPENAI_API_KEY", ""),
		Client:  &http.Client{Timeout: *requestTimeout},
	}
	runner := &harness.Runner{
		Provider: provider, Model: *model, MaxTokens: *maxTokens, MaxSteps: *maxSteps,
		System: system, Executor: executor, Session: session, Emit: emit,
	}
	if session != nil {
		runner.Messages = session.Messages()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := runner.Run(ctx, prompt)
	if err != nil {
		code := 1
		if errors.Is(err, harness.ErrProviderTruncated) {
			code = 3
		} else if errors.Is(err, harness.ErrMaxSteps) {
			code = 4
		}
		return fail(stderr, emit, err, code)
	}
	if !*jsonOutput {
		io.WriteString(stdout, result.Text)
		if !strings.HasSuffix(result.Text, "\n") {
			io.WriteString(stdout, "\n")
		}
	}
	return 0
}

func readPrompt(args []string, stdin *os.File) (string, error) {
	prompt := strings.Join(args, " ")
	info, err := stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect stdin: %w", err)
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		content, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		if prompt != "" && len(content) > 0 {
			prompt += "\n\n"
		}
		prompt += string(content)
	}
	return prompt, nil
}

func envOr(names ...string) string {
	var fallback string
	for i, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
		if i == len(names)-1 {
			fallback = name
		}
	}
	return fallback
}

func fail(stderr io.Writer, emit func(harness.Event), err error, code int) int {
	if emit != nil {
		emit(harness.Event{Type: "error", Error: err.Error()})
	} else {
		fmt.Fprintln(stderr, "uncut:", err)
	}
	return code
}
