package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type scriptedProvider struct {
	mu        sync.Mutex
	responses []CompletionResponse
	requests  []CompletionRequest
}

func (p *scriptedProvider) Complete(_ context.Context, request CompletionRequest) (CompletionResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, request)
	if len(p.responses) == 0 {
		return CompletionResponse{}, errors.New("no scripted response")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func assistantResponse(content string) CompletionResponse {
	return CompletionResponse{Choices: []Choice{{Message: Message{Role: "assistant", Content: content}, FinishReason: "stop"}}}
}

func toolResponse(id, name string, args any) CompletionResponse {
	encoded, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return CompletionResponse{Choices: []Choice{{Message: Message{
		Role:      "assistant",
		ToolCalls: []ToolCall{{ID: id, Type: "function", Function: FunctionCall{Name: name, Arguments: string(encoded)}}},
	}, FinishReason: "tool_calls"}}}
}

func TestRunnerSendsCompleteToolResultAndPersistsSession(t *testing.T) {
	root := t.TempDir()
	large := strings.Repeat("the complete tool result\n", 20000)
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.writeFile("large.txt", large); err != nil {
		t.Fatal(err)
	}
	provider := &scriptedProvider{responses: []CompletionResponse{
		toolResponse("call-1", "read_file", map[string]string{"path": "large.txt"}),
		assistantResponse("done"),
	}}
	session, err := OpenSession(filepath.Join(root, "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	runner := &Runner{Provider: provider, Model: "test", MaxSteps: 4, System: "system", Executor: executor, Session: session}
	result, err := runner.Run(context.Background(), "read it")
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "done" || result.Steps != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(provider.requests))
	}
	messages := provider.requests[1].Messages
	if len(messages) != 4 {
		t.Fatalf("second request has %d messages, want 4", len(messages))
	}
	if messages[3].Role != "tool" || messages[3].Content != large {
		t.Fatalf("tool result changed or was truncated: got %d bytes, want %d", len(messages[3].Content), len(large))
	}
	if got := len(session.Messages()); got != 5 {
		t.Fatalf("session contains %d messages, want 5", got)
	}
}

func TestRunnerResumeKeepsPreviousMessages(t *testing.T) {
	root := t.TempDir()
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(filepath.Join(root, "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	firstProvider := &scriptedProvider{responses: []CompletionResponse{assistantResponse("first answer")}}
	first := &Runner{Provider: firstProvider, Model: "test", MaxSteps: 2, System: "system", Executor: executor, Session: session}
	if _, err := first.Run(context.Background(), "first prompt"); err != nil {
		t.Fatal(err)
	}

	secondProvider := &scriptedProvider{responses: []CompletionResponse{assistantResponse("second answer")}}
	second := &Runner{Provider: secondProvider, Model: "test", MaxSteps: 2, System: "ignored on resume", Messages: session.Messages(), Executor: executor, Session: session}
	if _, err := second.Run(context.Background(), "second prompt"); err != nil {
		t.Fatal(err)
	}
	messages := secondProvider.requests[0].Messages
	if len(messages) != 4 {
		t.Fatalf("resumed request has %d messages, want 4", len(messages))
	}
	if messages[0].Content != "system" || messages[1].Content != "first prompt" || messages[2].Content != "first answer" || messages[3].Content != "second prompt" {
		t.Fatalf("resumed history changed: %+v", messages)
	}
}

func TestRunnerRejectsProviderTruncation(t *testing.T) {
	executor, err := NewExecutor(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	response := assistantResponse("partial")
	response.Choices[0].FinishReason = "length"
	provider := &scriptedProvider{responses: []CompletionResponse{response}}
	runner := &Runner{Provider: provider, Model: "test", MaxSteps: 2, System: "system", Executor: executor}
	result, err := runner.Run(context.Background(), "prompt")
	if !errors.Is(err, ErrProviderTruncated) {
		t.Fatalf("got error %v, want ErrProviderTruncated", err)
	}
	if result.Text != "" {
		t.Fatal("partial provider answer was accepted as final")
	}
}

func TestRunnerMaxSteps(t *testing.T) {
	executor, err := NewExecutor(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedProvider{responses: []CompletionResponse{
		toolResponse("call-1", "run_command", map[string]string{"command": "exit 0"}),
	}}
	runner := &Runner{Provider: provider, Model: "test", MaxSteps: 1, System: "system", Executor: executor}
	if _, err := runner.Run(context.Background(), "prompt"); !errors.Is(err, ErrMaxSteps) {
		t.Fatalf("got error %v, want ErrMaxSteps", err)
	}
}

// chatHistoryProblems lists violations of the Chat Completions message graph that
// make a provider reject an entire request: an assistant tool call must be
// answered by tool results before any other message follows, and an assistant
// message must carry either content or tool calls.
func chatHistoryProblems(messages []Message) []string {
	var problems []string
	var unanswered []string
	for i, message := range messages {
		if message.Role == "tool" {
			answered := false
			for j, id := range unanswered {
				if id == message.ToolCallID {
					unanswered = append(unanswered[:j], unanswered[j+1:]...)
					answered = true
					break
				}
			}
			if !answered {
				problems = append(problems, fmt.Sprintf("message %d is a tool result for tool call %q, which is not pending", i, message.ToolCallID))
			}
			continue
		}
		if len(unanswered) > 0 {
			problems = append(problems, fmt.Sprintf("message %d arrived while tool calls %v were unanswered", i, unanswered))
			unanswered = nil
		}
		if message.Role == "assistant" && len(message.ToolCalls) == 0 && message.Content == "" {
			problems = append(problems, fmt.Sprintf("message %d is an assistant message without content or tool calls", i))
		}
		for _, call := range message.ToolCalls {
			unanswered = append(unanswered, call.ID)
		}
	}
	if len(unanswered) > 0 {
		problems = append(problems, fmt.Sprintf("history ends with unanswered tool calls %v", unanswered))
	}
	return problems
}

// A rejected step must never reach the session: otherwise the next --session run
// sends a history the provider refuses, and the session cannot be resumed at all.
func TestRunnerRejectedStepsStayOutOfSession(t *testing.T) {
	truncatedToolCall := toolResponse("call-1", "read_file", map[string]string{"path": "note.txt"})
	truncatedToolCall.Choices[0].FinishReason = "length"
	truncatedText := assistantResponse("partial")
	truncatedText.Choices[0].FinishReason = "max_tokens"
	contentFiltered := assistantResponse("filtered")
	contentFiltered.Choices[0].FinishReason = "content_filter"
	unexpectedFinish := assistantResponse("why")
	unexpectedFinish.Choices[0].FinishReason = "end_turn"
	emptyFinal := assistantResponse("")
	missingID := toolResponse("", "read_file", map[string]string{"path": "note.txt"})
	missingName := toolResponse("call-1", "", map[string]string{"path": "note.txt"})

	for _, tc := range []struct {
		name      string
		response  CompletionResponse
		want      string
		truncated bool
	}{
		{"truncated tool call", truncatedToolCall, "truncated the response", true},
		{"truncated text", truncatedText, "truncated the response", true},
		{"content filter", contentFiltered, "content_filter", false},
		{"unexpected finish reason", unexpectedFinish, "stopped unexpectedly", false},
		{"empty final message", emptyFinal, "empty final message", false},
		{"tool call without id", missingID, "without an id or function name", false},
		{"tool call without name", missingName, "without an id or function name", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("note\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			executor, err := NewExecutor(root)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "session.jsonl")
			session, err := OpenSession(path)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			provider := &scriptedProvider{responses: []CompletionResponse{tc.response}}
			runner := &Runner{Provider: provider, Model: "test", MaxSteps: 4, System: "system", Executor: executor, Session: session}
			_, err = runner.Run(context.Background(), "prompt")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want %q", err, tc.want)
			}
			if tc.truncated && !errors.Is(err, ErrProviderTruncated) {
				t.Fatalf("got error %v, want ErrProviderTruncated", err)
			}
			if len(provider.requests) != 1 {
				t.Fatalf("got %d requests, want exactly one (no retry)", len(provider.requests))
			}
			// The prompt is kept, the rejected step is not.
			stored := session.Messages()
			if len(stored) != 2 || stored[0].Role != "system" || stored[1].Role != "user" {
				t.Fatalf("session kept the rejected step: %+v", stored)
			}
			reopened, err := OpenSession(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if problems := chatHistoryProblems(reopened.Messages()); len(problems) != 0 {
				t.Fatalf("stored session is unusable: %v", problems)
			}
		})
	}
}

func TestRunnerResumeAfterRejectedStepKeepsHistoryValid(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor, err := NewExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(filepath.Join(root, "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	rejected := toolResponse("call-1", "read_file", map[string]string{"path": "note.txt"})
	rejected.Choices[0].FinishReason = "length"
	first := &Runner{Provider: &scriptedProvider{responses: []CompletionResponse{rejected}},
		Model: "test", MaxSteps: 2, System: "system", Executor: executor, Session: session}
	if _, err := first.Run(context.Background(), "first prompt"); !errors.Is(err, ErrProviderTruncated) {
		t.Fatalf("got error %v, want ErrProviderTruncated", err)
	}

	provider := &scriptedProvider{responses: []CompletionResponse{assistantResponse("second answer")}}
	second := &Runner{Provider: provider, Model: "test", MaxSteps: 2, System: "ignored on resume",
		Messages: session.Messages(), Executor: executor, Session: session}
	if _, err := second.Run(context.Background(), "second prompt"); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	messages := provider.requests[0].Messages
	if problems := chatHistoryProblems(messages); len(problems) != 0 {
		t.Fatalf("resumed request would be rejected by the provider: %v", problems)
	}
	if len(messages) != 3 || messages[0].Role != "system" || messages[1].Content != "first prompt" || messages[2].Content != "second prompt" {
		t.Fatalf("resumed history changed: %+v", messages)
	}
}
