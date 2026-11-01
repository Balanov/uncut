package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionRoundTripLargeMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	session, err := OpenSession(path)
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("complete history — без потерь\n", 20000)
	want := []Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: large},
		{Role: "assistant", Content: "working", ToolCalls: []ToolCall{{ID: "call-1", Type: "function", Function: FunctionCall{Name: "read_file", Arguments: `{"path":"large.txt"}`}}}},
		{Role: "tool", ToolCallID: "call-1", Name: "read_file", Content: large},
	}
	for _, message := range want {
		if err := session.Append(message); err != nil {
			t.Fatal(err)
		}
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	resumed, err := OpenSession(path)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	got := resumed.Messages()
	if len(got) != len(want) {
		t.Fatalf("got %d messages, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content || got[i].ToolCallID != want[i].ToolCallID {
			t.Fatalf("message %d changed during round trip", i)
		}
		if len(got[i].ToolCalls) != len(want[i].ToolCalls) {
			t.Fatalf("message %d lost tool calls", i)
		}
	}
}

func TestSessionRefusesPartialRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.jsonl")
	if err := os.WriteFile(path, []byte(`{"v":1,"message":{"role":"user"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSession(path); err == nil {
		t.Fatal("expected invalid session to fail instead of dropping its last record")
	}
}

func TestSessionRefusesInvalidUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-utf8.jsonl")
	record := append([]byte(`{"v":1,"message":{"role":"user","content":"`), 0xff)
	record = append(record, []byte(`"}}`+"\n")...)
	if err := os.WriteFile(path, record, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSession(path); err == nil {
		t.Fatal("expected invalid UTF-8 to fail instead of replacing bytes")
	}
}
