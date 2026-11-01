package harness

import (
	"context"
	"encoding/json"
	"errors"
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
