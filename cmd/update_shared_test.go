package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateCheckUsesSharedMetadataWithoutInstalling(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "available", false: "server_says_current"}[available], func(t *testing.T) {
			oldOptions, oldVersion := updateOptions, CurrentVersion
			t.Cleanup(func() { updateOptions, CurrentVersion = oldOptions, oldVersion })
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/api/v1/treecli/call" || r.Header.Get("access-token") != "test-token" {
					t.Errorf("incorrect authenticated RPC: %s %s", r.Method, r.URL)
				}
				var call struct {
					Name      string `json:"name"`
					Arguments struct {
						Operation string                 `json:"operation"`
						Arguments map[string]interface{} `json:"arguments"`
					} `json:"arguments"`
				}
				if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
					t.Error(err)
				}
				args := call.Arguments.Arguments
				if call.Name != "treecli_reference" || call.Arguments.Operation != "update_check" || args["current_version"] != "v0.1.0" || args["repo"] != "Example/treecli" || args["version"] != "v0.2.0" {
					t.Errorf("lost update arguments %#v", call)
				}
				json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": map[string]interface{}{"latest_version": "v0.2.0", "update_available": available}})
			}))
			defer server.Close()
			configureSharedCommandProfile(t, server.URL)
			installDir := filepath.Join(t.TempDir(), "must-not-create")
			CurrentVersion = "v0.1.0"
			updateOptions.repo = "Example/treecli"
			updateOptions.check = true
			updateOptions.force = true
			updateOptions.json = true
			updateOptions.installDir = installDir
			output, err := captureUpdateOutput(t, func() error { return runUpdate(UpdateCmd, []string{"v0.2.0"}) })
			if err != nil || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			var result updateResult
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatalf("invalid result %q: %v", output, err)
			}
			if result.LatestVersion != "v0.2.0" || result.CurrentVersion != "v0.1.0" || result.UpdateAvailable != available || !result.CheckOnly || result.Updated || result.Asset != "" || result.InstallPath != "" {
				t.Fatalf("did not preserve server metadata %#v", result)
			}
			if _, err := os.Stat(installDir); !os.IsNotExist(err) {
				t.Fatalf("check touched installation directory: %v", err)
			}
		})
	}
}

func TestUpdateCheckPropagatesSharedFailuresWithoutInstalling(t *testing.T) {
	for _, tc := range []struct{ name, response, want string }{
		{"backend failure", `{"isError":true,"structuredContent":{"status":503,"body":{"error":"release lookup unavailable"}}}`, "release lookup unavailable"},
		{"missing version", `{"structuredContent":{"update_available":true}}`, "backend omitted release version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldOptions := updateOptions
			t.Cleanup(func() { updateOptions = oldOptions })
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(tc.response)) }))
			defer server.Close()
			configureSharedCommandProfile(t, server.URL)
			installDir := t.TempDir()
			binaryPath := filepath.Join(installDir, "treecli")
			if err := os.WriteFile(binaryPath, []byte("existing executable"), 0700); err != nil {
				t.Fatal(err)
			}
			updateOptions.repo = defaultUpdateRepo
			updateOptions.check = true
			updateOptions.force = true
			updateOptions.json = true
			updateOptions.installDir = installDir
			output, err := captureUpdateOutput(t, func() error { return runUpdate(UpdateCmd, nil) })
			if err == nil || !strings.Contains(err.Error(), tc.want) || calls != 1 || output != "" {
				t.Fatalf("calls=%d output=%q err=%v", calls, output, err)
			}
			data, err := os.ReadFile(binaryPath)
			if err != nil || string(data) != "existing executable" {
				t.Fatalf("check modified executable: %q %v", data, err)
			}
		})
	}
}

func captureUpdateOutput(t *testing.T, call func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original; reader.Close(); writer.Close() }()
	callErr := call()
	writer.Close()
	os.Stdout = original
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), callErr
}
