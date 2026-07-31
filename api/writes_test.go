package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateClipQuestSendsCallerQuestID(t *testing.T) {
	const questID = "550e8400-e29b-41d4-a716-446655440000"

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/plugin_new/clip" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("access-token") != "token" ||
			request.Header.Get("client") != "client" ||
			request.Header.Get("uid") != "uid" {
			t.Errorf("missing Treechat auth headers")
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parsing form: %v", err)
		}
		if request.Form.Get("id") != questID {
			t.Errorf("expected quest id %q, got %q", questID, request.Form.Get("id"))
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"id":%q}`, questID)
	}))
	defer server.Close()

	result, err := CreateClipQuest(
		server.URL,
		"token",
		"client",
		"uid",
		CreateClipQuestRequest{
			QuestID: questID,
			Content: "safe clip retry",
		},
	)
	if err != nil {
		t.Fatalf("unexpected create clip error: %v", err)
	}
	if result.Quest.ID != questID {
		t.Fatalf("expected result quest id %q, got %q", questID, result.Quest.ID)
	}
}
