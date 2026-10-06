package recorder

import (
	"strings"
	"testing"
)

const codexFixture = `{"timestamp":"2026-09-02T21:55:43.000Z","type":"session_meta","payload":{"id":"01a0-thread","timestamp":"2026-09-02T21:55:43.000Z","cwd":"/Users/me/src/app","originator":"codex_cli_rs","cli_version":"0.142.3"}}
{"timestamp":"2026-09-02T21:55:44.000Z","type":"turn_context","payload":{"turn_id":"t1","cwd":"/Users/me/src/app","model":"gpt-6-astra"}}
{"timestamp":"2026-09-02T21:55:44.100Z","type":"response_item","payload":{"type":"message","id":"msg_dev","role":"developer","content":[{"type":"input_text","text":"<permissions instructions>never</permissions instructions>"}]}}
{"timestamp":"2026-09-02T21:55:44.200Z","type":"response_item","payload":{"type":"message","id":"msg_env","role":"user","content":[{"type":"input_text","text":"<environment_context>\n<cwd>/Users/me/src/app</cwd>\n</environment_context>"}]}}
{"timestamp":"2026-09-02T21:55:45.000Z","type":"response_item","payload":{"type":"message","id":"msg_u1","role":"user","content":[{"type":"input_text","text":"add a signup notification"}]}}
{"timestamp":"2026-09-02T21:55:46.000Z","type":"event_msg","payload":{"type":"task_started","turn_id":"t1"}}
{"timestamp":"2026-09-02T21:55:50.000Z","type":"response_item","payload":{"type":"reasoning","summary":[],"encrypted_content":"zzz"}}
{"timestamp":"2026-09-02T21:55:51.000Z","type":"response_item","payload":{"type":"message","id":"msg_a1","role":"assistant","content":[{"type":"output_text","text":"I will look at the signup flow."}],"phase":"commentary"}}
{"timestamp":"2026-09-02T21:55:52.000Z","type":"response_item","payload":{"type":"custom_tool_call","id":"ctc_1","status":"completed","call_id":"call_1","name":"exec","input":"rg -n signup app/"}}
{"timestamp":"2026-09-02T21:55:53.000Z","type":"response_item","payload":{"type":"custom_tool_call_output","id":"ctco_1","call_id":"call_1","output":[{"type":"input_text","text":"app/models/user.rb:12: after_create :notify"}]}}
{"timestamp":"2026-09-02T21:55:54.000Z","type":"response_item","payload":{"type":"function_call","id":"fc_1","name":"sleep","namespace":"clock","arguments":"{\"duration_ms\":1000}","call_id":"call_2"}}
{"timestamp":"2026-09-02T21:55:55.000Z","type":"response_item","payload":{"type":"function_call_output","id":"fco_1","call_id":"call_2","output":"Sleep completed."}}
{"timestamp":"2026-09-02T21:55:56.000Z","type":"response_item","payload":{"type":"message","id":"msg_a2","role":"assistant","content":[{"type":"output_text","text":"Done: after_create now enqueues the push."}],"phase":"final_answer"}}
{"timestamp":"2026-09-02T21:55:57.000Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"t1","last_agent_message":"Done: after_create now enqueues the push."}}
{"timestamp":"2026-09-02T21:56:00.000Z","type":"token_usage_record","payload":{"session_id":"01a0-thread"}}
`

func TestParseCodexRolloutExtractsPromptsRepliesAndTools(t *testing.T) {
	session, err := ParseCodexRollout(strings.NewReader(codexFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if session.Agent != AgentCodex || session.SessionID != "01a0-thread" || session.CWD != "/Users/me/src/app" || session.Model != "gpt-6-astra" {
		t.Fatalf("unexpected session header: %+v", session)
	}
	roles := []string{}
	for _, message := range session.Messages {
		roles = append(roles, message.Role)
	}
	expected := []string{RoleUser, RoleAssistant, RoleToolResult, RoleToolResult, RoleAssistant}
	if strings.Join(roles, ",") != strings.Join(expected, ",") {
		t.Fatalf("roles = %v, want %v", roles, expected)
	}
	if session.Messages[0].Text != "add a signup notification" {
		t.Fatalf("injected environment context must be dropped; first user text %q", session.Messages[0].Text)
	}
	first := session.Messages[1]
	if first.Text != "I will look at the signup flow." || len(first.ToolCalls) != 2 {
		t.Fatalf("tool calls should fold into the preceding assistant message: %+v", first)
	}
	if first.ToolCalls[0].Name != "exec" || !strings.Contains(string(first.ToolCalls[0].Arguments), "rg -n signup") {
		t.Fatalf("custom tool call lost its input: %+v", first.ToolCalls[0])
	}
	if first.ToolCalls[1].Name != "sleep" || string(first.ToolCalls[1].Arguments) != `{"duration_ms":1000}` {
		t.Fatalf("function call arguments must stay JSON: %+v", first.ToolCalls[1])
	}
	if session.Messages[2].ToolCallID != "call_1" || !strings.Contains(session.Messages[2].Text, "after_create :notify") {
		t.Fatalf("unexpected custom tool output: %+v", session.Messages[2])
	}
	if session.Messages[3].Text != "Sleep completed." {
		t.Fatalf("unexpected function output: %+v", session.Messages[3])
	}
	if turns := session.Turns(); len(turns) != 1 || turns[0].FinalReply != "Done: after_create now enqueues the push." || turns[0].ToolCallsSum != 2 {
		t.Fatalf("unexpected turns: %+v", turns)
	}
}
