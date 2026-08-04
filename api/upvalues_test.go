package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetUpvalueHistorySendsAuthAndPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/bsv/history" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "25" {
			t.Fatalf("unexpected pagination query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("access-token") != "token" || r.Header.Get("client") != "client" || r.Header.Get("uid") != "uid" {
			t.Fatalf("missing authentication headers")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
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
        }`)
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
