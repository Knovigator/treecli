package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPostDeltaRequestContract(t *testing.T) {
	for _, reply := range []bool{false, true} {
		name := "root"
		if reply {
			name = "reply"
		}
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				name  string
				delta string
			}{
				{name: "plain text delegates parsing to backend"},
				{name: "explicit action Delta is preserved", delta: `{"ops":[{"insert":"!flux","attributes":{"bold":true}},{"insert":" Hello @Treechat"}]}`},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var called atomic.Bool
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/api/v1/treecli/call" || r.Method != "POST" {
							t.Errorf("unexpected RPC %s %s", r.Method, r.URL)
						}
						var call struct {
							Name      string                 `json:"name"`
							Arguments map[string]interface{} `json:"arguments"`
						}
						if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
							t.Fatal(err)
						}
						if call.Name == "create_treechat_write" {
							json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": map[string]interface{}{"operation_id": "q", "state": "completed", "result": map[string]interface{}{"status": 200, "body": map[string]interface{}{"quest": map[string]string{"id": "quest-1"}, "answer": map[string]string{"id": "answer-1"}}}}})
							return
						}
						called.Store(true)
						if call.Name != "quote_treechat_write" {
							t.Errorf("unexpected tool %s", call.Name)
						}
						fields, _ := call.Arguments["arguments"].(map[string]interface{})
						if fields["content"] != "Hello @Treechat" {
							t.Errorf("lost content %#v", fields)
						}
						if tc.delta == "" {
							if _, ok := fields["delta_json"]; ok {
								t.Error("plain text must omit delta_json")
							}
						} else if fields["delta_json"] != tc.delta {
							t.Errorf("lost delta %#v", fields)
						}
						if reply && fields["quest_id"] != "quest-1" {
							t.Errorf("lost reply destination %#v", fields)
						}
						json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": map[string]string{"quote_id": "q"}})

					}))
					defer server.Close()
					profile := profileConfig{BackendURL: server.URL, AccessToken: "t", Client: "c", UID: "u", ActiveSpaceID: "space-1"}
					var err error
					if reply {
						_, err = createReply(profile, replyCreateOptions{
							ReplyToQuestID: "quest-1", Content: "Hello @Treechat", DeltaJSON: tc.delta,
						})
					} else {
						_, err = createRootThread(profile, rootThreadCreateOptions{
							Content: "Hello @Treechat", DeltaJSON: tc.delta,
						})
					}
					if err != nil {
						t.Fatal(err)
					}
					if !called.Load() {
						t.Fatal("post request was not sent")
					}
				})
			}
		})
	}
}
