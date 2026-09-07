package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestQuestReconciliationRequiresMatchingDestination(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target streamTarget
		stored string
		clip   bool
		wantOK bool
	}{
		{"private", defaultPrivateStreamTarget(), `"private":true,"public":null,"is_clip":false`, false, true},
		{"public", streamTarget{Kind: "public"}, `"public":true,"private":null,"is_clip":false`, false, true},
		{"public instead of private", defaultPrivateStreamTarget(), `"public":true,"private":false,"is_clip":false`, false, false},
		{"private instead of public", streamTarget{Kind: "public"}, `"private":true,"public":false,"is_clip":false`, false, false},
		{"missing visibility", defaultPrivateStreamTarget(), `"is_clip":false`, false, false},
		{"team", streamTarget{Kind: "team", ID: "650e8400-e29b-41d4-a716-446655440000"}, `"team_id":"650e8400-e29b-41d4-a716-446655440000","is_clip":false`, false, true},
		{"wrong team", streamTarget{Kind: "team", ID: "650e8400-e29b-41d4-a716-446655440000"}, `"team_id":"750e8400-e29b-41d4-a716-446655440000","is_clip":false`, false, false},
		{"clip", streamTarget{Kind: "clips"}, `"private":true,"public":false,"is_clip":true`, true, true},
		{"clip wrong visibility", streamTarget{Kind: "clips"}, `"private":false,"public":true,"is_clip":true`, true, false},
		{"clip instead of post", defaultPrivateStreamTarget(), `"private":true,"is_clip":true`, false, false},
		{"post instead of clip", streamTarget{Kind: "clips"}, `"private":true,"is_clip":false`, true, false},
		{"unexpected URL", defaultPrivateStreamTarget(), `"private":true,"is_clip":false`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parentURL := ""
			if tc.name == "unexpected URL" {
				parentURL = `,"url":{"address":"https://example.com"}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"error":"duplicate"}`))
					return
				}
				_, _ = fmt.Fprintf(w, `{"quest":{"id":%q,"user_id":"user-id","space_id":"space-id",%s,"parent":{"content":"retry","delta_json":{"ops":[{"insert":"retry"}]}%s}}}`, testWriteID, tc.stored, parentURL)
			}))
			defer server.Close()
			profile := profileConfig{BackendURL: server.URL, CurrentUserID: "user-id", ActiveSpaceID: "space-id"}
			var err error
			if tc.clip {
				_, err = createClipQuest(profile, "", "retry", "", tc.target, testWriteID)
			} else {
				options := rootThreadCreateOptions{WriteID: testWriteID, Content: "retry"}
				switch tc.target.Kind {
				case "private":
					options.Private = boolPtr(true)
				case "public":
					options.Public = boolPtr(true)
				case "team":
					options.TeamID = tc.target.ID
				}
				_, err = createRootThread(profile, options)
			}
			if (err == nil) != tc.wantOK {
				t.Fatalf("success=%v, want %v: %v", err == nil, tc.wantOK, err)
			}
			if err != nil && writeIDFromTestError(err) != testWriteID {
				t.Fatalf("lost retry ID: %v", err)
			}
		})
	}
}

func TestReconciliationRejectsUnverifiedPayloads(t *testing.T) {
	attachment := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(attachment, []byte("new attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"post", "reply", "clip"} {
		for _, payload := range []string{"attachment", "action", "message type", "thread type"} {
			if (kind == "clip" && payload != "attachment") || (kind == "reply" && payload == "thread type") {
				continue
			}
			t.Run(kind+"/"+payload, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodPost {
						w.WriteHeader(http.StatusConflict)
						_, _ = w.Write([]byte(`{"error":"duplicate"}`))
						return
					}
					answer := map[string]interface{}{
						"id": testWriteID, "user_id": "user-id", "quest_id": "thread-id", "space_id": "space-id",
						"content": "retry", "delta_json": map[string]interface{}{"ops": []map[string]string{{"insert": "retry"}}},
					}
					if kind == "reply" {
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"answer": answer})
					} else {
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"quest": map[string]interface{}{
							"id": testWriteID, "user_id": "user-id", "space_id": "space-id", "private": true,
							"is_clip": kind == "clip", "parent": answer,
						}})
					}
				}))
				defer server.Close()
				profile := profileConfig{BackendURL: server.URL, CurrentUserID: "user-id", ActiveSpaceID: "space-id"}
				root := rootThreadCreateOptions{WriteID: testWriteID, Content: "retry", Private: boolPtr(true)}
				reply := replyCreateOptions{WriteID: testWriteID, Content: "retry", ReplyToQuestID: "thread-id"}
				switch payload {
				case "attachment":
					root.Attachment, reply.Attachment = attachment, attachment
				case "action":
					root.ActionRequestsJSON, reply.ActionRequestsJSON = `[{"model":"new-model"}]`, `[{"model":"new-model"}]`
				case "message type":
					root.MessageType, reply.MessageType = "agent", "agent"
				case "thread type":
					root.ThreadType = "new-type"
				}
				var err error
				switch kind {
				case "post":
					_, err = createRootThread(profile, root)
				case "reply":
					_, err = createReply(profile, reply)
				case "clip":
					_, err = createClipQuest(profile, "", "retry", attachment, streamTarget{Kind: "clips"}, testWriteID)
				}
				if err == nil || writeIDFromTestError(err) != testWriteID {
					t.Fatalf("expected original error and retry ID for unverified %s, got %v", payload, err)
				}
			})
		}
	}
}

func TestWriteSuccessJSONRejectsNullResponse(t *testing.T) {
	if _, err := prettyWriteSuccessJSON(json.RawMessage(`null`), testWriteID); err == nil {
		t.Fatal("expected an error for a null write response")
	}
}
