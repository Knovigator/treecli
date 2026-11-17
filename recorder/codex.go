package recorder

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// codexInjectedPrefixes mark user-role items Codex injects on the model's
// behalf (environment context, plugin lists, permissions), not human prompts.
var codexInjectedPrefixes = []string{
	"<environment_context>",
	"<recommended_plugins>",
	"<user_instructions>",
	"<permissions instructions>",
	"<app-context>",
	"<collaboration_mode>",
	"<skills_instructions>",
	"<plugins_instructions>",
	"<multi_agent_mode>",
	"<turn_aborted>",
	"<system_notification>",
}

type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexSessionMeta struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
}

type codexTurnContext struct {
	Model string `json:"model"`
}

type codexResponseItem struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	CallID    string          `json:"call_id"`
	Output    json.RawMessage `json:"output"`
}

type codexContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ParseCodexRollout converts a Codex CLI rollout
// (~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl) into a Session.
//
// Response items are the source of truth: user/assistant messages,
// function_call / custom_tool_call (tool calls) and their outputs. Reasoning
// items and Codex's event stream are ignored; developer-role messages and the
// context blocks Codex injects as user messages are dropped.
func ParseCodexRollout(r io.Reader) (*Session, error) {
	session := &Session{Agent: AgentCodex}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	lineNumber := 0
	lastUserID := ""
	for scanner.Scan() {
		lineNumber++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var line codexLine
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			continue
		}
		timestamp, _ := time.Parse(time.RFC3339Nano, line.Timestamp)
		switch line.Type {
		case "session_meta":
			var meta codexSessionMeta
			if err := json.Unmarshal(line.Payload, &meta); err == nil {
				if meta.ID != "" {
					session.SessionID = meta.ID
				} else if meta.SessionID != "" {
					session.SessionID = meta.SessionID
				}
				session.CWD = meta.CWD
				if started, err := time.Parse(time.RFC3339Nano, meta.Timestamp); err == nil {
					session.StartedAt = started
				}
			}
		case "turn_context":
			var context codexTurnContext
			if err := json.Unmarshal(line.Payload, &context); err == nil && context.Model != "" {
				session.Model = context.Model
			}
		case "response_item":
			var item codexResponseItem
			if err := json.Unmarshal(line.Payload, &item); err != nil {
				continue
			}
			if session.StartedAt.IsZero() && !timestamp.IsZero() {
				session.StartedAt = timestamp
			}
			switch item.Type {
			case "message":
				switch item.Role {
				case "user":
					text := flattenCodexContent(item.Content)
					if isCodexInjected(text) {
						continue
					}
					id := item.ID
					if id == "" {
						id = fmt.Sprintf("line-%d", lineNumber)
					}
					session.Messages = append(session.Messages, Message{
						ID: id, Timestamp: timestamp, Role: RoleUser, Text: strings.TrimSpace(text),
					})
					lastUserID = id
				case "assistant":
					text := strings.TrimSpace(flattenCodexContent(item.Content))
					if text == "" {
						continue
					}
					id := item.ID
					if id == "" {
						id = fmt.Sprintf("line-%d", lineNumber)
					}
					session.Messages = append(session.Messages, Message{
						ID: id, ParentID: lastUserID, Timestamp: timestamp, Role: RoleAssistant, Text: text,
					})
				}
			case "function_call", "custom_tool_call":
				arguments := item.Arguments
				if item.Type == "custom_tool_call" {
					encoded, _ := json.Marshal(map[string]string{"input": Truncate(item.Input, MaxToolArgumentChars)})
					arguments = string(encoded)
				}
				var rawArguments json.RawMessage
				if json.Valid([]byte(arguments)) {
					rawArguments = compactArguments(json.RawMessage(arguments))
				} else if arguments != "" {
					encoded, _ := json.Marshal(Truncate(arguments, MaxToolArgumentChars))
					rawArguments = encoded
				}
				id := item.ID
				if id == "" {
					id = fmt.Sprintf("line-%d", lineNumber)
				}
				session.Messages = append(session.Messages, Message{
					ID: id, ParentID: lastUserID, Timestamp: timestamp, Role: RoleAssistant,
					ToolCalls: []ToolCall{{ID: item.CallID, Name: item.Name, Arguments: rawArguments}},
				})
			case "function_call_output", "custom_tool_call_output":
				id := item.ID
				if id == "" {
					id = fmt.Sprintf("line-%d", lineNumber)
				}
				session.Messages = append(session.Messages, Message{
					ID: id, Timestamp: timestamp, Role: RoleToolResult, ToolCallID: item.CallID,
					Text: Truncate(flattenCodexContent(item.Output), MaxToolResultChars),
				})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading codex rollout (line %d): %w", lineNumber, err)
	}
	mergeToolCallsIntoPreviousAssistant(session)
	return session, nil
}

func isCodexInjected(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	for _, prefix := range codexInjectedPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func flattenCodexContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []codexContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}
