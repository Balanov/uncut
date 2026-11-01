package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

type Provider interface {
	Complete(context.Context, CompletionRequest) (CompletionResponse, error)
}

type OpenAIProvider struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

type ProviderError struct {
	StatusCode int
	Body       string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider returned HTTP %d: %s", e.StatusCode, e.Body)
}

func (p *OpenAIProvider) Complete(ctx context.Context, request CompletionRequest) (CompletionResponse, error) {
	var response CompletionResponse
	body, err := json.Marshal(request)
	if err != nil {
		return response, fmt.Errorf("encode request: %w", err)
	}

	endpoint := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return response, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "uncut/0.1")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return response, fmt.Errorf("call provider: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return response, fmt.Errorf("read provider response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return response, &ProviderError{StatusCode: resp.StatusCode, Body: string(responseBody)}
	}
	if !utf8.Valid(responseBody) {
		return response, errors.New("provider response is not valid UTF-8")
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return response, fmt.Errorf("decode provider response: %w", err)
	}
	if len(response.Choices) == 0 {
		return response, fmt.Errorf("provider returned no choices")
	}
	return response, nil
}
