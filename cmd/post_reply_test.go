package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReplyDestinationRejectsInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name, post, thread string
		status             int
		branch             bool
	}{
		{"missing post", `{"answer":{}}`, `{}`, 200, true},
		{"invalid post JSON", `{`, `{}`, 200, true},
		{"wrong containing thread", `{"answer":{"id":"post","quest_id":"room"}}`, `{"quest":{"id":"wrong"}}`, 200, false},
		{"inaccessible thread", `{"answer":{"id":"post","child_quests":[{"id":"room"}]}}`, `{}`, 403, true},
		{"multiple discussion branches", `{"answer":{"id":"post","child_quests":[{"id":"a"},{"id":"b"}]}}`, ``, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/answers/post" {
					fmt.Fprint(w, tc.post)
					return
				}
				w.WriteHeader(tc.status)
				if tc.thread != "" {
					fmt.Fprint(w, tc.thread)
					return
				}
				fmt.Fprintf(w, `{"quest":{"id":%q,"parent_id":"post","side_quest":true}}`, r.URL.Path[len("/api/v1/quests/"):])
			}))
			defer server.Close()
			_, err := resolvePostReplyDestination(profileConfig{BackendURL: server.URL}, "post", tc.branch)
			if err == nil {
				t.Fatal("expected invalid destination to fail")
			}
		})
	}
}
