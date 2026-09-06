package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCompiledWhoami(t *testing.T) {
	binary := buildCLI(t, repositoryRoot(t))
	var mode atomic.Value
	mode.Store("success")
	var calls, redirectCalls atomic.Int32
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectCalls.Add(1) }))
	defer trap.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/api/v1/gon" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("access-token") != "whoami-secret-token" || r.Header.Get("client") != "whoami-secret-client" || r.Header.Get("uid") != "bot@example.test" {
			t.Error("missing authentication headers")
		}
		switch mode.Load().(string) {
		case "success":
			fmt.Fprint(w, `{"currentUser":{"id":"live-user-id","name":"Brooz_GM_Tip_Bot","email":"private@example.test","tokens":"secret-server-data"}}`)
		case "changed":
			fmt.Fprint(w, `{"currentUser":{"id":"different-live-id","name":"DifferentUser"}}`)
		case "unauthorized":
			http.Error(w, "whoami-secret-token", 401)
		case "forbidden":
			http.Error(w, "whoami-secret-token", 403)
		case "server-error":
			http.Error(w, "whoami-secret-token", 500)
		case "anonymous":
			fmt.Fprint(w, `{"currentUser":null}`)
		case "missing-name":
			fmt.Fprint(w, `{"currentUser":{"id":"live-user-id"}}`)
		case "missing-id":
			fmt.Fprint(w, `{"currentUser":{"name":"Name"}}`)
		case "malformed":
			fmt.Fprint(w, `{"currentUser":`)
		case "redirect":
			http.Redirect(w, r, trap.URL, http.StatusFound)
		}
	}))
	defer server.Close()
	config := t.TempDir()
	path := filepath.Join(config, "treecli", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	saved := fmt.Sprintf(`[environments.prod]
backend_url = %q
active_account = "gm-bot"
[accounts.prod.gm-bot]
backend_url = %q
access_token = "whoami-secret-token"
client = "whoami-secret-client"
uid = "bot@example.test"
current_user_id = "stale-saved-id"
`, server.URL, server.URL)
	if err := os.WriteFile(path, []byte(saved), 0600); err != nil {
		t.Fatal(err)
	}
	out, stderr, err := runCLI(t, binary, config, "", "whoami")
	if err != nil || !strings.Contains(out, "Username: Brooz_GM_Tip_Bot") || !strings.Contains(out, "Account: gm-bot") || !strings.Contains(out, "Environment: prod") {
		t.Fatalf("default whoami: %v %s %s", err, out, stderr)
	}
	mode.Store("changed")
	out, stderr, err = runCLI(t, binary, config, "", "whoami", "--json")
	var identity map[string]interface{}
	if err != nil || json.Unmarshal([]byte(out), &identity) != nil || identity["username"] != "DifferentUser" || identity["user_id"] != "different-live-id" {
		t.Fatalf("live JSON: %v %s %s", err, out, stderr)
	}
	for _, secret := range []string{"whoami-secret-token", "whoami-secret-client", "bot@example.test", "stale-saved-id", "secret-server-data"} {
		if strings.Contains(out+stderr, secret) {
			t.Fatalf("identity output leaked %s", secret)
		}
	}
	for _, failure := range []string{"unauthorized", "forbidden", "server-error", "anonymous", "missing-name", "missing-id", "malformed", "redirect"} {
		t.Run(failure, func(t *testing.T) {
			mode.Store(failure)
			out, stderr, err := runCLI(t, binary, config, "", "whoami", "--json")
			if err == nil || out != "" {
				t.Fatalf("failed verification must not report identity: %v %s %s", err, out, stderr)
			}
			if strings.Contains(stderr, "whoami-secret-token") || strings.Contains(stderr, "stale-saved-id") {
				t.Fatal("error leaked credentials or used stale identity")
			}
		})
	}
	if redirectCalls.Load() != 0 {
		t.Fatal("forwarded credentials to redirect destination")
	}
	before := calls.Load()
	out, stderr, err = runCLI(t, binary, config, "", "account", "show", "--json")
	if err != nil || !strings.Contains(out, "stale-saved-id") || calls.Load() != before {
		t.Fatalf("account show must remain offline: %v %s %s", err, out, stderr)
	}
	out, stderr, err = runCLI(t, binary, config, "", "--account", "missing", "whoami")
	if err == nil || calls.Load() != before || out != "" {
		t.Fatalf("missing credentials should fail locally: %v %s %s", err, out, stderr)
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != saved {
		t.Fatal("whoami rewrote saved configuration")
	}
	server.Close()
	out, stderr, err = runCLI(t, binary, config, "", "whoami", "--json")
	if err == nil || out != "" {
		t.Fatalf("network failure should not report identity: %v %s %s", err, out, stderr)
	}
}
