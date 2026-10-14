package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestSharedNativeWriteQuotesExactArgumentsAndReusesID(t *testing.T) {
	for _, operation := range []string{"post", "reply", "clip", "action"} {
		t.Run(operation, func(t *testing.T) {
			calls := []string{}
			args := map[string]interface{}{"id": testWriteID, "content": "hello @Treechat", "stream": "Team Name", "source_post_id": "https://app.example/answer/ABC", "no_quote": true, "delta_json": "{\"ops\":[]}"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/api/v1/treecli/call" || r.Header.Get("access-token") != "token" {
					t.Errorf("incorrect RPC: %s %s", r.Method, r.URL)
				}
				var call struct {
					Name      string                 `json:"name"`
					Arguments map[string]interface{} `json:"arguments"`
				}
				if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
					t.Fatal(err)
				}
				calls = append(calls, call.Name)
				var result interface{}
				switch call.Name {
				case "quote_treechat_write":
					if call.Arguments["operation"] != operation || call.Arguments["max_price_cents"] != float64(123) || !reflect.DeepEqual(call.Arguments["arguments"], args) {
						t.Errorf("lost quote input %#v", call.Arguments)
					}
					result = map[string]interface{}{"quote_id": "server-quote"}
				case "create_treechat_write":
					if call.Arguments["quote_id"] != "server-quote" || call.Arguments["idempotency_key"] != testWriteID {
						t.Errorf("lost quote/retry ID %#v", call.Arguments)
					}
					result = map[string]interface{}{"operation_id": "server-quote", "state": "completed", "result": map[string]interface{}{"status": 200, "body": map[string]interface{}{"future": "preserved"}}}
				default:
					t.Fatalf("unexpected RPC %s", call.Name)
				}
				json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": result})
			}))
			defer server.Close()
			var output map[string]interface{}
			err := sharedNativeWrite(profileConfig{BackendURL: server.URL, AccessToken: "token", Client: "client", UID: "uid"}, operation, args, "", 123, &output)
			if err != nil || output["future"] != "preserved" || !reflect.DeepEqual(calls, []string{"quote_treechat_write", "create_treechat_write"}) {
				t.Fatalf("calls=%v output=%v err=%v", calls, output, err)
			}
		})
	}
}
func TestSharedWriteStopsOnQuoteFailureAndPreservesRetryID(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"isError":true,"structuredContent":{"status":409,"body":{"error":"destination mismatch"}}}`))
	}))
	defer server.Close()
	var out map[string]interface{}
	err := sharedNativeWrite(profileConfig{BackendURL: server.URL, AccessToken: "test-token", Client: "test-client", UID: "test-uid"}, "post", map[string]interface{}{"id": testWriteID}, "", 0, &out)
	if err == nil || calls != 1 || writeIDFromTestError(err) != testWriteID || !strings.Contains(err.Error(), "destination mismatch") {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
func TestSharedWritePropagatesBackendStates(t *testing.T) {
	for _, state := range []string{"rejected", "pending", "processing", "unknown"} {
		t.Run(state, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					w.Write([]byte(`{"structuredContent":{"quote_id":"q"}}`))
					return
				}
				json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": map[string]interface{}{"operation_id": "q", "state": state, "result": map[string]interface{}{"status": 409, "body": map[string]interface{}{"error": "conflicting write"}}}})
			}))
			defer server.Close()
			var out map[string]interface{}
			err := sharedNativeWrite(profileConfig{BackendURL: server.URL, AccessToken: "test-token", Client: "test-client", UID: "test-uid"}, "reply", map[string]interface{}{"id": testWriteID}, "", 0, &out)
			if err == nil || calls != 2 || writeIDFromTestError(err) != testWriteID {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestSharedWriteRejectsInvalidQuoteAndResultEnvelopes(t *testing.T) {
	for _, tc := range []struct {
		name, quote, result string
		wantCalls           int
	}{
		{"missing quote", `{}`, `{}`, 1},
		{"mismatched operation", `{"quote_id":"q"}`, `{"operation_id":"wrong","state":"completed","result":{"status":200,"body":{}}}`, 2},
		{"completed error", `{"quote_id":"q"}`, `{"operation_id":"q","state":"completed","result":{"status":403,"body":{"error":"denied"}}}`, 2},
		{"malformed result", `{"quote_id":"q"}`, `{"operation_id":"q","state":"completed","result":{"status":200,"body":[]}}`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body := tc.quote
				if calls == 2 {
					body = tc.result
				}
				w.Write([]byte(`{"structuredContent":` + body + `}`))
			}))
			defer server.Close()
			var out map[string]interface{}
			err := sharedNativeWrite(profileConfig{BackendURL: server.URL, AccessToken: "test-token", Client: "test-client", UID: "test-uid"}, "post", map[string]interface{}{"id": testWriteID}, "", 0, &out)
			if err == nil || calls != tc.wantCalls || writeIDFromTestError(err) != testWriteID {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
