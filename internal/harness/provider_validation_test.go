package harness

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAIProviderRejectsInvalidResponsesWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"invalid UTF-8", "{\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"\xff\"}}]}", "not valid UTF-8"},
		{"invalid JSON", `{"choices":[`, "decode provider response"},
		{"empty choices", `{"choices":[]}`, "provider returned no choices"},
		{"missing choices", `{}`, "provider returned no choices"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client := server.Client()
			client.Timeout = 5 * time.Second
			provider := &OpenAIProvider{BaseURL: server.URL + "/v1", Client: client}
			_, err := provider.Complete(t.Context(), CompletionRequest{Model: "fixture-model"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Complete error = %v, want %q", err, tc.want)
			}
			if tc.name == "invalid JSON" {
				var syntaxError *json.SyntaxError
				if !errors.As(err, &syntaxError) {
					t.Fatalf("error does not preserve JSON syntax cause: %v", err)
				}
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("requests = %d, want exactly one (no retry)", got)
			}
		})
	}
}

func TestOpenAIProviderAuthenticationErrorsPreserveStatusAndBody(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			const body = "<html>Access denied by fixture gateway</html>\n"
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			client := server.Client()
			client.Timeout = 5 * time.Second
			provider := &OpenAIProvider{BaseURL: server.URL + "/v1", APIKey: "fixture-key", Client: client}
			_, err := provider.Complete(t.Context(), CompletionRequest{Model: "fixture-model"})
			var providerError *ProviderError
			if !errors.As(err, &providerError) {
				t.Fatalf("Complete error = %T %v, want ProviderError", err, err)
			}
			if providerError.StatusCode != status || providerError.Body != body {
				t.Fatalf("provider failure changed: %+v", providerError)
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("requests = %d, want exactly one (no retry)", got)
			}
		})
	}
}

// Observe the real HTTP response body, so cancellation happens after a partial
// body was read, rather than racing with receipt of the response headers.
type observedProviderBody struct {
	io.ReadCloser
	readStarted chan struct{}
	once        sync.Once
	closed      atomic.Bool
}

func (b *observedProviderBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.once.Do(func() { close(b.readStarted) })
	}
	return n, err
}

func (b *observedProviderBody) Close() error {
	b.closed.Store(true)
	return b.ReadCloser.Close()
}

type providerRoundTripFunc func(*http.Request) (*http.Response, error)

func (f providerRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestOpenAIProviderCancellationDuringBodyRead(t *testing.T) {
	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[`)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	client := server.Client()
	client.Timeout = 5 * time.Second
	transport := client.Transport
	readStarted := make(chan struct{})
	observed := make(chan *observedProviderBody, 1)
	client.Transport = providerRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		response, err := transport.RoundTrip(r)
		if err != nil {
			return response, err
		}
		body := &observedProviderBody{ReadCloser: response.Body, readStarted: readStarted}
		response.Body = body
		observed <- body
		return response, nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	provider := &OpenAIProvider{BaseURL: server.URL + "/v1", Client: client}
	result := make(chan error, 1)
	go func() {
		_, err := provider.Complete(ctx, CompletionRequest{Model: "fixture-model"})
		result <- err
	}()
	select {
	case <-readStarted:
	case <-ctx.Done():
		t.Fatal("provider did not read the partial response body")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "read provider response") {
			t.Fatalf("Complete error = %v, want canceled response-body read", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not return after cancellation")
	}
	if !((<-observed).closed.Load()) {
		t.Error("provider did not close the response body")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want exactly one (no retry)", got)
	}
}
