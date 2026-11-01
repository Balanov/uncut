package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Balanov/uncut/internal/harness"
)

func emptyStdin(t *testing.T) *os.File {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close() })
	return reader
}

func TestRunPlainOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(harness.CompletionResponse{Choices: []harness.Choice{{Message: harness.Message{Role: "assistant", Content: "ok"}, FinishReason: "stop"}}})
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := run([]string{"-model", "test", "-base-url", server.URL, "-cwd", t.TempDir(), "-no-agents", "hello"}, emptyStdin(t), &stdout, &stderr)
	if code != 0 || stdout.String() != "ok\n" || stderr.String() != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunJSONEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(harness.CompletionResponse{Choices: []harness.Choice{{Message: harness.Message{Role: "assistant", Content: "ok"}, FinishReason: "stop"}}})
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := run([]string{"-model", "test", "-base-url", server.URL, "-cwd", t.TempDir(), "-no-agents", "-json", "hello"}, emptyStdin(t), &stdout, &stderr)
	if code != 0 || stderr.String() != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	decoder := json.NewDecoder(&stdout)
	var events []harness.Event
	for decoder.More() {
		var event harness.Event
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if len(events) != 4 || events[0].Type != "user" || events[1].Type != "assistant" || events[2].Type != "assistant_meta" || events[3].Type != "done" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestRunTruncatedExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(harness.CompletionResponse{Choices: []harness.Choice{{Message: harness.Message{Role: "assistant", Content: "partial"}, FinishReason: "length"}}})
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := run([]string{"-model", "test", "-base-url", server.URL, "-cwd", t.TempDir(), "-no-agents", "hello"}, emptyStdin(t), &stdout, &stderr)
	if code != 3 || !strings.Contains(stderr.String(), "truncated") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestHelpExitsSuccessfully(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-help"}, emptyStdin(t), &stdout, &stderr); code != 0 {
		t.Fatalf("help exit code %d", code)
	}
}
