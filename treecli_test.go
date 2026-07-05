package main

import (
	"strings"
	"testing"

	treeclicmd "github.com/Knovigator/treecli/cmd"
)

func TestRootExecuteReturnsCommandErrors(t *testing.T) {
	treeclicmd.SelectedProfile = "isolated-test"
	treeclicmd.BackendURLOverride = "https://example.invalid"
	rootCmd.SetArgs([]string{"get", "thread", "00000000-0000-4000-8000-000000000000"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		treeclicmd.SelectedProfile = ""
		treeclicmd.BackendURLOverride = ""
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected command failure to return an error")
	}
	if !strings.Contains(err.Error(), "missing credentials") {
		t.Fatalf("expected missing credentials error, got %v", err)
	}
}

func TestRootExecuteReturnsBillingCommandErrors(t *testing.T) {
	rootCmd.SetArgs([]string{"billing", "mode", "credits"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected billing command failure to return an error")
	}
	if !strings.Contains(err.Error(), "invalid --payment") {
		t.Fatalf("expected invalid payment error, got %v", err)
	}
}
