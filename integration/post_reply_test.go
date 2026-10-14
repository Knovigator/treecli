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

func TestCompiledPostReplies(t *testing.T) {
	const target = "550e8400-e29b-41d4-a716-446655440001"
	const writeID = "550e8400-e29b-41d4-a716-446655440002"
	binary := buildCLI(t, repositoryRoot(t))
	for _, tc := range []struct {
		name, command, destination, quote, failure string
		noQuote                                    bool
	}{
		{name: "uppercase post ID", command: "branch-reply", destination: "branch", quote: target},
		{name: "branch title quote", command: "branch-reply", destination: "branch", quote: target},
		{name: "branch without quote", command: "branch-reply", destination: "branch", noQuote: true},
		{name: "containing thread quote", command: "quote-reply", destination: "main", quote: target},
		{name: "canonical discussion among children", command: "branch-reply", destination: "branch", quote: target, failure: "multiple-canonical"},
		{name: "ambiguous children", command: "branch-reply", failure: "ambiguous"},
		{name: "missing children", command: "branch-reply", failure: "no-child"},
		{name: "wrong child parent", command: "branch-reply", failure: "wrong-parent"},
		{name: "root has no containing thread", command: "quote-reply", failure: "root"},
		{name: "inaccessible target", command: "branch-reply", failure: "forbidden"},
		{name: "backend drops quote", command: "quote-reply", failure: "drops-quote"},
		{name: "write denied", command: "quote-reply", failure: "write-denied"},
		{name: "invalid output", command: "branch-reply", failure: "output"},
		{name: "quote rejects no quote", command: "quote-reply", noQuote: true, failure: "flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			fallback := fakeTreechatHandler(t, &fakeBackendState{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/treecli/call" {
					fallback.ServeHTTP(w, r)
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
				switch call.Name {
				case "quote_treechat_write":
					a, _ := call.Arguments["arguments"].(map[string]interface{})
					operation := strings.ReplaceAll(tc.command, "-", "_")
					source := "https://app.example.test/answer/" + target
					if tc.name == "uppercase post ID" {
						source = strings.ToUpper(target)
					}
					if call.Arguments["operation"] != operation || a["source_post_id"] != source || a["content"] != "hello" || a["id"] != writeID || a["no_quote"] != tc.noQuote {
						t.Errorf("wrong shared arguments: %#v", call.Arguments)
					}
					if tc.failure != "" && tc.failure != "multiple-canonical" && tc.failure != "write-denied" && tc.failure != "drops-quote" {
						http.Error(w, "backend rejected reply destination", 422)
						return
					}
					writeJSON(t, w, map[string]interface{}{"structuredContent": map[string]interface{}{"quote_id": "reply-quote"}})
				case "create_treechat_write":
					posts++
					if call.Arguments["quote_id"] != "reply-quote" || call.Arguments["idempotency_key"] != writeID {
						t.Errorf("wrong execute: %#v", call.Arguments)
					}
					if tc.failure == "write-denied" {
						http.Error(w, "denied", 403)
						return
					}
					if tc.failure == "drops-quote" {
						http.Error(w, "write may already exist", 409)
						return
					}
					fmt.Fprintf(w, `{"structuredContent":{"operation_id":"reply-quote","state":"completed","result":{"status":200,"body":{"answer":{"id":%q,"quest_id":%q,"reply_to_answer_id":%q,"content":"hello"}}}}}`, writeID, tc.destination, tc.quote)
				default:
					t.Errorf("unexpected RPC %s", call.Name)
					http.Error(w, "unexpected RPC", 500)
				}
			}))
			defer server.Close()
			config := filepath.Join(t.TempDir(), "config")
			base := []string{"--profile", "integration", "--backend-url", server.URL, "--app-host", "https://app.example.test"}
			out, stderr, err := runCLI(t, binary, config, integrationPassword+"\n", append(base, "login", "--email", integrationEmail, "--password-stdin")...)
			if err != nil {
				t.Fatalf("login: %v %s %s", err, out, stderr)
			}
			args := append(base, tc.command, "https://app.example.test/answer/"+target, "hello", "--id", writeID, "--json")
			if tc.name == "uppercase post ID" {
				args[len(base)+1] = strings.ToUpper(target)
			}
			if tc.noQuote {
				args = append(args, "--no-quote")
			}
			if tc.failure == "output" {
				args = args[:len(args)-1]
				args = append(args, "--output", "bogus")
			}
			out, stderr, err = runCLI(t, binary, config, "", args...)
			success := tc.failure == "" || tc.failure == "multiple-canonical"
			if success {
				if err != nil {
					t.Fatalf("reply: %v %s %s", err, out, stderr)
				}
				var result struct {
					Answer struct {
						ID      string `json:"id"`
						QuestID string `json:"quest_id"`
						Quote   string `json:"reply_to_answer_id"`
					} `json:"answer"`
				}
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatal(err)
				}
				if result.Answer.ID != writeID || result.Answer.QuestID != tc.destination || result.Answer.Quote != tc.quote {
					t.Fatalf("wrong result: %s", out)
				}
				if posts != 1 {
					t.Fatalf("got %d writes", posts)
				}
			} else {
				if err == nil {
					t.Fatalf("expected failure: %s", out)
				}
				expected := 0
				if tc.failure == "drops-quote" || tc.failure == "write-denied" {
					expected = 1
				}
				if posts != expected {
					t.Fatalf("unexpected writes: %d", posts)
				}
				if tc.failure == "drops-quote" && !strings.Contains(stderr, "may already exist") {
					t.Fatalf("missing uncertain-write warning: %s", stderr)
				}
			}
		})
	}
}
