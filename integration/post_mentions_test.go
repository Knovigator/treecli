package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestCompiledCLIPlainTextPostsOmitDelta(t *testing.T) {
	const threadID = "7a5e85c9-9dca-4140-ba9a-f5db0030afca"
	const content = "Hello @Treechat"
	var writes atomic.Int32
	authHandler := fakeTreechatHandler(t, &fakeBackendState{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/treecli/call" {
			authHandler.ServeHTTP(w, r)
			return
		}
		if !hasIntegrationAuth(r) {
			t.Error("missing authentication")
		}
		var call struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Error(err)
			return
		}
		if call.Name == "quote_treechat_write" {
			a, _ := call.Arguments["arguments"].(map[string]interface{})
			if a["content"] != content || a["delta_json"] != nil {
				t.Errorf("text must remain unchanged without local Delta: %#v", a)
			}
			writeJSON(t, w, map[string]interface{}{"structuredContent": map[string]interface{}{"quote_id": "plain-quote"}})
		} else if call.Name == "create_treechat_write" {
			writes.Add(1)
			writeJSON(t, w, map[string]interface{}{"structuredContent": map[string]interface{}{"operation_id": "plain-quote", "state": "completed", "result": map[string]interface{}{"status": 200, "body": map[string]interface{}{"quest": map[string]string{"id": threadID}, "answer": map[string]string{"id": "answer-1", "quest_id": threadID}}}}})
		} else {
			t.Errorf("unexpected RPC %s", call.Name)
			http.Error(w, "unexpected RPC", 500)
		}

	}))
	defer server.Close()
	binary := buildCLI(t, repositoryRoot(t))
	configHome := filepath.Join(t.TempDir(), "config")
	baseArgs := []string{"--profile", "integration", "--backend-url", server.URL, "--app-host", "https://app.example.test"}
	if stdout, stderr, err := runCLI(t, binary, configHome, integrationPassword+"\n", append(baseArgs,
		"login", "--email", integrationEmail, "--password-stdin")...); err != nil {
		t.Fatalf("login failed: %v\n%s\n%s", err, stdout, stderr)
	}
	for _, args := range [][]string{
		{"new", "post", content, "--json"},
		{"new", "post", content, "--reply-to", threadID, "--json"},
		{"new", "reply", content, "--reply-to", threadID, "--json"},
	} {
		if stdout, stderr, err := runCLI(t, binary, configHome, "", append(baseArgs, args...)...); err != nil {
			t.Fatalf("%v failed: %v\n%s\n%s", args, err, stdout, stderr)
		}
	}
	if writes.Load() != 3 {
		t.Fatalf("got %d writes, want 3", writes.Load())
	}
}
