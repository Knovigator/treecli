package cmd

import (
	"testing"

	"github.com/spf13/viper"
)

func TestResolveProfileNameDefaultsToProdAndAllowsExplicitDev(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("TREECLI_PROFILE", "")
	t.Setenv("TREECTL_PROFILE", "")

	originalSelectedProfile := SelectedProfile
	SelectedProfile = ""
	t.Cleanup(func() {
		SelectedProfile = originalSelectedProfile
	})

	if profileName := resolveProfileName(); profileName != "prod" {
		t.Fatalf("expected default profile prod, got %q", profileName)
	}

	SelectedProfile = "dev"
	if profileName := resolveProfileName(); profileName != "dev" {
		t.Fatalf("expected explicit dev profile, got %q", profileName)
	}
}
