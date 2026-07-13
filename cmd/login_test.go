package cmd

import (
	"testing"

	"github.com/spf13/viper"
)

func TestResolveProfileNameDefaultsToProd(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("TREECLI_PROFILE", "")
	t.Setenv("TREECTL_PROFILE", "")

	previousSelectedProfile := SelectedProfile
	SelectedProfile = ""
	t.Cleanup(func() {
		SelectedProfile = previousSelectedProfile
	})

	if got := resolveProfileName(); got != "prod" {
		t.Fatalf("expected default profile to be prod, got %q", got)
	}
}
