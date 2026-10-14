package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetUpvalueHistorySendsAuthAndPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/treecli/call" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var call struct {
			Name      string `json:"name"`
			Arguments struct {
				Operation string                 `json:"operation"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Fatal(err)
		}
		if call.Name != "treechat_read" || call.Arguments.Operation != "upvalues" || call.Arguments.Arguments["page"] != float64(2) || call.Arguments.Arguments["per_page"] != float64(25) {
			t.Fatalf("unexpected RPC: %#v", call)
		}

		if r.Header.Get("access-token") != "token" || r.Header.Get("client") != "client" || r.Header.Get("uid") != "uid" {
			t.Fatalf("missing authentication headers")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"structuredContent":{
          "upvalues": [{
            "id": "upvalue-1",
            "tx_id": "tx-1",
            "amount": "40000.0",
            "fee": "6000.0",
            "created_at": "2026-07-30T01:41:13.835-04:00",
            "from_user_id": "sender",
            "to_user_id": "recipient",
            "is_boost": false,
            "metadata": {"sentiment_emoji": "🌱"},
            "status": "completed",
            "from_user": {"id": "sender", "name": "Sender"},
            "to_user": {"id": "recipient", "name": "Recipient"},
            "bsv_tx": {"id": "bsv-tx-1", "tx_id": "tx-1", "status": "confirmed", "confirmations": 2, "confirmed": true},
            "answer": {"id": "answer-1", "child_quests": [{"id": "quest-1"}]}
          }],
          "page": 2,
          "per_page": 25,
          "has_more": true
        }}`)
	}))
	defer server.Close()

	history, err := GetUpvalueHistory(server.URL, "token", "client", "uid", 2, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if history.Page != 2 || history.PerPage != 25 || !history.HasMore {
		t.Fatalf("unexpected pagination: %#v", history)
	}
	if len(history.Upvalues) != 1 || history.Upvalues[0].Amount != "40000.0" {
		t.Fatalf("unexpected upvalues: %#v", history.Upvalues)
	}
	if history.Upvalues[0].Answer.ID != "answer-1" || history.Upvalues[0].Answer.ChildQuests[0].ID != "quest-1" {
		t.Fatalf("unexpected answer reference: %#v", history.Upvalues[0].Answer)
	}
	if len(history.Raw) == 0 {
		t.Fatal("expected raw response to be retained")
	}
}
