package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const testWriteID = "550e8400-e29b-41d4-a716-446655440000"

func TestResolveWriteIDUsesCallerIDOrGeneratesOne(t *testing.T) {
	callerID, err := resolveWriteID(strings.ToUpper(testWriteID))
	if err != nil {
		t.Fatalf("unexpected caller id error: %v", err)
	}
	if callerID != testWriteID {
		t.Fatalf("expected normalized caller id %q, got %q", testWriteID, callerID)
	}

	generatedID, err := resolveWriteID("")
	if err != nil {
		t.Fatalf("unexpected generated id error: %v", err)
	}
	if !looksLikeUUID(generatedID) {
		t.Fatalf("expected generated UUID, got %q", generatedID)
	}

	if _, err := resolveWriteID("not-a-uuid"); err == nil {
		t.Fatal("expected invalid caller id to fail")
	}
}

func TestWriteOutcomeErrorPreservesRetryID(t *testing.T) {
	err := withWriteID(testWriteID, fmt.Errorf("request timed out"))
	if writeIDFromTestError(err) != testWriteID {
		t.Fatalf("expected write id %q, got %q", testWriteID, writeIDFromTestError(err))
	}
	if !strings.Contains(err.Error(), "reuse this value with --id") {
		t.Fatalf("expected retry guidance, got %v", err)
	}
}

func TestContentWriteCommandsExposeIDFlag(t *testing.T) {
	for _, command := range []*cobra.Command{
		newPostCmd,
		newReplyCmd,
		newClipCmd,
		ActionCmd,
	} {
		if command.Flags().Lookup("id") == nil {
			t.Fatalf("expected %s to expose --id", command.Name())
		}
	}
}

func TestCreateReplyUsesWriteIDWithoutASecondRetryID(t *testing.T) {
	var receivedID string
	var receivedChildQuestID string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/answers" {
			http.NotFound(writer, request)
			return
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			_ = request.ParseForm()
		}
		receivedID = request.FormValue("id")
		receivedChildQuestID = request.FormValue("child_quest_id")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(
			writer,
			`{"answer":{"id":%q,"quest_id":"thread-id"}}`,
			receivedID,
		)
	}))
	defer server.Close()

	result, err := createReply(
		profileConfig{
			BackendURL:    server.URL,
			AccessToken:   "token",
			Client:        "client",
			UID:           "uid",
			CurrentUserID: "user-id",
			ActiveSpaceID: "space-id",
		},
		replyCreateOptions{
			WriteID:        testWriteID,
			ReplyToQuestID: "thread-id",
			Content:        "safe retry",
		},
	)
	if err != nil {
		t.Fatalf("unexpected create reply error: %v", err)
	}
	if receivedID != testWriteID || result.Answer.ID != testWriteID {
		t.Fatalf("expected answer id %q, received=%q result=%q", testWriteID, receivedID, result.Answer.ID)
	}
	if receivedChildQuestID != "" {
		t.Fatalf("expected backend-generated child quest id, got %q", receivedChildQuestID)
	}
}

func TestCreateRootThreadReturnsGeneratedIDWithRequestError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "temporary failure", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := createRootThread(
		profileConfig{
			BackendURL:    server.URL,
			AccessToken:   "token",
			Client:        "client",
			UID:           "uid",
			ActiveSpaceID: "space-id",
		},
		rootThreadCreateOptions{
			Content: "ambiguous write",
			Private: boolPtr(true),
		},
	)
	if err == nil {
		t.Fatal("expected request error")
	}

	generatedID := writeIDFromTestError(err)
	if !looksLikeUUID(generatedID) {
		t.Fatalf("expected generated write id in error, got %q (%v)", generatedID, err)
	}
}

