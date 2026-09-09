package cmd

import (
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
						called.Store(true)
						path := "/api/v1/quests"
						contentKey, deltaKey := "parent_attributes[content]", "parent_attributes[delta_json]"
						if reply {
							path, contentKey, deltaKey = "/api/v1/answers", "content", "delta_json"
						}
						if r.Method != http.MethodPost || r.URL.Path != path {
							t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
						}
						if err := r.ParseForm(); err != nil {
							t.Errorf("parse form: %v", err)
						}
						if got := r.FormValue(contentKey); got != "Hello @Treechat" {
							t.Errorf("content = %q", got)
						}
						if got := r.FormValue(deltaKey); got != tc.delta {
							t.Errorf("Delta = %q, want %q", got, tc.delta)
						}
						if tc.delta == "" && r.PostForm.Has(deltaKey) {
							t.Error("plain text must omit delta_json entirely")
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"quest":{"id":"quest-1"},"answer":{"id":"answer-1"}}`))
					}))
					defer server.Close()
					profile := profileConfig{BackendURL: server.URL, ActiveSpaceID: "space-1"}
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
