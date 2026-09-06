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
				switch r.URL.Path {
				case "/api/v1/answers/" + target:
					if tc.failure == "forbidden" {
						http.Error(w, "denied", 403)
						return
					}
					children := `[{"id":"branch"}]`
					quest := "main"
					if tc.failure == "no-child" {
						children = `[]`
					}
					if tc.failure == "ambiguous" || tc.failure == "multiple-canonical" {
						children = `[{"id":"other"},{"id":"branch"},{"id":"branch"}]`
					}
					if tc.failure == "root" {
						quest = ""
					}
					fmt.Fprintf(w, `{"answer":{"id":%q,"quest_id":%q,"child_quests":%s}}`, target, quest, children)
				case "/api/v1/quests/branch", "/api/v1/quests/main", "/api/v1/quests/other":
					parent := target
					if tc.failure == "wrong-parent" {
						parent = "another-post"
					}
					id := strings.TrimPrefix(r.URL.Path, "/api/v1/quests/")
					fmt.Fprintf(w, `{"quest":{"id":%q,"parent_id":%q,"space_id":"space-1","side_quest":%t}}`, id, parent, tc.failure == "multiple-canonical" && id == "branch")
				case "/api/v1/answers":
					posts++
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.Header.Get("access-token") == "" {
						t.Error("missing auth")
					}
					quote := r.FormValue("reply_to_answer_id")
					if tc.failure == "" || tc.failure == "multiple-canonical" {
						if r.FormValue("quest_id") != tc.destination || quote != tc.quote || r.FormValue("id") != writeID || r.FormValue("content") != "hello" {
							t.Errorf("wrong payload: %v", r.PostForm)
						}
						if tc.noQuote {
							if _, exists := r.PostForm["reply_to_answer_id"]; exists {
								t.Error("no-quote must omit the edge")
							}
						}
					}
					if tc.failure == "write-denied" {
						http.Error(w, "denied", 403)
						return
					}
					if tc.failure == "drops-quote" {
						quote = ""
					}
					fmt.Fprintf(w, `{"answer":{"id":%q,"quest_id":%q,"reply_to_answer_id":%q,"content":"hello"}}`, writeID, r.FormValue("quest_id"), quote)
				default:
					fallback.ServeHTTP(w, r)
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
