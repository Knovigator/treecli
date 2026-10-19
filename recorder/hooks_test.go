package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestHandler(t *testing.T, agent Agent, extraEnv ...string) (*HookHandler, *bytes.Buffer, string) {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	memdb, logPath := fakeMemdb(t, extraEnv...)
	stdout := &bytes.Buffer{}
	handler := &HookHandler{
		Agent:    agent,
		Recorder: &Recorder{Store: store, Memdb: memdb},
		Options:  HookOptions{RecallOnStart: true, RecallOnPrompt: true, MaxContextChars: 1800},
		Stdout:   stdout,
		Now:      func() time.Time { return time.Date(2026, 9, 9, 15, 0, 0, 0, time.UTC) },
	}
	return handler, stdout, logPath
}

func TestParseHookPayloadNormalizesDialects(t *testing.T) {
	claude := []byte(`{"session_id":"s1","transcript_path":"/t.jsonl","cwd":"/w","hook_event_name":"UserPromptSubmit","prompt":"hi"}`)
	event, err := ParseHookPayload(claude, "")
	if err != nil || event.Name != EventUserPromptSubmit || event.SessionID != "s1" || event.TranscriptPath != "/t.jsonl" || event.CWD != "/w" || event.Prompt != "hi" {
		t.Fatalf("claude payload parsed wrong: %+v (%v)", event, err)
	}
	grok := []byte(`{"sessionId":"g1","hookEventName":"post_tool_use","cwd":"/g","toolName":"run_terminal_command","toolInput":{"command":"ls"},"toolResult":"a\nb"}`)
	event, err = ParseHookPayload(grok, "")
	if err != nil || event.Name != EventPostToolUse || event.SessionID != "g1" || event.ToolName != "run_terminal_command" || string(event.ToolInput) != `{"command":"ls"}` {
		t.Fatalf("grok payload parsed wrong: %+v (%v)", event, err)
	}
	notify := []byte(`{"type":"agent-turn-complete","thread-id":"c1","cwd":"/c","last-assistant-message":"done"}`)
	event, err = ParseHookPayload(notify, "agent-turn-complete")
	if err != nil || event.Name != EventStop || event.SessionID != "c1" || event.LastAssistantMessage != "done" {
		t.Fatalf("codex notify payload parsed wrong: %+v (%v)", event, err)
	}
	if _, err := ParseHookPayload([]byte(`[1,2]`), ""); err == nil {
		t.Fatal("non-object payloads must be rejected")
	}
}

