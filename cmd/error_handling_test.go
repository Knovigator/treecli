package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestRunGetThreadReturnsErrorWhenUnauthenticated(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigFile(filepath.Join(t.TempDir(), "config.toml"))

	SelectedProfile = "isolated-test"
	BackendURLOverride = "https://example.invalid"
	AppHostOverride = ""
	t.Cleanup(func() {
		SelectedProfile = ""
		BackendURLOverride = ""
		AppHostOverride = ""
	})

	err := runGetThread(nil, []string{"00000000-0000-4000-8000-000000000000"})
	if err == nil {
		t.Fatal("expected missing credentials to return an error")
	}
	if !strings.Contains(err.Error(), "missing credentials") {
		t.Fatalf("expected missing credentials error, got %v", err)
	}
}

func TestSaveProfileUsesOwnerOnlyConfigPermissions(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	viper.SetConfigFile(configPath)

	err := saveProfile(profileConfig{
		Name:        "test",
		BackendURL:  "https://example.invalid",
		AppHost:     "https://app.example.invalid",
		AccessToken: "access-token",
		Client:      "client",
		UID:         "user@example.invalid",
	}, true)
	if err != nil {
		t.Fatalf("saveProfile returned error: %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("expected config file mode 0600, got %04o", got)
	}
}

func TestRedactProfileRedactsCredentialFields(t *testing.T) {
	profile := profileConfig{
		AccessToken: "access-token-secret",
		Client:      "client-secret",
		UID:         "user@example.test",
	}

	redactedProfile := redactProfile(profile)

	if redactedProfile.AccessToken == profile.AccessToken {
		t.Fatal("expected access token to be redacted")
	}
	if redactedProfile.Client == profile.Client {
		t.Fatal("expected client credential to be redacted")
	}
	if redactedProfile.UID == profile.UID {
		t.Fatal("expected email-like uid to be redacted")
	}
}

func TestValidateCredentialTransportAllowsHTTPS(t *testing.T) {
	if err := validateCredentialTransport("https://example.test"); err != nil {
		t.Fatalf("expected https backend to be allowed, got %v", err)
	}
}

func TestValidateCredentialTransportAllowsLoopbackHTTP(t *testing.T) {
	for _, backendURL := range []string{
		"http://localhost:5001",
		"http://127.0.0.1:5001",
		"http://[::1]:5001",
	} {
		if err := validateCredentialTransport(backendURL); err != nil {
			t.Fatalf("expected loopback backend %q to be allowed, got %v", backendURL, err)
		}
	}
}

func TestValidateCredentialTransportRejectsNonLocalHTTP(t *testing.T) {
	err := validateCredentialTransport("http://example.test")
	if err == nil {
		t.Fatal("expected non-local http backend to be rejected")
	}
}

func TestValidateCredentialTransportAllowsExplicitInsecureOverride(t *testing.T) {
	t.Setenv("TREECTL_ALLOW_INSECURE_HTTP", "1")

	if err := validateCredentialTransport("http://example.test"); err != nil {
		t.Fatalf("expected explicit insecure override to allow backend, got %v", err)
	}
}
