package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestInviteToChannelQuotesChannelAndUsernames(t *testing.T) {
	var quoted map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Fatal(err)
		}
		var result interface{}
		switch call.Name {
		case "quote_treechat_write":
			if call.Arguments["operation"] != "channel_invite" || call.Arguments["max_price_cents"] != float64(0) {
				t.Errorf("unexpected quote %#v", call.Arguments)
			}
			quoted, _ = call.Arguments["arguments"].(map[string]interface{})
			result = map[string]interface{}{"quote_id": "server-quote"}
		case "create_treechat_write":
			result = map[string]interface{}{"operation_id": "server-quote", "state": "completed", "result": map[string]interface{}{"status": 200, "body": map[string]interface{}{"team": map[string]interface{}{
				"id": "team-id", "name": "Agent notes",
				"users":         []map[string]interface{}{{"name": "owner"}, {"name": "Carol"}},
				"pending_users": []map[string]interface{}{{"name": "Alice"}},
			}}}}
		default:
			t.Fatalf("unexpected RPC %s", call.Name)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": result})
	}))
	defer server.Close()

	usernames := cleanUsernames([]string{" @alice ", "bob", "ALICE", "", "carol"})
	if !reflect.DeepEqual(usernames, []string{"alice", "bob", "carol"}) {
		t.Fatalf("unexpected usernames %#v", usernames)
	}
	result, writeID, err := inviteToChannel(profileConfig{BackendURL: server.URL, AccessToken: "token", Client: "client", UID: "uid"}, testWriteID, "Agent notes", usernames)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]interface{}{"id": testWriteID, "channel": "Agent notes", "usernames": []interface{}{"alice", "bob", "carol"}}
	if !reflect.DeepEqual(quoted, expected) {
		t.Fatalf("expected quote arguments %#v, got %#v", expected, quoted)
	}
	if writeID != testWriteID {
		t.Fatalf("unexpected write id %q", writeID)
	}
	outcomes := map[string]string{"alice": "invited, waiting for them to accept", "bob": "not invited", "carol": "already a member"}
	for username, expectedOutcome := range outcomes {
		if outcome := inviteOutcome(result, username); outcome != expectedOutcome {
			t.Fatalf("expected %s to be %q, got %q", username, expectedOutcome, outcome)
		}
	}
}

func TestChannelInviteNeedsAChannelAndAUsername(t *testing.T) {
	if err := channelInviteCmd.Args(channelInviteCmd, []string{"Agent notes"}); err == nil {
		t.Fatal("expected an error with no usernames")
	}
	if err := runChannelInvite(channelInviteCmd, []string{"Agent notes", " @ "}); err == nil {
		t.Fatal("expected a usage error for a blank username")
	}
	if err := runChannelInvite(channelInviteCmd, []string{"  ", "alice"}); err == nil {
		t.Fatal("expected a usage error for a blank channel")
	}
}
