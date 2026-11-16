package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func channelTestServer(t *testing.T, quoted *map[string]interface{}, body map[string]interface{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			if call.Arguments["operation"] != "channel" || call.Arguments["max_price_cents"] != float64(0) {
				t.Errorf("unexpected quote %#v", call.Arguments)
			}
			*quoted, _ = call.Arguments["arguments"].(map[string]interface{})
			result = map[string]interface{}{"quote_id": "server-quote"}
		case "create_treechat_write":
			result = map[string]interface{}{"operation_id": "server-quote", "state": "completed", "result": map[string]interface{}{"status": 200, "body": body}}
		default:
			t.Fatalf("unexpected RPC %s", call.Name)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": result})
	}))
}

func TestCreateChannelLeavesVisibilityToBackendUnlessFlagged(t *testing.T) {
	var quoted map[string]interface{}
	server := channelTestServer(t, &quoted, map[string]interface{}{"team": map[string]interface{}{"id": "team-id", "name": "Agent notes", "public": false}})
	defer server.Close()
	profile := profileConfig{BackendURL: server.URL, AccessToken: "token", Client: "client", UID: "uid"}

	result, writeID, err := createChannel(profile, channelCreateOptions{WriteID: testWriteID, Name: "Agent notes", Description: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]interface{}{"id": testWriteID, "name": "Agent notes", "description": "notes"}
	if !reflect.DeepEqual(quoted, expected) {
		t.Fatalf("expected quote arguments %#v, got %#v", expected, quoted)
	}
	if writeID != testWriteID || result.Team.ID != "team-id" || result.Team.Public {
		t.Fatalf("unexpected result %#v write id %q", result.Team, writeID)
	}
	if link := channelLink(profileConfig{AppHost: "https://app.example/"}, result.Team.ID); link != "https://app.example/stream/team-id" {
		t.Fatalf("unexpected channel link %q", link)
	}

	if _, _, err := createChannel(profile, channelCreateOptions{WriteID: testWriteID, Name: "Open floor", Public: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if quoted["public"] != true {
		t.Fatalf("expected public flag in quote, got %#v", quoted)
	}
}

func TestCreateChannelRejectsResponseWithoutChannel(t *testing.T) {
	var quoted map[string]interface{}
	server := channelTestServer(t, &quoted, map[string]interface{}{"error": "nothing created"})
	defer server.Close()

	_, _, err := createChannel(profileConfig{BackendURL: server.URL, AccessToken: "token", Client: "client", UID: "uid"}, channelCreateOptions{Name: "Agent notes"})
	if err == nil {
		t.Fatal("expected an error when the backend returns no channel")
	}
	if !looksLikeUUID(writeIDFromTestError(err)) {
		t.Fatalf("expected generated write id in error, got %v", err)
	}
}

func TestNewChannelRejectsBothVisibilityFlags(t *testing.T) {
	channelPublic, channelPrivate = true, true
	defer func() { channelPublic, channelPrivate = false, false }()
	if err := runNewChannel(newChannelCmd, []string{"Agent notes"}); err == nil {
		t.Fatal("expected a usage error for --public with --private")
	}
}
