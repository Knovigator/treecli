package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompiledThreadsCommands(t *testing.T) {
	fallback := fakeTreechatHandler(t, &fakeBackendState{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/users/name/alice":
			fmt.Fprint(w, `{"user":{"id":"123"}}`)
		case "/api/v1/quests":
			if r.URL.Query().Get("authored") != "true" {
				t.Error("missing authored mode")
			}
			page := r.URL.Query().Get("page")
			limit := r.URL.Query().Get("limit")
			if r.URL.Query().Get("user") == "123" && r.URL.Query().Get("thread_scope") != "branch" {
				t.Error("missing branch filter")
			}
			fmt.Fprintf(w, `{"threads":[{"id":"newest","future_field":true}],"pagination":{"page":%s,"limit":%s,"next_page":null,"has_more":false}}`, page, limit)
		case "/api/v1/quests/q1":
			fmt.Fprint(w, `{"quest":{"id":"q1","future_field":true}}`)
		case "/api/v1/answers/a1":
			fmt.Fprint(w, `{"answer":{"id":"a1","child_quests":[{"id":"q1"}]}}`)
		default:
			fallback.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	binary := buildCLI(t, repositoryRoot(t))
	config := filepath.Join(t.TempDir(), "config")
	base := []string{"--profile", "integration", "--backend-url", server.URL, "--app-host", "https://app.example.test"}
	out, stderr, err := runCLI(t, binary, config, integrationPassword+"\n", append(base, "login", "--email", integrationEmail, "--password-stdin")...)
	if err != nil {
		t.Fatalf("login %v %s %s", err, out, stderr)
	}
	for _, tc := range []struct {
		name       string
		args       []string
		key        string
		deprecated bool
	}{
		{"newest", []string{"get", "threads", "--root", "--limit", "1", "--json"}, "threads", false},
		{"author branches page", []string{"get", "threads", "--user", "alice", "--branch", "--page", "2", "--limit", "10", "--json"}, "threads", false},
		{"thread ID", []string{"get", "threads", "q1", "--json"}, "quest", false},
		{"answer children", []string{"get", "threads", "--answer", "a1", "--json"}, "threads", false},
		{"deprecated singular", []string{"get", "thread", "q1", "--json"}, "quest", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, stderr, err := runCLI(t, binary, config, "", append(base, tc.args...)...)
			if err != nil {
				t.Fatalf("%v %s %s", err, out, stderr)
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatalf("invalid JSON %s: %v", out, err)
			}
			if result[tc.key] == nil || !strings.Contains(out, "future_field") {
				t.Fatalf("lost payload: %s", out)
			}
			if strings.Contains(stderr, "get thread is deprecated") != tc.deprecated {
				t.Fatalf("unexpected stderr %q", stderr)
			}
		})
	}
}