func TestHookHandlerRecordsClaudeSessionFromTranscript(t *testing.T) {
	transcript := writeFixture(t, "sess-1.jsonl", claudeFixture)
	handler, stdout, logPath := newTestHandler(t, AgentClaudeCode)
	ctx := context.Background()
	start := HookEvent{Name: EventSessionStart, SessionID: "sess-1", TranscriptPath: transcript, CWD: "/Users/me/src/app", Source: "startup"}
	if err := handler.Handle(ctx, start); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	marker, ok, err := handler.Recorder.Store.LoadMarker(AgentClaudeCode, "/Users/me/src/app")
	if err != nil || !ok || marker.SessionID != "sess-1" || marker.TranscriptPath != transcript {
		t.Fatalf("marker not written: %+v %v %v", marker, ok, err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("no history yet, so SessionStart must stay silent; got %q", stdout.String())
	}
	if err := handler.Handle(ctx, HookEvent{Name: EventStop, SessionID: "sess-1", TranscriptPath: transcript, CWD: "/Users/me/src/app"}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	recording := handler.Recorder.Store.RecordingPath(AgentClaudeCode, "sess-1")
	data, err := os.ReadFile(recording)
	if err != nil {
		t.Fatalf("recording missing: %v", err)
	}
	if !strings.Contains(string(data), `"sessionKey":"agent:treecli:claude-code:group:-Users-me-src-app:thread:sess-1"`) {
		t.Fatalf("recording lacks the session key:\n%s", data)
	}
	if !strings.Contains(string(data), `"cwd":"/Users/me/src/app"`) {
		t.Fatalf("hook cwd must win over the transcript cwd:\n%s", data)
	}
	calls := fakeMemdbCalls(t, logPath)
	if !hasCall(calls, "ingest", "--chat-app claude-code", recording) {
		t.Fatalf("expected an ingest call for %s, got %v", recording, calls)
	}
	marker, _, _ = handler.Recorder.Store.LoadMarker(AgentClaudeCode, "/Users/me/src/app")
	if marker.Turns != 2 || marker.Ended {
		t.Fatalf("marker should count 2 turns and stay active: %+v", marker)
	}
	if err := handler.Handle(ctx, HookEvent{Name: EventSessionEnd, SessionID: "sess-1", TranscriptPath: transcript, CWD: "/Users/me/src/app", Reason: "other"}); err != nil {
		t.Fatalf("SessionEnd: %v", err)
	}
	marker, _, _ = handler.Recorder.Store.LoadMarker(AgentClaudeCode, "/Users/me/src/app")
	if !marker.Ended {
		t.Fatalf("SessionEnd must close the marker: %+v", marker)
	}
	ingests := 0
	for _, call := range fakeMemdbCalls(t, logPath) {
		if hasCall([][]string{call}, "ingest") {
			ingests++
		}
	}
	if ingests != 2 {
		t.Fatalf("each sync ingests once (memdb itself skips unchanged files), got %v", fakeMemdbCalls(t, logPath))
	}
}

func TestHookHandlerInjectsContextAtStartAndPrompt(t *testing.T) {
	recent := `{"scope":{},"results":[{"type":"message","ts":"2026-09-01T10:00:00Z","author_role":"user","text":"migrate the UTXO ledger","session_key":"agent:treecli:claude-code:group:-w:thread:old"},{"type":"message","ts":"2026-09-01T10:01:00Z","author_role":"assistant","text":"Done; the ledger now lives in Postgres.","session_key":"agent:treecli:claude-code:group:-w:thread:old"},{"type":"message","ts":"2026-09-01T10:02:00Z","author_role":"toolResult","text":"noise","session_key":"agent:treecli:claude-code:group:-w:thread:old"}]}`
	longTerm := `{"mode":"long_term","query":"ledger","scope":{},"thread":null,"long_term":{"results":[{"type":"event","ts":"2026-09-01T10:03:00Z","text":"[decision] UTXO ledger lives in Postgres, not Redis","kind":"learned"}]}}`
	handler, stdout, logPath := newTestHandler(t, AgentClaudeCode, "TREECLI_FAKE_MEMDB_RECENT_JSON="+recent, "TREECLI_FAKE_MEMDB_RECALL_JSON="+longTerm)
	ctx := context.Background()
	if err := handler.Handle(ctx, HookEvent{Name: EventSessionStart, SessionID: "new", CWD: "/w", Source: "startup"}); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	var output map[string]map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("SessionStart stdout must be JSON: %v (%q)", err, stdout.String())
	}
	context := output["hookSpecificOutput"]["additionalContext"]
	if output["hookSpecificOutput"]["hookEventName"] != EventSessionStart || !strings.Contains(context, "migrate the UTXO ledger") || !strings.Contains(context, "Postgres") {
		t.Fatalf("unexpected start context: %v", output)
	}
	if strings.Contains(context, "noise") {
		t.Fatal("tool results must not be injected")
	}
	stdout.Reset()
	if err := handler.Handle(ctx, HookEvent{Name: EventUserPromptSubmit, SessionID: "new", CWD: "/w", Prompt: "where is the ledger stored now?"}); err != nil {
		t.Fatalf("UserPromptSubmit: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("UserPromptSubmit stdout must be JSON: %v (%q)", err, stdout.String())
	}
	if !strings.Contains(output["hookSpecificOutput"]["additionalContext"], "UTXO ledger lives in Postgres") {
		t.Fatalf("unexpected prompt context: %v", output)
	}
	calls := fakeMemdbCalls(t, logPath)
	if !hasCall(calls, "recall", "--mode long-term", "--query ledger OR stored", "--chat-app claude-code", "--chat-id -w") {
		t.Fatalf("expected a keyword OR recall scoped to the project, got %v", calls)
	}
	if !hasCall(calls, "recent", "--chat-app claude-code", "--chat-id -w") {
		t.Fatalf("expected a project-scoped recent call, got %v", calls)
	}
}

func TestHookHandlerBuildsGrokSessionsFromPayloadsOnly(t *testing.T) {
	handler, _, logPath := newTestHandler(t, AgentGrok)
	ctx := context.Background()
	steps := []HookEvent{
		{Name: EventSessionStart, SessionID: "g1", CWD: "/g"},
		{Name: EventUserPromptSubmit, SessionID: "g1", CWD: "/g", Prompt: "list the files"},
		{Name: EventPostToolUse, SessionID: "g1", CWD: "/g", ToolName: "run_terminal_command", ToolInput: json.RawMessage(`{"command":"ls"}`), ToolResponse: json.RawMessage(`"a.txt\nb.txt"`)},
		{Name: EventStop, SessionID: "g1", CWD: "/g", LastAssistantMessage: "Two files: a.txt and b.txt."},
		{Name: EventSessionEnd, SessionID: "g1", CWD: "/g"},
	}
	for _, step := range steps {
		if err := handler.Handle(ctx, step); err != nil {
			t.Fatalf("%s: %v", step.Name, err)
		}
	}
	session, ok, err := handler.Recorder.LoadRecording(AgentGrok, "g1")
	if err != nil || !ok {
		t.Fatalf("recording missing: %v %v", ok, err)
	}
	roles := []string{}
	for _, message := range session.Messages {
		roles = append(roles, message.Role)
	}
	if strings.Join(roles, ",") != "user,assistant,toolResult,assistant" {
		t.Fatalf("unexpected roles %v", roles)
	}
	if session.Messages[1].ToolCalls[0].Name != "run_terminal_command" || session.Messages[2].Text != "a.txt\nb.txt" || session.Messages[3].Text != "Two files: a.txt and b.txt." {
		t.Fatalf("payload-driven messages wrong: %+v", session.Messages)
	}
	turns := session.Turns()
	if len(turns) != 1 || turns[0].ToolCallsSum != 1 || turns[0].FinalReply == "" {
		t.Fatalf("unexpected turns %+v", turns)
	}
	if !hasCall(fakeMemdbCalls(t, logPath), "ingest", "--chat-app grok") {
		t.Fatalf("expected grok ingest, got %v", fakeMemdbCalls(t, logPath))
	}
}

func TestHookHandlerSurvivesMemdbFailure(t *testing.T) {
	transcript := writeFixture(t, "sess-1.jsonl", claudeFixture)
	handler, _, _ := newTestHandler(t, AgentClaudeCode, "TREECLI_FAKE_MEMDB_FAIL=1")
	err := handler.Handle(context.Background(), HookEvent{Name: EventStop, SessionID: "sess-1", TranscriptPath: transcript, CWD: "/w"})
	if err == nil || !strings.Contains(err.Error(), "fake memdb failure") {
		t.Fatalf("expected the memdb failure to surface as a diagnostic, got %v", err)
	}
	if _, statErr := os.Stat(handler.Recorder.Store.RecordingPath(AgentClaudeCode, "sess-1")); statErr != nil {
		t.Fatal("the recording file must still be written before memdb runs")
	}
}
