package recorder

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// memdb ingest contract (see memdb-rust ingest_jsonl): one JSON object per
// line. A leading {"type":"session"} record carries id, cwd and sessionKey;
// {"type":"message"} records carry {message:{role,content}} where content is
// either a string or an array of {type:"text"} / {type:"toolCall"} blocks.
// Tool results are messages with role "toolResult".

type memdbSessionRecord struct {
	Type       string `json:"type"`
	Timestamp  string `json:"timestamp"`
	ID         string `json:"id"`
	CWD        string `json:"cwd"`
	SessionKey string `json:"sessionKey"`
	ChatApp    string `json:"chat_app,omitempty"`
	Model      string `json:"model,omitempty"`
	Recorder   string `json:"recorder,omitempty"`
}

type memdbMessageRecord struct {
	Type      string       `json:"type"`
	Timestamp string       `json:"timestamp"`
	ID        string       `json:"id"`
	ParentID  string       `json:"parentId,omitempty"`
	ToolName  string       `json:"toolName,omitempty"`
	CallID    string       `json:"toolCallId,omitempty"`
	Message   memdbMessage `json:"message"`
}

type memdbMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type memdbTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type memdbToolCallBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// WriteMemdbJSONL renders a session in memdb's ingest format.
func WriteMemdbJSONL(w io.Writer, session *Session) error {
	writer := bufio.NewWriter(w)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)

	startedAt := session.StartedAt
	if startedAt.IsZero() && len(session.Messages) > 0 {
		startedAt = session.Messages[0].Timestamp
	}
	if err := encoder.Encode(memdbSessionRecord{
		Type:       "session",
		Timestamp:  formatTimestamp(startedAt),
		ID:         session.SessionID,
		CWD:        session.CWD,
		SessionKey: session.Key(),
		ChatApp:    string(session.Agent),
		Model:      session.Model,
		Recorder:   "treecli",
	}); err != nil {
		return err
	}

	for _, message := range session.Messages {
		record := memdbMessageRecord{
			Type:      "message",
			Timestamp: formatTimestamp(message.Timestamp),
			ID:        message.ID,
			ParentID:  message.ParentID,
			CallID:    message.ToolCallID,
			ToolName:  message.ToolName,
			Message:   memdbMessage{Role: message.Role},
		}
		switch message.Role {
		case RoleAssistant:
			blocks := make([]interface{}, 0, 1+len(message.ToolCalls))
			if message.Text != "" {
				blocks = append(blocks, memdbTextBlock{Type: "text", Text: message.Text})
			}
			for _, call := range message.ToolCalls {
				blocks = append(blocks, memdbToolCallBlock{Type: "toolCall", ID: call.ID, Name: call.Name, Arguments: call.Arguments})
			}
			record.Message.Content = blocks
		default:
			record.Message.Content = message.Text
		}
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return writer.Flush()
}

// WriteRecordingFile atomically writes the session's memdb JSONL to path.
// memdb's ingest keys on the file path plus mtime/size, so rewriting the
// whole file on every turn is the idempotent path: unchanged files are
// skipped, changed files replace their previous rows.
func WriteRecordingFile(path string, session *Session) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var rendered bytes.Buffer
	if err := WriteMemdbJSONL(&rendered, session); err != nil {
		return err
	}
	// Leave identical files untouched so memdb's mtime/size check skips them.
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, rendered.Bytes()) {
		return nil
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".recording-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() { _ = os.Remove(tempPath) }
	if _, err := temp.Write(rendered.Bytes()); err != nil {
		_ = temp.Close()
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return fmt.Errorf("replacing recording %s: %w", path, err)
	}
	return nil
}

func formatTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05.000Z")
}
