package harness

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

const sessionVersion = 1

type sessionRecord struct {
	Version int     `json:"v"`
	Message Message `json:"message"`
}

// Session is an append-only, crash-resumable JSONL message log.
type Session struct {
	file     *os.File
	messages []Message
}

func OpenSession(path string) (*Session, error) {
	if path == "" {
		return nil, errors.New("session path is empty")
	}
	if parent := filepath.Dir(path); parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return nil, fmt.Errorf("create session directory: %w", err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	s := &Session{file: file}
	reader := bufio.NewReader(file)
	lineNumber := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineNumber++
			if !utf8.Valid(line) {
				file.Close()
				return nil, fmt.Errorf("read session record %d: invalid UTF-8", lineNumber)
			}
			var record sessionRecord
			if err := json.Unmarshal(line, &record); err != nil {
				file.Close()
				return nil, fmt.Errorf("read session record %d (refusing to ignore a partial or invalid record): %w", lineNumber, err)
			}
			if record.Version != sessionVersion {
				file.Close()
				return nil, fmt.Errorf("unsupported session version %d", record.Version)
			}
			s.messages = append(s.messages, record.Message)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			file.Close()
			return nil, fmt.Errorf("read session: %w", readErr)
		}
	}
	return s, nil
}

func (s *Session) Append(message Message) error {
	record, err := json.Marshal(sessionRecord{Version: sessionVersion, Message: message})
	if err != nil {
		return fmt.Errorf("encode session record: %w", err)
	}
	record = append(record, '\n')
	if _, err := s.file.Write(record); err != nil {
		return fmt.Errorf("append session: %w", err)
	}
	if err := s.file.Sync(); err != nil {
		return fmt.Errorf("sync session: %w", err)
	}
	s.messages = append(s.messages, message)
	return nil
}

func (s *Session) Messages() []Message {
	messages := make([]Message, len(s.messages))
	for i, message := range s.messages {
		messages[i] = cloneMessage(message)
	}
	return messages
}

func (s *Session) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	return s.file.Close()
}

func cloneMessage(message Message) Message {
	message.ToolCalls = append([]ToolCall(nil), message.ToolCalls...)
	return message
}
