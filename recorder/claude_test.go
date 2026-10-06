package recorder

import (
	"strings"
	"testing"
)

const claudeFixture = `{"type":"summary","summary":"bookkeeping line"}
{"parentUuid":null,"isSidechain":false,"type":"user","message":{"role":"user","content":"fix the failing spec"},"uuid":"u1","timestamp":"2026-09-09T10:00:00.000Z","cwd":"/Users/me/src/app","sessionId":"sess-1","origin":{"kind":"human"}}
{"parentUuid":"u1","isSidechain":false,"type":"assistant","message":{"model":"claude-fable-5-1","role":"assistant","content":[{"type":"thinking","thinking":"private","signature":"x"},{"type":"text","text":"Reading the spec."},{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"bundle exec rspec spec/a_spec.rb"}}]},"uuid":"a1","timestamp":"2026-09-09T10:00:05.000Z","sessionId":"sess-1"}
{"parentUuid":"a1","isSidechain":false,"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_1","type":"tool_result","content":"1 example, 1 failure"}]},"uuid":"u2","timestamp":"2026-09-09T10:00:07.000Z","sessionId":"sess-1","toolUseResult":{"stdout":"..."}}
{"parentUuid":"u2","isSidechain":false,"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_2","name":"Edit","input":{"file_path":"spec/a_spec.rb","old_string":"1","new_string":"2"}}]},"uuid":"a2","timestamp":"2026-09-09T10:00:09.000Z","sessionId":"sess-1"}
{"parentUuid":"a2","isSidechain":false,"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_2","type":"tool_result","content":[{"type":"text","text":"edited"}]}]},"uuid":"u3","timestamp":"2026-09-09T10:00:10.000Z","sessionId":"sess-1"}
{"parentUuid":"u3","isSidechain":false,"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Fixed: the expectation was off by one."}]},"uuid":"a3","timestamp":"2026-09-09T10:00:12.000Z","sessionId":"sess-1"}
{"parentUuid":"a3","isSidechain":true,"type":"user","message":{"role":"user","content":"subagent prompt"},"uuid":"s1","timestamp":"2026-09-09T10:00:13.000Z","sessionId":"sess-1"}
{"parentUuid":"a3","isSidechain":false,"isMeta":true,"type":"user","message":{"role":"user","content":"<local-command-caveat>internal</local-command-caveat>"},"uuid":"m1","timestamp":"2026-09-09T10:00:14.000Z","sessionId":"sess-1"}
{"parentUuid":"a3","isSidechain":false,"type":"user","message":{"role":"user","content":"<task-notification>\n<task-id>x</task-id>\n</task-notification>"},"uuid":"n1","timestamp":"2026-09-09T10:00:15.000Z","sessionId":"sess-1"}
{"parentUuid":"a3","isSidechain":false,"type":"user","message":{"role":"user","content":"<system-reminder>ignore me</system-reminder>\nnow run the whole suite"},"uuid":"u4","timestamp":"2026-09-09T10:01:00.000Z","sessionId":"sess-1","origin":{"kind":"human"}}
not json at all
`

func TestParseClaudeTranscriptKeepsTheMainConversation(t *testing.T) {
	session, err := ParseClaudeTranscript(strings.NewReader(claudeFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if session.Agent != AgentClaudeCode || session.SessionID != "sess-1" || session.CWD != "/Users/me/src/app" || session.Model != "claude-fable-5-1" {
		t.Fatalf("unexpected session header: %+v", session)
	}
	roles := []string{}
	for _, message := range session.Messages {
		roles = append(roles, message.Role)
	}
	expected := []string{RoleUser, RoleAssistant, RoleToolResult, RoleToolResult, RoleAssistant, RoleUser}
	if strings.Join(roles, ",") != strings.Join(expected, ",") {
		t.Fatalf("roles = %v, want %v", roles, expected)
	}
	first := session.Messages[1]
	if first.Text != "Reading the spec." {
		t.Fatalf("thinking blocks must be dropped and text kept, got %q", first.Text)
	}
	// The tool-only assistant message (a2) folds into a1 within the same turn.
	if len(first.ToolCalls) != 2 || first.ToolCalls[0].Name != "Bash" || first.ToolCalls[1].Name != "Edit" {
		t.Fatalf("expected Bash+Edit tool calls merged into the first assistant message, got %+v", first.ToolCalls)
	}
	if session.Messages[2].ToolCallID != "toolu_1" || session.Messages[2].Text != "1 example, 1 failure" {
		t.Fatalf("unexpected tool result: %+v", session.Messages[2])
	}
	if session.Messages[3].Text != "edited" {
		t.Fatalf("array tool results must flatten, got %q", session.Messages[3].Text)
	}
	last := session.Messages[5]
	if last.Text != "now run the whole suite" {
		t.Fatalf("system reminders must be stripped from prompts, got %q", last.Text)
	}
	if turns := session.Turns(); len(turns) != 2 || turns[0].FinalReply != "Fixed: the expectation was off by one." || turns[0].ToolCallsSum != 2 {
		t.Fatalf("unexpected turns: %+v", turns)
	}
}

func TestParseClaudeTranscriptTruncatesHugeToolOutput(t *testing.T) {
	huge := strings.Repeat("y", MaxToolResultChars*3)
	transcript := `{"type":"user","message":{"role":"user","content":"go"},"uuid":"u1","timestamp":"2026-09-09T10:00:00.000Z","sessionId":"s","cwd":"/x","origin":{"kind":"human"}}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t","type":"tool_result","content":"` + huge + `"}]},"uuid":"u2","timestamp":"2026-09-09T10:00:01.000Z","sessionId":"s"}
`
	session, err := ParseClaudeTranscript(strings.NewReader(transcript))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(session.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(session.Messages))
	}
	if got := len([]rune(session.Messages[1].Text)); got > MaxToolResultChars+40 {
		t.Fatalf("tool result not truncated: %d runes", got)
	}
	if !strings.Contains(session.Messages[1].Text, "[truncated") {
		t.Fatal("truncation marker missing")
	}
}
