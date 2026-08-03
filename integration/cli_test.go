package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	integrationEmail       = "integration-user@example.test"
	integrationPassword    = "integration-password"
	integrationAccessToken = "integration-access-token-secret"
	integrationClient      = "integration-client-secret"
)

type fakeBackendState struct {
	mu             sync.Mutex
	createdQuestID string
}

func TestCompiledCLIUserBoundary(t *testing.T) {
	state := &fakeBackendState{}
	server := httptest.NewServer(fakeTreechatHandler(t, state))
	t.Cleanup(server.Close)

	repoRoot := repositoryRoot(t)
	configHome := filepath.Join(t.TempDir(), "config")
	binaryPath := buildCLI(t, repoRoot)
	baseArgs := []string{
		"--profile", "integration",
		"--backend-url", server.URL,
		"--app-host", "https://app.example.test",
	}

	stdout, stderr, err := runCLI(t, binaryPath, configHome, integrationPassword+"\n", append(baseArgs,
		"login", "--email", integrationEmail, "--password-stdin",
	)...)
	if err != nil {
		t.Fatalf("login failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Login successful") {
		t.Fatalf("expected successful login output, got %q", stdout)
	}
	if strings.Contains(stdout+stderr, integrationPassword) || strings.Contains(stdout+stderr, integrationAccessToken) {
		t.Fatal("login output exposed a credential")
	}
	assertOwnerOnlyConfig(t, configHome)

	t.Run("profile output is redacted", func(t *testing.T) {
		stdout, stderr, err := runCLI(t, binaryPath, configHome, "", append(baseArgs, "profile", "show", "--json")...)
		if err != nil {
			t.Fatalf("profile show failed: %v\n%s", err, stderr)
		}
		if !json.Valid([]byte(stdout)) {
			t.Fatalf("profile output is not JSON: %q", stdout)
		}
		for _, secret := range []string{integrationAccessToken, integrationClient, integrationEmail} {
			if strings.Contains(stdout+stderr, secret) {
				t.Fatalf("profile output exposed %q", secret)
			}
		}
	})

	t.Run("thread JSON preserves the command contract", func(t *testing.T) {
		stdout, stderr, err := runCLI(t, binaryPath, configHome, "", append(baseArgs, "get", "thread", "thread-1", "--json")...)
		if err != nil {
			t.Fatalf("get thread failed: %v\n%s", err, stderr)
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("get thread output is not JSON: %v\n%s", err, stdout)
		}
		quest, ok := payload["quest"].(map[string]interface{})
		if !ok || quest["id"] != "thread-1" {
			t.Fatalf("unexpected thread payload: %#v", payload)
		}
		if strings.Contains(stdout, integrationEmail) {
			t.Fatal("thread JSON exposed an identity email")
		}
	})

	t.Run("private post is sent and can be read back", func(t *testing.T) {
		stdout, stderr, err := runCLI(t, binaryPath, configHome, "", append(baseArgs,
			"new", "post", "integration post", "--private", "--json",
		)...)
		if err != nil {
			t.Fatalf("new post failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		var payload struct {
			Quest struct {
				ID string `json:"id"`
			} `json:"quest"`
		}
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("new post output is not JSON: %v\n%s", err, stdout)
		}
		if payload.Quest.ID == "" {
			t.Fatalf("new post did not return a quest id: %s", stdout)
		}

		stdout, stderr, err = runCLI(t, binaryPath, configHome, "", append(baseArgs,
			"get", "thread", payload.Quest.ID, "--json",
		)...)
		if err != nil {
			t.Fatalf("reading created post failed: %v\n%s", err, stderr)
		}
		if !strings.Contains(stdout, "integration post") {
			t.Fatalf("created post was not returned: %s", stdout)
		}
	})

	t.Run("upvalue history is wired through the binary", func(t *testing.T) {
		stdout, stderr, err := runCLI(t, binaryPath, configHome, "", append(baseArgs,
			"get", "upvalues", "--page", "2", "--per-page", "25", "--json",
		)...)
		if err != nil {
			t.Fatalf("get upvalues failed: %v\n%s", err, stderr)
		}
		var payload struct {
			Page    int  `json:"page"`
			PerPage int  `json:"per_page"`
			HasMore bool `json:"has_more"`
		}
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("get upvalues output is not JSON: %v\n%s", err, stdout)
		}
		if payload.Page != 2 || payload.PerPage != 25 || !payload.HasMore {
			t.Fatalf("unexpected upvalue pagination: %#v", payload)
		}
	})

	t.Run("backend failures are nonzero and redacted", func(t *testing.T) {
		stdout, stderr, err := runCLI(t, binaryPath, configHome, "", append(baseArgs,
			"get", "thread", "failing-thread", "--json",
		)...)
		if err == nil {
			t.Fatalf("expected failing thread command to return nonzero\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
		}
		for _, secret := range []string{"backend-leaked@example.test", "backend-secret-token"} {
			if strings.Contains(stdout+stderr, secret) {
				t.Fatalf("backend error exposed %q: %s%s", secret, stdout, stderr)
			}
		}
		if !strings.Contains(stderr, "Error:") {
			t.Fatalf("expected command error on stderr, got %q", stderr)
		}
	})
}

func fakeTreechatHandler(t *testing.T, state *fakeBackendState) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/auth/sign_in":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse login form: %v", err)
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			if r.Form.Get("email") != integrationEmail || r.Form.Get("password") != integrationPassword {
				t.Errorf("unexpected login form: %#v", r.Form)
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}
			w.Header().Set("access-token", integrationAccessToken)
			w.Header().Set("client", integrationClient)
			w.Header().Set("uid", integrationEmail)
			w.Header().Set("expiry", "4102444800")
			writeJSON(t, w, map[string]interface{}{"ok": true})

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/gon":
			if !hasIntegrationAuth(r) {
				http.Error(w, "missing auth", http.StatusUnauthorized)
				return
			}
			writeJSON(t, w, map[string]interface{}{
				"host": "app.example.test",
				"currentUser": map[string]string{
					"id":       "user-1",
					"space_id": "space-1",
				},
			})

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/quests/"):
			if !hasIntegrationAuth(r) {
				http.Error(w, "missing auth", http.StatusUnauthorized)
				return
			}
			questID := strings.TrimPrefix(r.URL.Path, "/api/v1/quests/")
			if questID == "failing-thread" {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(t, w, map[string]string{
					"email":        "backend-leaked@example.test",
					"access_token": "backend-secret-token",
					"error":        "failed for backend-leaked@example.test",
				})
				return
			}

			content := "hello from the fake backend"
			state.mu.Lock()
			if questID == state.createdQuestID {
				content = "integration post"
			}
			state.mu.Unlock()
			writeJSON(t, w, map[string]interface{}{
				"quest": map[string]interface{}{
					"id": questID,
					"parent": map[string]interface{}{
						"id":      "answer-1",
						"content": content,
						"user": map[string]string{
							"id":    "user-2",
							"name":  "Integration User",
							"email": integrationEmail,
						},
					},
					"sorted_answers": []interface{}{},
				},
			})

		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/quests":
			if !hasIntegrationAuth(r) {
				http.Error(w, "missing auth", http.StatusUnauthorized)
				return
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse quest form: %v", err)
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			questID := r.FormValue("id")
			answerID := r.FormValue("parent_attributes[id]")
			if questID == "" || answerID == "" {
				t.Errorf("missing retry-safe ids in quest form: %#v", r.Form)
			}
			if r.FormValue("space_id") != "space-1" || r.FormValue("parent_attributes[content]") != "integration post" {
				t.Errorf("unexpected quest form: %#v", r.Form)
			}
			if r.FormValue("private") != "true" {
				t.Errorf("expected private quest, got form %#v", r.Form)
			}
			state.mu.Lock()
			state.createdQuestID = questID
			state.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			writeJSON(t, w, map[string]interface{}{
				"quest": map[string]interface{}{
					"id": questID,
					"parent": map[string]string{
						"id":      answerID,
						"content": "integration post",
					},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/bsv/history":
			if !hasIntegrationAuth(r) {
				http.Error(w, "missing auth", http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "25" {
				t.Errorf("unexpected upvalue query: %s", r.URL.RawQuery)
			}
			writeJSON(t, w, map[string]interface{}{
				"upvalues": []interface{}{},
				"page":     2,
				"per_page": 25,
				"has_more": true,
			})

		default:
			http.NotFound(w, r)
		}
	})
}

func hasIntegrationAuth(r *http.Request) bool {
	return r.Header.Get("access-token") == integrationAccessToken &&
		r.Header.Get("client") == integrationClient &&
		r.Header.Get("uid") == integrationEmail
}

func writeJSON(t *testing.T, w http.ResponseWriter, value interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("write JSON response: %v", err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func buildCLI(t *testing.T, repoRoot string) string {
	t.Helper()
	binaryName := "treecli-integration"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binaryPath, ".")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	return binaryPath
}

func runCLI(t *testing.T, binaryPath string, configHome string, stdin string, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Env = isolatedEnvironment(configHome)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("CLI command timed out: %s %s", binaryPath, strings.Join(args, " "))
	}
	return stdout.String(), stderr.String(), err
}

func isolatedEnvironment(configHome string) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "XDG_CONFIG_HOME=") ||
			strings.HasPrefix(entry, "TREECTL_") ||
			strings.HasPrefix(entry, "TREECLI_") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "XDG_CONFIG_HOME="+configHome)
}

func assertOwnerOnlyConfig(t *testing.T, configHome string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}

	candidates := []string{
		filepath.Join(configHome, "treecli", "config.toml"),
		filepath.Join(configHome, "treectl", "config.toml"),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("stat config %s: %v", candidate, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("expected config mode 0600, got %04o", info.Mode().Perm())
		}
		return
	}
	t.Fatalf("no CLI config found under %s", configHome)
}
