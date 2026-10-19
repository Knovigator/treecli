// Package recorder turns AI coding-agent sessions (Claude Code, Codex, Grok)
// into a normalized transcript that memdb ingests and Treechat can display.
//
// The normalized model is deliberately small: a Session is an ordered list of
// Messages with user / assistant / toolResult roles. Every host-specific
// transcript format (Claude Code project JSONL, Codex rollouts, hook
// payloads) converts into it, and every sink (memdb JSONL, Treechat posts)
// renders from it.
package recorder

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Agent identifies the host that produced a session. The values double as
// memdb's chat_app and are stable on-disk identifiers, so never rename them.
type Agent string

const (
	AgentClaudeCode Agent = "claude-code"
	AgentCodex      Agent = "codex"
	AgentGrok       Agent = "grok"
)

// KnownAgents lists the agents the recorder understands, in display order.
var KnownAgents = []Agent{AgentClaudeCode, AgentCodex, AgentGrok}

// ParseAgent accepts the canonical names plus the aliases people actually type.
func ParseAgent(value string) (Agent, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "claude", "claude-code", "claudecode", "claude_code", "cc":
		return AgentClaudeCode, nil
	case "codex", "openai-codex":
		return AgentCodex, nil
	case "grok", "grok-build", "grok-cli", "xai":
		return AgentGrok, nil
	}
	return "", fmt.Errorf("unknown agent %q (use claude, codex, or grok)", value)
}

// Message roles. They match memdb's ingest contract, where "toolResult" is a
// message role rather than a separate record type.
const (
	RoleUser       = "user"
	RoleAssistant  = "assistant"
	RoleToolResult = "toolResult"
)

// ToolCall is a tool invocation made by the assistant.
type ToolCall struct {
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Message is one normalized transcript entry.
type Message struct {
	ID         string     `json:"id"`
	ParentID   string     `json:"parent_id,omitempty"`
	Timestamp  time.Time  `json:"timestamp"`
	Role       string     `json:"role"`
	Text       string     `json:"text,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Session is a normalized agent session.
type Session struct {
	Agent      Agent     `json:"agent"`
	SessionID  string    `json:"session_id"`
	CWD        string    `json:"cwd"`
	Model      string    `json:"model,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	Messages   []Message `json:"messages"`
	SourcePath string    `json:"source_path,omitempty"`
}

// ProjectKey mirrors Claude Code's project-directory naming: every run of
// non-alphanumeric characters in the working directory becomes a single '-'.
// It is memdb's chat_id for agent sessions and must not contain ':'.
func ProjectKey(cwd string) string {
	cleaned := filepath.Clean(strings.TrimSpace(cwd))
	if cleaned == "." || cleaned == "" {
		return "-"
	}
	var builder strings.Builder
	for _, r := range cleaned {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('-')
		}
	}
	return builder.String()
}

// SessionKey builds the memdb session key for a session. memdb's
// `coalesce_scope_from_session_key` grammar turns it into chat_app = agent,
// chat_id = project key, thread_id = session id, which is exactly the scope
// `recall --session-key` resolves.
func SessionKey(agent Agent, projectKey, sessionID string) string {
	return fmt.Sprintf("agent:treecli:%s:group:%s:thread:%s", agent, projectKey, sessionID)
}

// ProjectSessionKey scopes to a whole project (no thread), for cross-session
// recall inside one repository.
func ProjectSessionKey(agent Agent, projectKey string) string {
	return fmt.Sprintf("agent:treecli:%s:group:%s", agent, projectKey)
}

// Key returns the session's memdb session key.
func (s *Session) Key() string {
	return SessionKey(s.Agent, ProjectKey(s.CWD), s.SessionID)
}

// Turn groups one user prompt with everything the assistant did in response.
type Turn struct {
	Index        int
	UserPrompt   string
	StartedAt    time.Time
	EndedAt      time.Time
	Assistant    string         // concatenated assistant text of the turn
	FinalReply   string         // the last assistant text block, what the user saw last
	ToolCounts   map[string]int // tool name -> calls
	ToolCallsSum int
}

// Turns splits the messages at user prompts. Tool results never start a turn.
func (s *Session) Turns() []Turn {
	turns := []Turn{}
	var current *Turn
	for _, message := range s.Messages {
		switch message.Role {
		case RoleUser:
			turns = append(turns, Turn{
				Index:      len(turns),
				UserPrompt: message.Text,
				StartedAt:  message.Timestamp,
				EndedAt:    message.Timestamp,
				ToolCounts: map[string]int{},
			})
			current = &turns[len(turns)-1]
		case RoleAssistant:
			if current == nil {
				turns = append(turns, Turn{Index: len(turns), StartedAt: message.Timestamp, ToolCounts: map[string]int{}})
				current = &turns[len(turns)-1]
			}
			if text := strings.TrimSpace(message.Text); text != "" {
				if current.Assistant != "" {
					current.Assistant += "\n\n"
				}
				current.Assistant += text
				current.FinalReply = text
			}
			for _, call := range message.ToolCalls {
				current.ToolCounts[call.Name]++
				current.ToolCallsSum++
			}
			if message.Timestamp.After(current.EndedAt) {
				current.EndedAt = message.Timestamp
			}
		case RoleToolResult:
			if current != nil && message.Timestamp.After(current.EndedAt) {
				current.EndedAt = message.Timestamp
			}
		}
	}
	return turns
}

// ToolSummary renders "12 tool calls: Bash×7, Edit×3, Read×2" (top names first).
func (t Turn) ToolSummary() string {
	if t.ToolCallsSum == 0 {
		return ""
	}
	type pair struct {
		name  string
		count int
	}
	pairs := make([]pair, 0, len(t.ToolCounts))
	for name, count := range t.ToolCounts {
		pairs = append(pairs, pair{name, count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].name < pairs[j].name
	})
	parts := make([]string, 0, len(pairs))
	for index, p := range pairs {
		if index == 6 {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, fmt.Sprintf("%s×%d", p.name, p.count))
	}
	noun := "tool calls"
	if t.ToolCallsSum == 1 {
		noun = "tool call"
	}
	return fmt.Sprintf("%d %s: %s", t.ToolCallsSum, noun, strings.Join(parts, ", "))
}

// DeterministicID derives a stable UUID (version 5 layout) from a namespace
// and name, so retried writes reuse the same Treechat ids and the backend
// deduplicates them instead of creating twins.
func DeterministicID(namespace string, parts ...string) string {
	hasher := sha1.New()
	hasher.Write([]byte("treecli-recorder:" + namespace))
	for _, part := range parts {
		hasher.Write([]byte{0})
		hasher.Write([]byte(part))
	}
	sum := hasher.Sum(nil)
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// Truncate cuts text to at most limit runes, appending a marker when it cut.
func Truncate(text string, limit int) string {
	if limit <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + fmt.Sprintf("… [truncated %d chars]", len(runes)-limit)
}

// FirstLine returns the first non-empty line of text, trimmed to limit runes.
func FirstLine(text string, limit int) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > limit {
			return string(runes[:limit]) + "…"
		}
		return line
	}
	return ""
}
