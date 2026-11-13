package harness

import (
	"context"
	"errors"
	"fmt"
)

var ErrProviderTruncated = errors.New("provider truncated the response")
var ErrMaxSteps = errors.New("maximum model steps reached")

type Runner struct {
	Provider  Provider
	Model     string
	MaxTokens int
	MaxSteps  int
	System    string
	Messages  []Message
	Executor  *Executor
	Session   *Session
	Emit      func(Event)
}

func (r *Runner) Run(ctx context.Context, prompt string) (Result, error) {
	var result Result
	if r.Provider == nil {
		return result, errors.New("provider is required")
	}
	if r.Model == "" {
		return result, errors.New("model is required")
	}
	if r.MaxSteps <= 0 {
		return result, errors.New("max steps must be positive")
	}
	if r.Executor == nil {
		return result, errors.New("executor is required")
	}

	r.Messages = cloneMessages(r.Messages)
	if len(r.Messages) == 0 {
		if r.System == "" {
			return result, errors.New("system prompt is required")
		}
		if err := r.append(Message{Role: "system", Content: r.System}, false); err != nil {
			return result, err
		}
	}
	if err := r.append(Message{Role: "user", Content: prompt}, true); err != nil {
		return result, err
	}

	for step := 1; step <= r.MaxSteps; step++ {
		response, err := r.Provider.Complete(ctx, CompletionRequest{
			Model: r.Model, Messages: cloneMessages(r.Messages), Tools: ToolDefinitions(), MaxTokens: r.MaxTokens,
		})
		if err != nil {
			return result, err
		}
		choice := response.Choices[0]
		message := cloneMessage(choice.Message)
		if message.Role == "" {
			message.Role = "assistant"
		}
		// Reject before committing. A rejected step is not part of the
		// conversation: storing it would append an unanswered tool call or an
		// empty assistant message to the session, and every later --session run
		// would send a history the provider refuses.
		if err := validateStep(choice, message); err != nil {
			return result, err
		}
		if err := r.append(message, true); err != nil {
			return result, err
		}
		r.emit(Event{Type: "assistant_meta", FinishReason: choice.FinishReason, Usage: usagePointer(response.Usage), Steps: step})
		result.Usage.Add(response.Usage)
		result.Steps = step

		if len(message.ToolCalls) == 0 {
			result.Text = message.Content
			r.emit(Event{Type: "done", Steps: result.Steps, Usage: usagePointer(result.Usage)})
			return result, nil
		}

		for _, call := range message.ToolCalls {
			content, err := r.Executor.Execute(ctx, call)
			if err != nil {
				content = "error: " + err.Error()
			}
			toolMessage := Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: content}
			if err := r.append(toolMessage, true); err != nil {
				return result, err
			}
		}
	}
	return result, fmt.Errorf("%w (%d); increase --max-steps to continue", ErrMaxSteps, r.MaxSteps)
}

// validateStep reports why a provider step cannot join the conversation.
// Every error here is returned before the step reaches the session or the event
// stream, so the stored history only ever contains steps UNCUT accepted.
func validateStep(choice Choice, message Message) error {
	switch choice.FinishReason {
	case "length", "max_tokens":
		return fmt.Errorf("%w (finish_reason=%s); no partial answer is accepted", ErrProviderTruncated, choice.FinishReason)
	case "content_filter":
		return errors.New("provider stopped with finish_reason=content_filter")
	}
	if len(message.ToolCalls) == 0 {
		if choice.FinishReason != "" && choice.FinishReason != "stop" {
			return fmt.Errorf("provider stopped unexpectedly with finish_reason=%s", choice.FinishReason)
		}
		if message.Content == "" {
			return errors.New("provider returned an empty final message")
		}
		return nil
	}
	for _, call := range message.ToolCalls {
		if call.ID == "" || call.Function.Name == "" {
			return errors.New("provider returned a tool call without an id or function name")
		}
	}
	return nil
}

func (r *Runner) append(message Message, emit bool) error {
	if r.Session != nil {
		if err := r.Session.Append(message); err != nil {
			return err
		}
	}
	r.Messages = append(r.Messages, cloneMessage(message))
	if emit {
		r.emit(Event{Type: message.Role, Message: messagePointer(message)})
	}
	return nil
}

func (r *Runner) emit(event Event) {
	if r.Emit != nil {
		r.Emit(event)
	}
}

func cloneMessages(messages []Message) []Message {
	cloned := make([]Message, len(messages))
	for i, message := range messages {
		cloned[i] = cloneMessage(message)
	}
	return cloned
}

func messagePointer(message Message) *Message {
	cloned := cloneMessage(message)
	return &cloned
}

func usagePointer(usage Usage) *Usage {
	return &usage
}
