package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Knovigator/treecli/api"
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

func TestUSDPayablePrefersTheServersCreditsAnswer(t *testing.T) {
	var credits api.StripeAiBillingStatusResponse
	if err := json.Unmarshal([]byte(`{"usd":{"mode":"credits","payable":true,"credits":{"balance_cents":1250,"auto_reload":true,"card_saved":true,"reload_cents":2000}},"stripe":{"subscription_status":null}}`), &credits); err != nil {
		t.Fatal(err)
	}
	if !usdPayable(credits) {
		t.Fatal("a credits account with a balance should be payable")
	}
	if got := usdReadyLine(credits); got != "AI credits: $12.50. Auto-reload adds $20.00 when it runs low." {
		t.Fatalf("usdReadyLine = %q", got)
	}

	var legacy api.StripeAiBillingStatusResponse
	if err := json.Unmarshal([]byte(`{"stripe":{"subscription_status":"active"}}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if !usdPayable(legacy) || !strings.Contains(usdReadyLine(legacy), "Card billing is on") {
		t.Fatal("an older server's active card billing should still count")
	}
}

func TestBillingFlagsRejectAmountsTheServerDoesNotOffer(t *testing.T) {
	t.Cleanup(func() { billingCheckoutAmount, billingAutoReloadAmount, billingAutoReloadBelow = 0, 0, 0 })

	billingCheckoutAmount = 7
	if err := runBillingCheckout(nil, nil); ExitCode(err) != ExitUsage {
		t.Fatalf("checkout --amount 7 should be a usage error, got %v", err)
	}
	billingCheckoutAmount = 0

	billingAutoReloadBelow = 3
	if err := runBillingAutoReload(nil, []string{"on"}); ExitCode(err) != ExitUsage {
		t.Fatalf("auto-reload --below 3 should be a usage error, got %v", err)
	}
	billingAutoReloadBelow = 0

	if err := runBillingAutoReload(nil, []string{"maybe"}); ExitCode(err) != ExitUsage {
		t.Fatalf("auto-reload maybe should be a usage error, got %v", err)
	}
	if err := runBillingAutoReload(nil, nil); ExitCode(err) != ExitUsage {
		t.Fatalf("auto-reload with nothing to change should be a usage error, got %v", err)
	}
}

func TestNotEnoughCreditsIsASpendingRefusal(t *testing.T) {
	err := errors.New("Not enough AI credits. Add credits in your wallet, or run `treecli billing checkout`.")
	if ExitCode(err) != ExitSpending {
		t.Fatalf("exit code = %d, want %d", ExitCode(err), ExitSpending)
	}
}
