package integration_test

import (
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
		if r.Method != http.MethodPost || (r.URL.Path != "/api/v1/quests" && r.URL.Path != "/api/v1/answers") {
			authHandler.ServeHTTP(w, r)
			return
		}
		writes.Add(1)
		if !hasIntegrationAuth(r) {
			t.Error("post is missing authentication")
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse post form: %v", err)
		}
		contentKey, deltaKey := "parent_attributes[content]", "parent_attributes[delta_json]"
		if r.URL.Path == "/api/v1/answers" {
			contentKey, deltaKey = "content", "delta_json"
			if r.FormValue("quest_id") != threadID {
				t.Errorf("reply target = %q", r.FormValue("quest_id"))
			}
		}
		if r.FormValue(contentKey) != content || r.PostForm.Has(deltaKey) {
			t.Errorf("expected unchanged text and omitted Delta, got %#v", r.PostForm)
		}
		writeJSON(t, w, map[string]interface{}{
			"quest":  map[string]string{"id": threadID},
			"answer": map[string]string{"id": "answer-1", "quest_id": threadID},
		})
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
