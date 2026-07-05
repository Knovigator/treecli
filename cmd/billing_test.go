package cmd

import (
	"strings"
	"testing"
)

func TestCentsToUSD(t *testing.T) {
	cases := map[int64]string{
		0:     "$0.00",
		5:     "$0.05",
		1234:  "$12.34",
		-250:  "-$2.50",
		10000: "$100.00",
	}
	for cents, want := range cases {
		if got := centsToUSD(cents); got != want {
			t.Fatalf("centsToUSD(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestRunBillingModeInvalidModeReturnsError(t *testing.T) {
	err := runBillingMode(nil, []string{"credits"})
	if err == nil {
		t.Fatal("expected invalid billing mode to return an error")
	}
	if !strings.Contains(err.Error(), "invalid --payment") {
		t.Fatalf("expected invalid payment error, got %v", err)
	}
}

func TestRunBillingWalletImportMissingFileReturnsError(t *testing.T) {
	err := runBillingWalletImport(nil, []string{"/tmp/treecli-missing-wallet-for-test.json"})
	if err == nil {
		t.Fatal("expected missing import file to return an error")
	}
	if !strings.Contains(err.Error(), "reading wallet file") {
		t.Fatalf("expected reading wallet file error, got %v", err)
	}
}