func TestCreateRootThreadReturnsGeneratedIDWithPreflightError(t *testing.T) {
	_, err := createRootThread(
		profileConfig{ActiveSpaceID: "space-id"},
		rootThreadCreateOptions{
			Content:    "ambiguous local failure",
			Attachment: "/path/that/does/not/exist",
			Private:    boolPtr(true),
		},
	)
	if err == nil {
		t.Fatal("expected attachment preflight error")
	}

	generatedID := writeIDFromTestError(err)
	if !looksLikeUUID(generatedID) {
		t.Fatalf("expected generated write id in preflight error, got %q (%v)", generatedID, err)
	}
}

func TestCreateRootThreadReconcilesConflictWithMatchingWrite(t *testing.T) {
	getCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/quests":
			writer.WriteHeader(http.StatusConflict)
			_, _ = writer.Write([]byte(`{"error":"Thread already exists."}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/quests/"+testWriteID:
			getCount++
			_, _ = fmt.Fprintf(
				writer,
				`{"quest":{"id":%q,"space_id":"space-id","user_id":"user-id","private":true,"public":null,"is_clip":false,"parent":{"id":"root-answer-id","user_id":"user-id","content":"Hello @Treechat","delta_json":{"ops":[{"insert":"Hello "},{"insert":{"mention":{"id":"mentioned-user","value":"Treechat","content":"Treechat","denotationChar":"@"}}}]}}}}`,
				testWriteID,
			)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	result, err := createRootThread(
		profileConfig{
			BackendURL:    server.URL,
			AccessToken:   "token",
			Client:        "client",
			UID:           "uid",
			CurrentUserID: "user-id",
			ActiveSpaceID: "space-id",
		},
		rootThreadCreateOptions{
			WriteID: testWriteID,
			Content: "Hello @Treechat",
			Private: boolPtr(true),
		},
	)
	if err != nil {
		t.Fatalf("expected matching conflict to reconcile, got %v", err)
	}
	if result.Quest.ID != testWriteID || getCount != 1 {
		t.Fatalf("expected reconciled quest %q and one lookup, got id=%q lookups=%d", testWriteID, result.Quest.ID, getCount)
	}
}

func TestCreateReplyReconcilesUnprocessableEntityWithMatchingWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/answers":
			writer.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = writer.Write([]byte(`{"error":"Answer id has already been used."}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/answers/"+testWriteID:
			_, _ = fmt.Fprintf(
				writer,
				`{"answer":{"id":%q,"quest_id":"thread-id","space_id":"space-id","user_id":"user-id","content":"Hello @Treechat","delta_json":{"ops":[{"insert":"Hello "},{"insert":{"mention":{"id":"mentioned-user","value":"Treechat","content":"Treechat","denotationChar":"@"}}}]}}}`,
				testWriteID,
			)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	result, err := createReply(
		profileConfig{
			BackendURL:    server.URL,
			AccessToken:   "token",
			Client:        "client",
			UID:           "uid",
			CurrentUserID: "user-id",
			ActiveSpaceID: "space-id",
		},
		replyCreateOptions{
			WriteID:        testWriteID,
			ReplyToQuestID: "thread-id",
			Content:        "Hello @Treechat",
		},
	)
	if err != nil {
		t.Fatalf("expected matching conflict to reconcile, got %v", err)
	}
	if result.Answer.ID != testWriteID {
		t.Fatalf("expected reconciled answer %q, got %q", testWriteID, result.Answer.ID)
	}
}

func TestCreateReplyRejectsConflictWithDifferentDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			writer.WriteHeader(http.StatusConflict)
			_, _ = writer.Write([]byte(`{"error":"Answer id has already been used."}`))
			return
		}
		_, _ = fmt.Fprintf(
			writer,
			`{"answer":{"id":%q,"quest_id":"thread-id","space_id":"space-id","user_id":"user-id","content":"safe retry","delta_json":{"ops":[{"insert":"different delta"}]}}}`,
			testWriteID,
		)
	}))
	defer server.Close()

	_, err := createReply(
		profileConfig{
			BackendURL:    server.URL,
			AccessToken:   "token",
			Client:        "client",
			UID:           "uid",
			CurrentUserID: "user-id",
			ActiveSpaceID: "space-id",
		},
		replyCreateOptions{
			WriteID:        testWriteID,
			ReplyToQuestID: "thread-id",
			Content:        "safe retry",
			DeltaJSON:      `{"ops":[{"insert":"safe retry"}]}`,
		},
	)
	if err == nil {
		t.Fatal("expected mismatched existing answer to remain an error")
	}
	if writeIDFromTestError(err) != testWriteID {
		t.Fatalf("expected original write id on mismatch, got %v", err)
	}
}

