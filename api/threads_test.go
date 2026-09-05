package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListThreadsContract(t *testing.T) {
	var resolved, listed int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("access-token") != "token" || r.Header.Get("client") != "client" || r.Header.Get("uid") != "uid" {
			t.Errorf("incorrect authenticated request: %s %s", r.Method, r.URL)
		}
		switch r.URL.Path {
		case "/api/v1/users/name/alice":
			resolved++
			fmt.Fprint(w, `{"user":{"id":"123"}}`)
		case "/api/v1/quests":
			listed++
			q := r.URL.Query()
			for key, want := range map[string]string{"authored": "true", "user": "123", "thread_scope": "root", "page": "2", "limit": "1"} {
				if q.Get(key) != want {
					t.Errorf("%s = %q, want %q", key, q.Get(key), want)
				}
			}
			fmt.Fprint(w, `{"threads":[{"id":"q1","future_field":true}],"pagination":{"page":2,"limit":1,"next_page":3,"has_more":true}}`)
		default:
			t.Errorf("unexpected path %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	for _, user := range []string{"alice", "@alice", "123"} {
		result, err := ListThreads(server.URL, "token", "client", "uid", ThreadListOptions{User: user, Scope: "root", Page: 2, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Threads) != 1 || result.Threads[0].ID != "q1" || !result.Pagination.HasMore || *result.Pagination.NextPage != 3 || !strings.Contains(string(result.Raw), "future_field") {
			t.Fatalf("unexpected result %#v", result)
		}
	}
	if resolved != 2 || listed != 3 {
		t.Fatalf("requests resolved=%d listed=%d", resolved, listed)
	}
}

func TestListThreadsErrorsAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"old backend", `{"quests":[]}`, 200, "backend does not support"},
		{"malformed", `{`, 200, "parsing threads"},
		{"forbidden", `{"error":"denied"}`, 403, "403"},
		{"empty", `{"threads":[],"pagination":{"page":1,"limit":20,"next_page":null,"has_more":false}}`, 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("user") != "me" {
					t.Errorf("me not passed to server")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			result, err := ListThreads(server.URL, "t", "c", "u", ThreadListOptions{User: "me", Scope: "all", Page: 1, Limit: 20})
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v", err)
				}
			} else if err != nil || result.Threads == nil {
				t.Fatalf("empty list: %#v %v", result, err)
			}
		})
	}
}

func TestThreadsForAnswer(t *testing.T) {
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		switch r.URL.Path {
		case "/api/v1/answers/a1":
			fmt.Fprint(w, `{"answer":{"id":"a1","child_quests":[{"id":"q1"},{"id":"q1"},{"id":"hidden"},{"id":"gone"},{"id":"q2"}]}}`)
		case "/api/v1/answers/empty":
			fmt.Fprint(w, `{"answer":{"id":"empty","child_quests":[]}}`)
		case "/api/v1/quests/q1", "/api/v1/quests/q2":
			fmt.Fprintf(w, `{"quest":{"id":%q,"future_field":"preserved"}}`, strings.TrimPrefix(r.URL.Path, "/api/v1/quests/"))
		case "/api/v1/quests/hidden":
			w.WriteHeader(403)
		case "/api/v1/quests/gone":
			w.WriteHeader(404)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	result, err := ThreadsForAnswer(server.URL, "t", "c", "u", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Threads) != 2 || calls["/api/v1/quests/q1"] != 1 || !strings.Contains(string(result.Raw), "future_field") {
		t.Fatalf("unexpected children %s", result.Raw)
	}
	result, err = ThreadsForAnswer(server.URL, "t", "c", "u", "empty")
	if err != nil || string(result.Raw) != `{"threads":[]}` {
		t.Fatalf("empty: %s %v", result.Raw, err)
	}
}

func TestThreadUserResolutionErrors(t *testing.T) {
	for _, body := range []string{`{`, `{"user":{}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		_, err := ResolveThreadUser(server.URL, "t", "c", "u", "alice")
		server.Close()
		if err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestThreadsForAnswerRejectsFailedOrMalformedChildren(t *testing.T) {
	for _, tc := range []struct {
		name, answer, child string
		status              int
		want                string
	}{
		{"missing answer", `{}`, ``, 200, "answer response is missing"},
		{"failed child", `{"answer":{"id":"a","child_quests":[{"id":"q"}]}}`, `server error`, 500, "500"},
		{"malformed child", `{"answer":{"id":"a","child_quests":[{"id":"q"}]}}`, `{`, 200, "parsing child"},
		{"missing child", `{"answer":{"id":"a","child_quests":[{"id":"q"}]}}`, `{"quest":{}}`, 200, "missing an ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/v1/answers/") {
					fmt.Fprint(w, tc.answer)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.child)
			}))
			defer server.Close()
			_, err := ThreadsForAnswer(server.URL, "t", "c", "u", "a")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}
