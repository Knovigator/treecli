package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Hook event names, normalized to Claude Code's spelling. Codex and Grok Build
// use the same dialect (Grok also sends a snake_case hookEventName).
const (
	EventSessionStart     = "SessionStart"
	EventSessionEnd       = "SessionEnd"
	EventUserPromptSubmit = "UserPromptSubmit"
	EventPostToolUse      = "PostToolUse"
	EventStop             = "Stop"
)

// HookEvent is a normalized hook payload.
type HookEvent struct {
	Name                 string
	SessionID            string
	TranscriptPath       string
	CWD                  string
	Prompt               string
	Source               string
	Reason               string
	ToolName             string
	ToolInput            json.RawMessage
	ToolResponse         json.RawMessage
	LastAssistantMessage string
	Raw                  map[string]json.RawMessage
}

// ParseHookPayload decodes the JSON a host writes to a hook's stdin. It
// accepts snake_case (Claude Code, Codex, superagent grok-cli) and camelCase
// (Grok Build) field names. fallbackEvent names the event when the payload
// does not.
func ParseHookPayload(data []byte, fallbackEvent string) (HookEvent, error) {
	event := HookEvent{Raw: map[string]json.RawMessage{}}
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &event.Raw); err != nil {
			return event, fmt.Errorf("hook payload is not a JSON object: %w", err)
		}
	}
	str := func(keys ...string) string {
		for _, key := range keys {
			raw, ok := event.Raw[key]
			if !ok {
				continue
			}
			var value string
			if err := json.Unmarshal(raw, &value); err == nil && value != "" {
				return value
			}
		}
		return ""
	}
	rawField := func(keys ...string) json.RawMessage {
		for _, key := range keys {
			if raw, ok := event.Raw[key]; ok && len(raw) > 0 && string(raw) != "null" {
				return raw
			}
		}
		return nil
	}
	event.Name = NormalizeEventName(str("hook_event_name", "hookEventName"))
	if event.Name == "" {
		event.Name = NormalizeEventName(fallbackEvent)
	}
	event.SessionID = str("session_id", "sessionId", "thread-id", "thread_id")
	event.TranscriptPath = str("transcript_path", "transcriptPath")
	event.CWD = str("cwd", "workspaceRoot", "workspace_root")
	event.Prompt = str("prompt", "user_prompt", "userPrompt")
	event.Source = str("source")
	event.Reason = str("reason")
	event.ToolName = str("tool_name", "toolName")
	event.ToolInput = rawField("tool_input", "toolInput")
	event.ToolResponse = rawField("tool_response", "toolResult", "tool_output", "toolOutput")
	event.LastAssistantMessage = str("last_assistant_message", "lastAssistantMessage", "last-assistant-message")
	if event.CWD == "" {
		if cwd, err := os.Getwd(); err == nil {
			event.CWD = cwd
		}
	}
	return event, nil
}

// NormalizeEventName maps "session_start", "sessionStart" and "SessionStart"
// to the canonical PascalCase name.
func NormalizeEventName(value string) string {
	compact := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.TrimSpace(value)))
	switch compact {
	case "sessionstart":
		return EventSessionStart
	case "sessionend":
		return EventSessionEnd
	case "userpromptsubmit":
		return EventUserPromptSubmit
	case "posttooluse":
		return EventPostToolUse
	case "stop", "agentturncomplete", "turnended", "turncomplete":
		return EventStop
	}
	return strings.TrimSpace(value)
}

// HookOptions tunes the handler.
type HookOptions struct {
	// RecallOnStart injects "last time in this project" context at SessionStart.
	RecallOnStart bool
	// RecallOnPrompt injects learned memories relevant to each prompt.
	RecallOnPrompt bool
	// MaxContextChars bounds injected context.
	MaxContextChars int
}

// DefaultHookOptions is what `treecli mcp install` configures.
var DefaultHookOptions = HookOptions{RecallOnStart: true, RecallOnPrompt: true, MaxContextChars: 1800}

// HookHandler turns hook events into recordings and context injections.
type HookHandler struct {
	Agent    Agent
	Recorder *Recorder
	Options  HookOptions
	Stdout   io.Writer
	Now      func() time.Time
}