func TestCreateClipQuestReconcilesConflictAndPreservesClipJSONShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/plugin_new/clip":
			writer.WriteHeader(http.StatusConflict)
			_, _ = writer.Write([]byte(`{"error":"Post id has already been used."}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/quests/"+testWriteID:
			_, _ = fmt.Fprintf(
				writer,
				`{"quest":{"id":%q,"space_id":"space-id","user_id":"user-id","private":true,"public":null,"is_clip":true,"parent":{"id":"root-answer-id","user_id":"user-id","content":"safe clip retry","delta_json":{"ops":[{"insert":"safe clip retry"}]},"url":{"address":"https://example.com"}}}}`,
				testWriteID,
			)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	result, err := createClipQuest(
		profileConfig{
			BackendURL:    server.URL,
			AccessToken:   "token",
			Client:        "client",
			UID:           "uid",
			CurrentUserID: "user-id",
			ActiveSpaceID: "space-id",
		},
		"https://example.com",
		"safe clip retry",
		"",
		streamTarget{Kind: "clips", ID: "PSEUDOSTREAM__CLIPS", Name: "Clips"},
		testWriteID,
	)
	if err != nil {
		t.Fatalf("expected matching clip conflict to reconcile, got %v", err)
	}
	if result.Quest.ID != testWriteID {
		t.Fatalf("expected reconciled clip %q, got %q", testWriteID, result.Quest.ID)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(result.Raw, &raw); err != nil {
		t.Fatalf("decoding reconciled clip JSON: %v", err)
	}
	if raw["id"] != testWriteID || raw["quest"] != nil {
		t.Fatalf("expected bare clip JSON response, got %#v", raw)
	}
}

func TestWriteJSONUsesSameStructuredIDOnSuccessAndFailure(t *testing.T) {
	successJSON, err := prettyWriteSuccessJSON(
		json.RawMessage(`{"answer":{"id":"550e8400-e29b-41d4-a716-446655440000"}}`),
		testWriteID,
	)
	if err != nil {
		t.Fatalf("formatting success JSON: %v", err)
	}
	var successPayload map[string]interface{}
	if err := json.Unmarshal([]byte(successJSON), &successPayload); err != nil {
		t.Fatalf("decoding success JSON: %v", err)
	}
	if successPayload["write_id"] != testWriteID {
		t.Fatalf("expected success write_id %q, got %#v", testWriteID, successPayload["write_id"])
	}

	writeErr := withWriteID(testWriteID, fmt.Errorf("request timed out"))
	structuredErr := writeErrorForOutput(fmt.Errorf("creating post: %w", writeErr), "json")
	var output bytes.Buffer
	PrintError(&output, structuredErr)

	var errorPayload map[string]interface{}
	if err := json.Unmarshal(output.Bytes(), &errorPayload); err != nil {
		t.Fatalf("decoding error JSON %q: %v", output.String(), err)
	}
	if errorPayload["status"] != "error" || errorPayload["write_id"] != testWriteID {
		t.Fatalf("unexpected structured error: %#v", errorPayload)
	}
	if errorPayload["error"] != "creating post: request timed out" {
		t.Fatalf("expected error without prose write-id parsing, got %#v", errorPayload["error"])
	}
}

func writeIDFromTestError(err error) string {
	var writeErr *writeOutcomeError
	if errors.As(err, &writeErr) {
		return writeErr.WriteID
	}
	return ""
}
