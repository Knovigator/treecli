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
