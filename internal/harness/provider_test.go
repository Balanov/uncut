package harness

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProviderRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing authorization header")
		}
		var request CompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "test-model" || len(request.Messages) != 1 || request.Messages[0].Content != "hello" {
			t.Errorf("unexpected request: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CompletionResponse{Choices: []Choice{{Message: Message{Role: "assistant", Content: "ok"}, FinishReason: "stop"}}})
	}))
	defer server.Close()
	provider := &OpenAIProvider{BaseURL: server.URL + "/v1", APIKey: "secret", Client: server.Client()}
	response, err := provider.Complete(context.Background(), CompletionRequest{Model: "test-model", Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Choices[0].Message.Content != "ok" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestOpenAIProviderErrorKeepsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"slow down"}}`))
	}))
	defer server.Close()
	provider := &OpenAIProvider{BaseURL: server.URL, Client: server.Client()}
	_, err := provider.Complete(context.Background(), CompletionRequest{Model: "test"})
	var providerError *ProviderError
	if !errors.As(err, &providerError) {
		t.Fatalf("got %T %v, want ProviderError", err, err)
	}
	if providerError.StatusCode != http.StatusTooManyRequests || providerError.Body != `{"error":{"message":"slow down"}}` {
		t.Fatalf("provider error changed: %+v", providerError)
	}
}
