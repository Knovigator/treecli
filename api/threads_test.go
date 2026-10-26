package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Resolution, visibility and child de-duplication are backend contracts. These
// adapter tests require a single RPC and preserve its complete response.
func TestThreadsSharedContract(t *testing.T) {
	for _, answer := range []bool{false, true} {
		t.Run(map[bool]string{false: "user", true: "answer"}[answer], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/api/v1/treecli/call" || r.Header.Get("access-token") != "t" {
					t.Errorf("incorrect RPC: %s %s", r.Method, r.URL)
				}
				var call struct {
					Name      string `json:"name"`
					Arguments struct {
						Operation string                 `json:"operation"`
						Arguments map[string]interface{} `json:"arguments"`
					} `json:"arguments"`
				}
				if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
					t.Fatal(err)
				}
				if call.Name != "treechat_read" || call.Arguments.Operation != "threads" {
					t.Fatalf("unexpected call %#v", call)
				}
				args := call.Arguments.Arguments
				if answer {
					if args["answer"] != "https://app.example/answer/A" {
						t.Errorf("lost answer reference: %#v", args)
					}
				} else if args["user"] != "@alice" || args["thread_scope"] != "root" || args["page"] != float64(2) || args["limit"] != float64(1) {
					t.Errorf("lost options: %#v", args)
				}
				json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": map[string]interface{}{"status": 200, "body": json.RawMessage(`{"threads":[{"id":"q1","future_field":true}],"pagination":{"page":2,"limit":1,"next_page":3,"has_more":true}}`)}})
			}))
			defer server.Close()
			var result ThreadsResponse
			var err error
			if answer {
				result, err = ThreadsForAnswer(server.URL, "t", "c", "u", "https://app.example/answer/A")
			} else {
				result, err = ListThreads(server.URL, "t", "c", "u", ThreadListOptions{User: "@alice", Scope: "root", Page: 2, Limit: 1})
			}
			if err != nil || calls != 1 || len(result.Threads) != 1 || !strings.Contains(string(result.Raw), "future_field") {
				t.Fatalf("result=%#v calls=%d err=%v", result, calls, err)
			}
		})
	}
}
func TestThreadsPropagatesSharedErrors(t *testing.T) {
	for _, body := range []string{`{"isError":true,"structuredContent":{"status":403,"body":{"error":"denied"}}}`, `{"structuredContent":{"status":200,"body":"invalid threads"}}`, `{`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		_, err := ListThreads(server.URL, "t", "c", "u", ThreadListOptions{User: "me"})
		server.Close()
		if err == nil {
			t.Fatalf("accepted invalid shared result %s", body)
		}
	}
}

func TestListPublicThreadsSendsFeedArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Arguments struct {
				Operation string                 `json:"operation"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Fatal(err)
		}
		args := call.Arguments.Arguments
		if call.Arguments.Operation != "threads" || args["public"] != true || args["order"] != "time" || args["limit"] != float64(5) || args["before"] != "cur" {
			t.Errorf("wrong feed call: %#v", call)
		}
		if _, ok := args["user"]; ok {
			t.Errorf("public feed must not send user: %#v", args)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": map[string]interface{}{"status": 200, "body": json.RawMessage(`{"threads":[{"id":"q1"}],"pagination":{"limit":5,"next_cursor":"nxt","has_more":true}}`)}})
	}))
	defer server.Close()
	result, err := ListThreads(server.URL, "t", "c", "u", ThreadListOptions{Public: true, Order: "time", Limit: 5, Before: "cur"})
	if err != nil || len(result.Threads) != 1 || result.Pagination == nil || *result.Pagination.NextCursor != "nxt" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
