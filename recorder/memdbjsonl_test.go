package recorder

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleSession() *Session {
	base := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	return &Session{
		Agent: AgentClaudeCode, SessionID: "sess-1", CWD: "/Users/me/src/app", Model: "claude-fable-5-1", StartedAt: base,
		Messages: []Message{
			{ID: "u1", Timestamp: base.Add(time.Second), Role: RoleUser, Text: "Decision: record sessions into memdb."},
			{ID: "a1", ParentID: "u1", Timestamp: base.Add(2 * time.Second), Role: RoleAssistant, Text: "Recording.", ToolCalls: []ToolCall{{ID: "t1", Name: "Bash", Arguments: json.RawMessage(`{"command":"ls"}`)}}},
			{ID: "r1", ParentID: "a1", Timestamp: base.Add(3 * time.Second), Role: RoleToolResult, ToolCallID: "t1", Text: "a.txt"},
		},
	}
}

func TestWriteMemdbJSONLMatchesIngestContract(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteMemdbJSONL(&buffer, sampleSession()); err != nil {
		t.Fatalf("write: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d:\n%s", len(lines), buffer.String())
	}
	var header map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("header: %v", err)
	}
	if header["type"] != "session" || header["id"] != "sess-1" || header["sessionKey"] != "agent:treecli:claude-code:group:-Users-me-src-app:thread:sess-1" || header["timestamp"] != "2026-09-09T12:00:00.000Z" {
		t.Fatalf("unexpected session record: %v", header)
	}
	var assistant struct {
		Type     string `json:"type"`
		ParentID string `json:"parentId"`
		Message  struct {
			Role    string                   `json:"role"`
			Content []map[string]interface{} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &assistant); err != nil {
		t.Fatalf("assistant: %v", err)
	}
	if assistant.Type != "message" || assistant.ParentID != "u1" || assistant.Message.Role != "assistant" {
		t.Fatalf("unexpected assistant envelope: %+v", assistant)
	}
	if len(assistant.Message.Content) != 2 || assistant.Message.Content[0]["type"] != "text" || assistant.Message.Content[1]["type"] != "toolCall" || assistant.Message.Content[1]["name"] != "Bash" {
		t.Fatalf("assistant content must be text + toolCall blocks: %v", assistant.Message.Content)
	}
	var toolResult map[string]interface{}
	_ = json.Unmarshal([]byte(lines[3]), &toolResult)
	message := toolResult["message"].(map[string]interface{})
	if message["role"] != "toolResult" || message["content"] != "a.txt" {
		t.Fatalf("tool results must be toolResult messages with string content: %v", toolResult)
	}
}

func TestRecordingRoundTripAndUnchangedWritesKeepMtime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recordings", "claude-code", "sess-1.jsonl")
	session := sampleSession()
	if err := WriteRecordingFile(path, session); err != nil {
		t.Fatalf("write: %v", err)
	}
	before, _ := os.Stat(path)
	if before.Mode().Perm() != 0o600 {
		t.Fatalf("recordings must be private, got %v", before.Mode().Perm())
	}
	time.Sleep(20 * time.Millisecond)
	if err := WriteRecordingFile(path, session); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("rewriting identical content must leave the file untouched so memdb skips it")
	}
	file, _ := os.Open(path)
	defer file.Close()
	loaded, err := ReadRecording(file)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if loaded.SessionID != "sess-1" || loaded.Agent != AgentClaudeCode || loaded.CWD != "/Users/me/src/app" || len(loaded.Messages) != 3 {
		t.Fatalf("round trip lost data: %+v", loaded)
	}
	if loaded.Messages[1].ToolCalls[0].Name != "Bash" || string(loaded.Messages[1].ToolCalls[0].Arguments) != `{"command":"ls"}` {
		t.Fatalf("tool calls did not survive the round trip: %+v", loaded.Messages[1])
	}
	if loaded.Messages[2].Role != RoleToolResult || loaded.Messages[2].ToolCallID != "t1" {
		t.Fatalf("tool result did not survive: %+v", loaded.Messages[2])
	}
	if !loaded.StartedAt.Equal(session.StartedAt) {
		t.Fatalf("started_at changed: %s vs %s", loaded.StartedAt, session.StartedAt)
	}
}
