package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCompiledEnvironmentAccounts(t *testing.T) {
	binary := buildCLI(t, repositoryRoot(t))
	config := filepath.Join(t.TempDir(), "config")
	serverFor := func(environment string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/auth/sign_in":
				r.ParseForm()
				name := r.FormValue("email")
				w.Header().Set("access-token", environment+"-"+name)
				w.Header().Set("client", "client")
				w.Header().Set("uid", name)
				fmt.Fprint(w, `{}`)
			case "/api/v1/gon":
				fmt.Fprintf(w, `{"host":"app.example.test","currentUser":{"id":%q,"space_id":"space"}}`, r.Header.Get("access-token"))
			case "/api/v1/quests/thread":
				fmt.Fprintf(w, `{"quest":{"id":"thread","content":%q}}`, r.Header.Get("access-token"))
			default:
				http.NotFound(w, r)
			}
		}))
	}
	first, second := serverFor("first"), serverFor("second")
	defer first.Close()
	defer second.Close()
	run := func(stdin string, args ...string) string {
		t.Helper()
		out, stderr, err := runCLI(t, binary, config, stdin, args...)
		if err != nil {
			t.Fatalf("%v: %v %s %s", args, err, out, stderr)
		}
		return out
	}
	for _, tc := range []struct{ env, account, url string }{{"qa", "brooz", first.URL}, {"qa", "bot", first.URL}, {"other", "bot", second.URL}} {
		run("pw\n", "--env", tc.env, "--account", tc.account, "--backend-url", tc.url, "login", "--email", tc.account, "--password-stdin")
	}
	for _, tc := range []struct{ env, account, expected string }{{"qa", "brooz", "first-brooz"}, {"qa", "bot", "first-bot"}, {"other", "bot", "second-bot"}} {
		out := run("", "--env", tc.env, "--account", tc.account, "get", "threads", "thread", "--json")
		if !strings.Contains(out, tc.expected) {
			t.Fatalf("wrong account: %s", out)
		}
	}
	run("", "--env", "qa", "account", "use", "brooz")
	if out := run("", "--env", "qa", "get", "threads", "thread", "--json"); !strings.Contains(out, "first-brooz") {
		t.Fatalf("default account not persisted: %s", out)
	}
	if out := run("", "--env", "other", "get", "threads", "thread", "--json"); !strings.Contains(out, "second-bot") {
		t.Fatalf("cross-environment default changed: %s", out)
	}
	var info map[string]interface{}
	out := run("", "--env", "qa", "--account", "bot", "account", "show", "--json")
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatal(err)
	}
	if info["environment"] != "qa" || info["account"] != "bot" || info["access_token"] == "first-bot" {
		t.Fatalf("wrong account metadata or unredacted credential: %s", out)
	}
	if out := run("", "account", "show", "--json"); !strings.Contains(out, `"environment": "prod"`) {
		t.Fatalf("default env changed by login: %s", out)
	}
	var calls atomic.Int32
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "must not receive credentials", 500)
	}))
	defer trap.Close()
	_, stderr, err := runCLI(t, binary, config, "", "--env", "qa", "--account", "bot", "--backend-url", trap.URL, "get", "threads", "thread", "--json")
	if err == nil || calls.Load() != 0 || !strings.Contains(stderr, "missing credentials") {
		t.Fatalf("override did not fail closed: %v %s calls=%d", err, stderr, calls.Load())
	}
	for _, args := range [][]string{{"--profile", "prod", "--account", "bot", "account", "show"}, {"--env", "qa", "profile", "show"}, {"--env", "", "account", "show"}, {"--account", "a.b", "account", "show"}} {
		if _, _, err := runCLI(t, binary, config, "", args...); err == nil {
			t.Fatalf("accepted invalid selection: %v", args)
		}
	}
	// Old credentials remain usable through both explicit legacy selection and
	// the new account interface, provided the backend matches.
	run("pw\n", "--profile", "legacy", "--backend-url", first.URL, "login", "--email", "legacy", "--password-stdin")
	out, stderr, err = runCLI(t, binary, config, "", "--profile", "legacy", "get", "threads", "thread", "--json")
	if err != nil || !strings.Contains(stderr, "deprecated") || !strings.Contains(out, "first-legacy") {
		t.Fatalf("legacy selection failed: %v %s %s", err, out, stderr)
	}
	if out := run("", "--env", "qa", "--account", "legacy", "get", "threads", "thread", "--json"); !strings.Contains(out, "first-legacy") {
		t.Fatalf("legacy migration failed: %s", out)
	}
}
