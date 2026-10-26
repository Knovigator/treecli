package recorder

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// MaxToolResultChars bounds tool output kept in recordings. Tool results can
// be hundreds of kilobytes; memdb needs enough to search, not the whole dump.
const MaxToolResultChars = 6000

// MaxToolArgumentChars bounds serialized tool arguments kept in recordings.
const MaxToolArgumentChars = 3000

var systemReminderRE = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)

// claudeInjectedPrefixes mark user-role records that Claude Code synthesizes
// (hook output, task notifications, slash-command plumbing), not prompts the
// person typed.
var claudeInjectedPrefixes = []string{
	"<task-notification>",
	"<command-name>",
	"<command-message>",
	"<local-command-stdout>",
	"<local-command-caveat>",
	"<user-prompt-submit-hook>",
	"<system-reminder>",
	"<ci-monitor-event>",
	"[Request interrupted",
}

type claudeRecord struct {
	Type        string                 `json:"type"`
	UUID        string                 `json:"uuid"`
	ParentUUID  string                 `json:"parentUuid"`
	Timestamp   string                 `json:"timestamp"`
	IsSidechain bool                   `json:"isSidechain"`
	IsMeta      bool                   `json:"isMeta"`
	CWD         string                 `json:"cwd"`
	SessionID   string                 `json:"sessionId"`
	Origin      *struct{ Kind string } `json:"origin"`
	Message     json.RawMessage        `json:"message"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// ParseClaudeTranscript converts a Claude Code project transcript
// (~/.claude/projects/<project>/<session>.jsonl) into a Session.
//
// Only the main conversation is kept: sidechain (subagent) records, meta
// records and Claude Code's own bookkeeping lines are skipped. Thinking blocks
// are dropped because they are not part of the durable conversation.
func ParseClaudeTranscript(r io.Reader) (*Session, error) {
	session := &Session{Agent: AgentClaudeCode}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record claudeRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue // Claude writes non-message bookkeeping lines; ignore anything odd.
		}
		if record.Type != "user" && record.Type != "assistant" {
			continue
		}
		if record.IsSidechain || record.IsMeta {
			continue
		}
		if session.SessionID == "" && record.SessionID != "" {
			session.SessionID = record.SessionID
		}
		if session.CWD == "" && record.CWD != "" {
			session.CWD = record.CWD
		}
		timestamp, _ := time.Parse(time.RFC3339Nano, record.Timestamp)
		if session.StartedAt.IsZero() && !timestamp.IsZero() {
			session.StartedAt = timestamp
		}
		var message claudeMessage
		if err := json.Unmarshal(record.Message, &message); err != nil {
			continue
		}
		if message.Model != "" && session.Model == "" {
			session.Model = message.Model
		}
		switch record.Type {
		case "user":
			session.Messages = append(session.Messages, claudeUserMessages(record, message, timestamp)...)
		case "assistant":
			if built, ok := claudeAssistantMessage(record, message, timestamp); ok {
				session.Messages = append(session.Messages, built)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading claude transcript (line %d): %w", lineNumber, err)
	}
	mergeToolCallsIntoPreviousAssistant(session)
	return session, nil
}

func claudeUserMessages(record claudeRecord, message claudeMessage, timestamp time.Time) []Message {
	var text string
	var blocks []claudeBlock
	if err := json.Unmarshal(message.Content, &text); err != nil {
		if err := json.Unmarshal(message.Content, &blocks); err != nil {
			return nil
		}
	}
	messages := []Message{}
	var promptParts []string
	if text != "" {
		promptParts = append(promptParts, text)
	}
	for index, block := range blocks {
		switch block.Type {
		case "text":
			promptParts = append(promptParts, block.Text)
		case "tool_result":
			messages = append(messages, Message{
				ID:         fmt.Sprintf("%s#%d", record.UUID, index),
				ParentID:   record.ParentUUID,
				Timestamp:  timestamp,
				Role:       RoleToolResult,
				ToolCallID: block.ToolUseID,
				Text:       Truncate(flattenClaudeContent(block.Content), MaxToolResultChars),
			})
		}
	}
	prompt := cleanClaudePrompt(strings.Join(promptParts, "\n"))
	if prompt != "" {
		messages = append([]Message{{
			ID:        record.UUID,
			ParentID:  record.ParentUUID,
			Timestamp: timestamp,
			Role:      RoleUser,
			Text:      prompt,
		}}, messages...)
	}
	return messages
}

func cleanClaudePrompt(text string) string {
	text = systemReminderRE.ReplaceAllString(text, "")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, prefix := range claudeInjectedPrefixes {
		if strings.HasPrefix(text, prefix) {
			return ""
		}
	}
	return text
}

func claudeAssistantMessage(record claudeRecord, message claudeMessage, timestamp time.Time) (Message, bool) {
	var blocks []claudeBlock
	if err := json.Unmarshal(message.Content, &blocks); err != nil {
		var text string
		if err := json.Unmarshal(message.Content, &text); err != nil || strings.TrimSpace(text) == "" {
			return Message{}, false
		}
		return Message{ID: record.UUID, ParentID: record.ParentUUID, Timestamp: timestamp, Role: RoleAssistant, Text: text}, true
	}
	built := Message{ID: record.UUID, ParentID: record.ParentUUID, Timestamp: timestamp, Role: RoleAssistant}
	var texts []string
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				texts = append(texts, block.Text)
			}
		case "tool_use":
			built.ToolCalls = append(built.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: compactArguments(block.Input),
			})
		}
	}
	built.Text = strings.Join(texts, "\n")
	if built.Text == "" && len(built.ToolCalls) == 0 {
		return Message{}, false
	}
	return built, true
}

func flattenClaudeContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []claudeBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if block.Text != "" {
				parts = append(parts, block.Text)
			} else if block.Type != "" && block.Type != "text" {
				parts = append(parts, "["+block.Type+"]")
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

// compactArguments keeps tool arguments as JSON but bounded in size, so a
// 200 KB Write payload does not end up verbatim in the memory database.
func compactArguments(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	if len(raw) <= MaxToolArgumentChars {
		return raw
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err == nil {
		trimmed := map[string]interface{}{}
		for key, value := range generic {
			var str string
			if err := json.Unmarshal(value, &str); err == nil {
				trimmed[key] = Truncate(str, MaxToolArgumentChars/2)
				continue
			}
			if len(value) > MaxToolArgumentChars/2 {
				trimmed[key] = Truncate(string(value), MaxToolArgumentChars/2)
				continue
			}
			trimmed[key] = json.RawMessage(value)
		}
		if encoded, err := json.Marshal(trimmed); err == nil {
			return encoded
		}
	}
	encoded, _ := json.Marshal(Truncate(string(raw), MaxToolArgumentChars))
	return encoded
}

// mergeToolCallsIntoPreviousAssistant folds text-less assistant messages that
// only carry tool calls into the preceding assistant message of the same turn,
// so recall does not surface empty assistant rows.
func mergeToolCallsIntoPreviousAssistant(session *Session) {
	merged := make([]Message, 0, len(session.Messages))
	for _, message := range session.Messages {
		if message.Role == RoleAssistant && strings.TrimSpace(message.Text) == "" && len(message.ToolCalls) > 0 {
			for index := len(merged) - 1; index >= 0; index-- {
				if merged[index].Role == RoleUser {
					break
				}
				if merged[index].Role == RoleAssistant {
					merged[index].ToolCalls = append(merged[index].ToolCalls, message.ToolCalls...)
					if message.Timestamp.After(merged[index].Timestamp) {
						merged[index].Timestamp = message.Timestamp
					}
					message = Message{}
					break
				}
			}
			if message.ID == "" {
				continue
			}
		}
		merged = append(merged, message)
	}
	session.Messages = merged
}
