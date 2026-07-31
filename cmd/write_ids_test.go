package cmd

import (
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

func writeIDFromTestError(err error) string {
	var writeErr *writeOutcomeError
	if errors.As(err, &writeErr) {
		return writeErr.WriteID
	}
	return ""
}