func (h *HookHandler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// Handle processes one event. It returns an error only for diagnostics; the
// caller must still exit 0 so the agent keeps running.
func (h *HookHandler) Handle(ctx context.Context, event HookEvent) error {
	if event.SessionID == "" {
		return fmt.Errorf("%s payload has no session id", event.Name)
	}
	store := h.Recorder.Store
	switch event.Name {
	case EventSessionStart:
		marker := Marker{Agent: h.Agent, SessionID: event.SessionID, CWD: event.CWD, TranscriptPath: event.TranscriptPath, StartedAt: h.now().UTC()}
		if existing, ok, _ := store.LoadMarker(h.Agent, event.CWD); ok && existing.SessionID == event.SessionID {
			marker.StartedAt = existing.StartedAt
			marker.Turns = existing.Turns
		}
		if err := store.SaveMarker(marker); err != nil {
			return err
		}
		if err := h.ensureRecording(ctx, event); err != nil {
			return err
		}
		if h.Options.RecallOnStart {
			h.emitContext(EventSessionStart, h.startContext(ctx, event))
		}
		return nil
	case EventUserPromptSubmit:
		h.touchMarker(event, false)
		if !h.hasTranscript(event) {
			if err := h.appendMessages(ctx, event, Message{
				ID:        DeterministicID("prompt", string(h.Agent), event.SessionID, fmt.Sprintf("%d", h.now().UnixNano())),
				Timestamp: h.now().UTC(), Role: RoleUser, Text: strings.TrimSpace(event.Prompt),
			}); err != nil {
				return err
			}
		}
		if h.Options.RecallOnPrompt && strings.TrimSpace(event.Prompt) != "" {
			h.emitContext(EventUserPromptSubmit, h.promptContext(ctx, event))
		}
		return nil
	case EventPostToolUse:
		if h.hasTranscript(event) {
			return nil // the transcript carries tool calls; synced at Stop.
		}
		now := h.now().UTC()
		callID := DeterministicID("tool", string(h.Agent), event.SessionID, fmt.Sprintf("%d", now.UnixNano()))
		return h.appendMessages(ctx, event,
			Message{ID: callID, Timestamp: now, Role: RoleAssistant, ToolCalls: []ToolCall{{ID: callID, Name: event.ToolName, Arguments: compactArguments(event.ToolInput)}}},
			Message{ID: callID + "-result", ParentID: callID, Timestamp: now, Role: RoleToolResult, ToolCallID: callID, ToolName: event.ToolName, Text: Truncate(flattenClaudeContent(event.ToolResponse), MaxToolResultChars)},
		)
	case EventStop, EventSessionEnd:
		ended := event.Name == EventSessionEnd
		h.touchMarker(event, ended)
		if h.hasTranscript(event) {
			result, err := h.Recorder.SyncTranscript(ctx, h.Agent, event.TranscriptPath, event.SessionID, event.CWD, ended)
			if err != nil {
				return err
			}
			h.recordTurns(event, result.Session)
			return result.TreechatErr
		}
		var extra []Message
		if text := strings.TrimSpace(event.LastAssistantMessage); text != "" {
			extra = append(extra, Message{ID: DeterministicID("reply", string(h.Agent), event.SessionID, text), Timestamp: h.now().UTC(), Role: RoleAssistant, Text: text})
		}
		session, err := h.loadOrCreate(event)
		if err != nil {
			return err
		}
		session.Messages = append(session.Messages, dedupeNew(session.Messages, extra)...)
		result, err := h.Recorder.SyncSession(ctx, session, ended)
		if err != nil {
			return err
		}
		h.recordTurns(event, session)
		return result.TreechatErr
	}
	return nil
}

func (h *HookHandler) hasTranscript(event HookEvent) bool {
	if event.TranscriptPath == "" {
		return false
	}
	if h.Agent != AgentClaudeCode && h.Agent != AgentCodex {
		return false
	}
	info, err := os.Stat(event.TranscriptPath)
	return err == nil && !info.IsDir()
}

func (h *HookHandler) ensureRecording(ctx context.Context, event HookEvent) error {
	if h.hasTranscript(event) {
		return nil
	}
	if _, ok, err := h.Recorder.LoadRecording(h.Agent, event.SessionID); err != nil || ok {
		return err
	}
	session := &Session{Agent: h.Agent, SessionID: event.SessionID, CWD: event.CWD, StartedAt: h.now().UTC()}
	return WriteRecordingFile(h.Recorder.Store.RecordingPath(h.Agent, event.SessionID), session)
}

func (h *HookHandler) loadOrCreate(event HookEvent) (*Session, error) {
	session, ok, err := h.Recorder.LoadRecording(h.Agent, event.SessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		session = &Session{Agent: h.Agent, SessionID: event.SessionID, CWD: event.CWD, StartedAt: h.now().UTC()}
	}
	if session.CWD == "" {
		session.CWD = event.CWD
	}
	return session, nil
}

func (h *HookHandler) appendMessages(ctx context.Context, event HookEvent, messages ...Message) error {
	session, err := h.loadOrCreate(event)
	if err != nil {
		return err
	}
	session.Messages = append(session.Messages, dedupeNew(session.Messages, messages)...)
	_, err = h.Recorder.SyncSession(ctx, session, false)
	return err
}

func dedupeNew(existing []Message, candidates []Message) []Message {
	seen := map[string]bool{}
	for _, message := range existing {
		seen[message.ID] = true
	}
	fresh := []Message{}
	for _, candidate := range candidates {
		if candidate.ID == "" || seen[candidate.ID] {
			continue
		}
		if candidate.Role == RoleUser && candidate.Text == "" {
			continue
		}
		fresh = append(fresh, candidate)
	}
	return fresh
}

func (h *HookHandler) touchMarker(event HookEvent, ended bool) {
	store := h.Recorder.Store
	marker, ok, _ := store.LoadMarker(h.Agent, event.CWD)
	if !ok || marker.SessionID != event.SessionID {
		marker = Marker{Agent: h.Agent, SessionID: event.SessionID, CWD: event.CWD, StartedAt: h.now().UTC()}
	}
	if event.TranscriptPath != "" {
		marker.TranscriptPath = event.TranscriptPath
	}
	marker.Ended = ended
	_ = store.SaveMarker(marker)
}

func (h *HookHandler) recordTurns(event HookEvent, session *Session) {
	store := h.Recorder.Store
	marker, ok, _ := store.LoadMarker(h.Agent, event.CWD)
	if !ok || marker.SessionID != event.SessionID {
		return
	}
	marker.Turns = len(session.Turns())
	_ = store.SaveMarker(marker)
}

// emitContext prints the JSON hosts read from a hook's stdout to add context.
func (h *HookHandler) emitContext(eventName, text string) {
	text = strings.TrimSpace(text)
	if text == "" || h.Stdout == nil {
		return
	}
	if h.Options.MaxContextChars > 0 {
		text = Truncate(text, h.Options.MaxContextChars)
	}
	payload := map[string]interface{}{
		"hookSpecificOutput": map[string]interface{}{
			"hookEventName":     eventName,
			"additionalContext": text,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintln(h.Stdout, string(encoded))
}

func (h *HookHandler) startContext(ctx context.Context, event HookEvent) string {
	if h.Recorder.Memdb == nil {
		return ""
	}
	items, err := h.Recorder.ProjectRecent(ctx, h.Agent, event.CWD, 12)
	if err != nil {
		h.Recorder.Store.AppendLog("start recall failed: %v", err)
		return ""
	}
	lines := []string{}
	for _, item := range items {
		if item.Type != "message" || item.SessionKey == SessionKey(h.Agent, ProjectKey(event.CWD), event.SessionID) {
			continue
		}
		text := strings.TrimSpace(item.Text)
		if text == "" || (item.AuthorRole != RoleUser && item.AuthorRole != RoleAssistant) {
			continue
		}
		lines = append(lines, fmt.Sprintf("- [%s %s] %s", shortDate(item.TS), item.AuthorRole, FirstLine(text, 200)))
		if len(lines) == 6 {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "treecli memory: recent exchanges from earlier " + AgentLabel(h.Agent) + " sessions in this project (memdb). Use the memory_recall / memory_search tools for more.\n" + strings.Join(lines, "\n")
}

func (h *HookHandler) promptContext(ctx context.Context, event HookEvent) string {
	if h.Recorder.Memdb == nil {
		return ""
	}
	query := FTSQuery(event.Prompt)
	if query == "" {
		return ""
	}
	payload, _, err := h.Recorder.Memdb.Recall(ctx, RecallRequest{
		Mode: "long-term", Query: query, QueryN: 4,
		ChatApp: string(h.Agent), ChatID: ProjectKey(event.CWD),
	})
	if err != nil {
		h.Recorder.Store.AppendLog("prompt recall failed: %v", err)
		return ""
	}
	if payload.LongTerm == nil || len(payload.LongTerm.Results) == 0 {
		return ""
	}
	lines := []string{}
	for _, item := range payload.LongTerm.Results {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		lines = append(lines, "- "+FirstLine(text, 240))
	}
	if len(lines) == 0 {
		return ""
	}
	return "treecli memory: durable memories that may apply to this prompt (memdb learned events):\n" + strings.Join(lines, "\n")
}

func shortDate(ts string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return parsed.Local().Format("Jan 2")
	}
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}
