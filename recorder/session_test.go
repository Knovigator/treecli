package recorder

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProjectKeyMirrorsClaudeProjectDirNaming(t *testing.T) {
	cases := map[string]string{
		"/Users/me/src/treechat":                     "-Users-me-src-treechat",
		"/Users/me/src/treechat/.claude/worktrees/x": "-Users-me-src-treechat--claude-worktrees-x",
		"C:\\work\\app":                              "C--work-app",
		"":                                           "-",
	}
	for input, expected := range cases {
		if got := ProjectKey(input); got != expected {
			t.Errorf("ProjectKey(%q) = %q, want %q", input, got, expected)
		}
	}
	if strings.Contains(ProjectKey("/a:b/c"), ":") {
		t.Fatal("project keys must never contain ':' (memdb session key separator)")
	}
}

func TestSessionKeyFollowsMemdbGrammar(t *testing.T) {
	key := SessionKey(AgentClaudeCode, "-Users-me-src-app", "sess-1")
	if key != "agent:treecli:claude-code:group:-Users-me-src-app:thread:sess-1" {
		t.Fatalf("unexpected session key %q", key)
	}
	if ProjectSessionKey(AgentCodex, "p") != "agent:treecli:codex:group:p" {
		t.Fatalf("unexpected project key %q", ProjectSessionKey(AgentCodex, "p"))
	}
}

func TestParseAgentAcceptsAliases(t *testing.T) {
	for input, expected := range map[string]Agent{"claude": AgentClaudeCode, "Claude-Code": AgentClaudeCode, "codex": AgentCodex, "grok-build": AgentGrok, "GROK": AgentGrok} {
		agent, err := ParseAgent(input)
		if err != nil || agent != expected {
			t.Errorf("ParseAgent(%q) = %q, %v; want %q", input, agent, err, expected)
		}
	}
	if _, err := ParseAgent("cursor"); err == nil {
		t.Fatal("expected unknown agent error")
	}
}

func TestTurnsGroupPromptsWithRepliesAndToolCalls(t *testing.T) {
	base := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	session := &Session{Messages: []Message{
		{ID: "u1", Role: RoleUser, Text: "fix the build", Timestamp: base},
		{ID: "a1", Role: RoleAssistant, Text: "Looking.", ToolCalls: []ToolCall{{Name: "Bash"}, {Name: "Bash"}, {Name: "Read"}}, Timestamp: base.Add(time.Minute)},
		{ID: "t1", Role: RoleToolResult, Text: "ok", Timestamp: base.Add(2 * time.Minute)},
		{ID: "a2", Role: RoleAssistant, Text: "Fixed it.", Timestamp: base.Add(3 * time.Minute)},
		{ID: "u2", Role: RoleUser, Text: "thanks", Timestamp: base.Add(4 * time.Minute)},
	}}
	turns := session.Turns()
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	first := turns[0]
	if first.UserPrompt != "fix the build" || first.FinalReply != "Fixed it." || first.ToolCallsSum != 3 {
		t.Fatalf("unexpected first turn: %+v", first)
	}
	if first.Assistant != "Looking.\n\nFixed it." {
		t.Fatalf("unexpected concatenated assistant text %q", first.Assistant)
	}
	if !first.EndedAt.Equal(base.Add(3 * time.Minute)) {
		t.Fatalf("turn end should track the last message, got %s", first.EndedAt)
	}
	if summary := first.ToolSummary(); summary != "3 tool calls: Bash×2, Read×1" {
		t.Fatalf("unexpected tool summary %q", summary)
	}
	if turns[1].FinalReply != "" || turns[1].ToolSummary() != "" {
		t.Fatalf("second turn should be empty, got %+v", turns[1])
	}
}

func TestDeterministicIDIsStableAndUUIDShaped(t *testing.T) {
	first := DeterministicID("turn", "claude-code", "sess", "3")
	second := DeterministicID("turn", "claude-code", "sess", "3")
	other := DeterministicID("turn", "claude-code", "sess", "4")
	if first != second {
		t.Fatalf("same inputs produced %q and %q", first, second)
	}
	if first == other {
		t.Fatal("different inputs produced the same id")
	}
	if len(first) != 36 || first[14] != '5' {
		t.Fatalf("expected a version-5 UUID layout, got %q", first)
	}
	if DeterministicID("a", "b", "c") == DeterministicID("a", "bc") {
		t.Fatal("part boundaries must be significant")
	}
}

func TestTruncateAndFirstLine(t *testing.T) {
	if got := Truncate("héllo wörld", 5); !strings.HasPrefix(got, "héllo…") {
		t.Fatalf("Truncate counts runes, got %q", got)
	}
	if got := Truncate("short", 10); got != "short" {
		t.Fatalf("Truncate must not touch short text, got %q", got)
	}
	if got := FirstLine("\n\n  first line here\nsecond", 8); got != "first li…" {
		t.Fatalf("unexpected FirstLine %q", got)
	}
}

func TestCompactArgumentsBoundsLargeToolInput(t *testing.T) {
	big := strings.Repeat("x", 10000)
	raw, _ := json.Marshal(map[string]string{"content": big, "path": "/tmp/a"})
	compacted := compactArguments(raw)
	if len(compacted) >= len(raw) {
		t.Fatalf("expected compaction, got %d bytes from %d", len(compacted), len(raw))
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(compacted, &decoded); err != nil {
		t.Fatalf("compacted arguments must stay valid JSON: %v", err)
	}
	if decoded["path"] != "/tmp/a" {
		t.Fatalf("short fields must survive, got %v", decoded["path"])
	}
	if small := compactArguments([]byte(`{"a":1}`)); string(small) != `{"a":1}` {
		t.Fatalf("small arguments must be untouched, got %s", small)
	}
}
