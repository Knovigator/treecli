package recorder

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectTestServer(t *testing.T, rec *Recorder, agent Agent, cwd string) *mcp.ClientSession {
	t.Helper()
	server := NewMCPServer(MCPServerOptions{Agent: agent, CWD: cwd, Recorder: rec, Version: "test"})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callText(t *testing.T, session *mcp.ClientSession, name string, args map[string]interface{}) (string, bool) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error %v", name, err)
	}
	texts := []string{}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	return strings.Join(texts, "\n"), result.IsError
}

func TestMCPServerListsToolsAndScopesToTheMarkedSession(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	recall := `{"mode":"thread","query":"deploy","scope":{},"thread":{"recent":[{"type":"message","ts":"2026-09-09T10:00:00Z","author_role":"user","text":"deploy the radio fix"}],"query":[]},"long_term":null}`
	memdb, logPath := fakeMemdb(t, "TREECLI_FAKE_MEMDB_RECALL_JSON="+recall)
	rec := &Recorder{Store: store, Memdb: memdb}
	if err := store.SaveMarker(Marker{Agent: AgentClaudeCode, SessionID: "live-1", CWD: "/w"}); err != nil {
		t.Fatal(err)
	}
	session := connectTestServer(t, rec, AgentClaudeCode, "/w")

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	for _, expected := range []string{"memory_recall", "memory_search", "memory_learn", "session_note", "treechat_post", "session_info"} {
		if !contains(names, expected) {
			t.Fatalf("tool %s missing from %v", expected, names)
		}
	}

	text, isError := callText(t, session, "session_info", nil)
	if isError || !strings.Contains(text, `"session_id": "live-1"`) || !strings.Contains(text, `"project_key": "-w"`) {
		t.Fatalf("unexpected session_info: %s (error=%v)", text, isError)
	}

	text, isError = callText(t, session, "memory_recall", map[string]interface{}{"query": "deploy the radio fix", "mode": "thread"})
	if isError || !strings.Contains(text, "deploy the radio fix") {
		t.Fatalf("unexpected recall: %s (error=%v)", text, isError)
	}
	if !hasCall(fakeMemdbCalls(t, logPath), "recall", "--session-key agent:treecli:claude-code:group:-w:thread:live-1", "--query deploy OR radio OR fix") {
		t.Fatalf("recall must scope to the marked session with a keyword query, got %v", fakeMemdbCalls(t, logPath))
	}

	_, isError = callText(t, session, "memory_learn", map[string]interface{}{"type": "rumour", "title": "x"})
	if !isError {
		t.Fatal("invalid memory type must be a tool error")
	}
	text, isError = callText(t, session, "memory_learn", map[string]interface{}{"type": "decision", "title": "Radio dead-air fix ships behind a flag", "summary": "see PR 3263", "tags": []string{"radio"}})
	if isError || !strings.Contains(text, `"memory_type": "decision"`) {
		t.Fatalf("unexpected learn result: %s (error=%v)", text, isError)
	}
	if !hasCall(fakeMemdbCalls(t, logPath), "learn", "--type decision", "--chat-app claude-code", "--chat-id -w", "--session-id live-1", "--tag radio") {
		t.Fatalf("learn must default to project scope, got %v", fakeMemdbCalls(t, logPath))
	}

	text, isError = callText(t, session, "session_note", map[string]interface{}{"text": "checkpoint: specs green"})
	if isError || !strings.Contains(text, `"recorded": true`) {
		t.Fatalf("unexpected note result: %s (error=%v)", text, isError)
	}
	loaded, ok, err := rec.LoadRecording(AgentClaudeCode, "live-1")
	if err != nil || !ok || len(loaded.Messages) != 1 || loaded.Messages[0].Text != "Note: checkpoint: specs green" {
		t.Fatalf("note not recorded: %+v %v %v", loaded, ok, err)
	}

	_, isError = callText(t, session, "treechat_post", map[string]interface{}{"text": "hello"})
	if !isError {
		t.Fatal("treechat_post must fail while the Treechat lane is off")
	}
}

func TestMCPServerWithoutMarkerWorksAtProjectScope(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	memdb, logPath := fakeMemdb(t)
	rec := &Recorder{Store: store, Memdb: memdb}
	session := connectTestServer(t, rec, AgentCodex, "/p")
	text, isError := callText(t, session, "memory_search", map[string]interface{}{"query": "share reward report"})
	if isError {
		t.Fatalf("search failed: %s", text)
	}
	if !hasCall(fakeMemdbCalls(t, logPath), "query", "share OR reward OR report", "--chat-app codex", "--chat-id -p") {
		t.Fatalf("search must default to project scope, got %v", fakeMemdbCalls(t, logPath))
	}
	_, isError = callText(t, session, "memory_search", map[string]interface{}{"query": "x", "scope": "session"})
	if !isError {
		t.Fatal("session scope without a marker must be a tool error")
	}
	text, isError = callText(t, session, "session_info", nil)
	if isError || !strings.Contains(text, "no active session marker") {
		t.Fatalf("unexpected info: %s", text)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatalf("session_info must be JSON: %v", err)
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
