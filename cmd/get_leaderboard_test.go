package cmd

import (
	"encoding/json"
	"github.com/spf13/viper"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLeaderboardForwardsDateArgumentsToSharedCore(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	oldEnv, oldAccount, oldBackend, oldHost := SelectedEnvironment, SelectedAccount, BackendURLOverride, AppHostOverride
	oldRange, oldStart, oldEnd, oldLimit, oldOutput := leaderboardRange, leaderboardStartDate, leaderboardEndDate, leaderboardLimit, leaderboardOutputFormat
	t.Cleanup(func() {
		SelectedEnvironment, SelectedAccount, BackendURLOverride, AppHostOverride = oldEnv, oldAccount, oldBackend, oldHost
		leaderboardRange, leaderboardStartDate, leaderboardEndDate, leaderboardLimit, leaderboardOutputFormat = oldRange, oldStart, oldEnd, oldLimit, oldOutput
	})
	SelectedEnvironment, SelectedAccount, AppHostOverride = "", "", ""
	for _, name := range []string{"TREECLI_ENV", "TREECLI_ACCOUNT", "TREECLI_BACKEND_URL", "TREECTL_BACKEND_URL", "TREECLI_PROFILE", "TREECTL_PROFILE"} {
		t.Setenv(name, "")
	}
	t.Setenv("TREECLI_ALLOW_INSECURE_HTTP", "1")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var call struct {
			Name      string `json:"name"`
			Arguments struct {
				Operation string                 `json:"operation"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"arguments"`
		}
		json.NewDecoder(r.Body).Decode(&call)
		if call.Name != "treechat_read" || call.Arguments.Operation != "leaderboard" || call.Arguments.Arguments["range"] != "last-week" || call.Arguments.Arguments["start_date"] != "20260525" || call.Arguments.Arguments["end_date"] != "20260601" || call.Arguments.Arguments["limit"] != float64(7) {
			t.Errorf("lost inputs %#v", call)
		}
		w.Write([]byte(`{"structuredContent":{"status":200,"body":{"items":[],"period":{}}}}`))
	}))
	defer server.Close()
	BackendURLOverride = server.URL
	viper.Set("active_profile", "prod")
	viper.Set("profiles.prod.backend_url", server.URL)
	viper.Set("profiles.prod.access_token", "test-token")
	viper.Set("profiles.prod.client", "test-client")
	viper.Set("profiles.prod.uid", "test-uid")
	leaderboardRange, leaderboardStartDate, leaderboardEndDate, leaderboardLimit, leaderboardOutputFormat = "last-week", "20260525", "20260601", 7, "json"
	if err := runGetLeaderboardUpvalues(getLeaderboardUpvaluesCmd, nil); err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
