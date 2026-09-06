package cmd

import (
	"testing"

	"github.com/spf13/viper"
)

func TestAccountSelectionScopesCredentials(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	oldEnv, oldAccount, oldBackend, oldHost := SelectedEnvironment, SelectedAccount, BackendURLOverride, AppHostOverride
	t.Cleanup(func() {
		SelectedEnvironment, SelectedAccount, BackendURLOverride, AppHostOverride = oldEnv, oldAccount, oldBackend, oldHost
	})
	SelectedEnvironment, SelectedAccount, BackendURLOverride, AppHostOverride = "", "", "", ""
	for _, name := range []string{"TREECLI_ENV", "TREECLI_ACCOUNT", "TREECLI_BACKEND_URL", "TREECTL_BACKEND_URL", "TREECLI_APP_HOST", "TREECTL_APP_HOST"} {
		t.Setenv(name, "")
	}
	viper.Set("active_profile", "dev")
	profile, err := resolveAccount()
	if err != nil || profile.Environment != "prod" || profile.Account != "default" {
		t.Fatalf("default: %+v %v", profile, err)
	}
	prod := builtInProfiles()["prod"].BackendURL
	viper.Set("profiles.prod.backend_url", prod)
	viper.Set("profiles.prod.access_token", "old-prod")
	viper.Set("profiles.custom.backend_url", prod)
	viper.Set("profiles.custom.access_token", "old-bot")
	profile, _ = resolveAccount()
	if profile.AccessToken != "old-prod" {
		t.Fatal("legacy default not available")
	}
	SelectedAccount = "custom"
	profile, _ = resolveAccount()
	if profile.AccessToken != "old-bot" {
		t.Fatal("legacy named account not available")
	}
	SelectedAccount = ""
	viper.Set("active_profile", "custom")
	profile, _ = resolveAccount()
	if profile.Account != "custom" || profile.AccessToken != "old-bot" {
		t.Fatal("lost legacy production identity")
	}
	SelectedAccount = "custom"

	SelectedEnvironment = "staging"
	profile, _ = resolveAccount()
	if profile.AccessToken != "" {
		t.Fatal("production credential leaked to staging")
	}
	viper.Set("accounts.staging.custom.backend_url", builtInProfiles()["staging"].BackendURL)
	viper.Set("accounts.staging.custom.access_token", "staging-bot")
	profile, _ = resolveAccount()
	if profile.AccessToken != "staging-bot" {
		t.Fatal("staging credential not selected")
	}
	BackendURLOverride = "https://different.example.test"
	profile, _ = resolveAccount()
	if profile.AccessToken != "" || profile.UID != "" || profile.CurrentUserID != "" {
		t.Fatal("override leaked identity")
	}
	BackendURLOverride = ""
	SelectedEnvironment = "production"
	profile, _ = resolveAccount()
	if profile.Environment != "prod" {
		t.Fatal("production alias")
	}
	SelectedAccount = "unsafe.name"
	if _, err := resolveAccount(); err == nil {
		t.Fatal("unsafe config key accepted")
	}
}

func TestEnvironmentAccountPrecedence(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	oldEnv, oldAccount := SelectedEnvironment, SelectedAccount
	t.Cleanup(func() { SelectedEnvironment, SelectedAccount = oldEnv, oldAccount })
	SelectedEnvironment, SelectedAccount = "", ""
	t.Setenv("TREECLI_ENV", "staging")
	t.Setenv("TREECLI_ACCOUNT", "bot")
	env, account, err := accountNames()
	if err != nil || env != "staging" || account != "bot" {
		t.Fatalf("env defaults: %s %s %v", env, account, err)
	}
	SelectedEnvironment, SelectedAccount = "production", "brooz"
	env, account, err = accountNames()
	if err != nil || env != "prod" || account != "brooz" {
		t.Fatalf("flag overrides: %s %s %v", env, account, err)
	}
}
