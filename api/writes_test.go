package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateQuestReturnsTypedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte(`{"error":"Thread already exists."}`))
	}))
	defer server.Close()

	_, err := CreateQuest(
		server.URL,
		"token",
		"client",
		"uid",
		CreateQuestRequest{
			QuestID:        "550e8400-e29b-41d4-a716-446655440000",
			ParentAnswerID: "650e8400-e29b-41d4-a716-446655440000",
			SpaceID:        "space-id",
			Content:        "safe retry",
		},
	)
	if err == nil {
		t.Fatal("expected conflict error")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, httpErr.StatusCode)
	}
	if httpErr.Body != `{"error":"Thread already exists."}` {
		t.Fatalf("unexpected safe response body: %q", httpErr.Body)
	}
}

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
