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
		if r.URL.Path != "/api/v1/treecli/call" {
			fallback.ServeHTTP(w, r)
			return
		}
		if !hasIntegrationAuth(r) {
			t.Error("missing auth")
		}
		var call struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Error(err)
			return
		}
		if call.Name != "treechat_read" {
			t.Errorf("unexpected tool: %s", call.Name)
		}
		args, _ := call.Arguments["arguments"].(map[string]interface{})
		if call.Arguments["operation"] == "thread" {
			if args["id"] != "q1" {
				t.Errorf("wrong ID: %#v", args)
			}
			fmt.Fprint(w, `{"structuredContent":{"quest":{"id":"q1","future_field":true}}}`)
		} else {
			if args["user"] == "alice" && args["thread_scope"] != "branch" {
				t.Errorf("missing branch scope: %#v", args)
			}
			fmt.Fprint(w, `{"structuredContent":{"threads":[{"id":"newest","future_field":true}],"pagination":{"page":1,"limit":10,"has_more":false}}}`)
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
