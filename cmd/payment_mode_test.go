package cmd

import "testing"

func TestNormalizePaymentMode(t *testing.T) {
	tests := map[string]string{
		"":               "",
		"default":        "",
		"bsv":            "bsv",
		"bitcoinsv":      "bsv",
		"bitcoin-sv":     "bsv",
		"usd":            "stripe_metered",
		"stripe":         "stripe_metered",
		"stripe_metered": "stripe_metered",
		"stripe-metered": "stripe_metered",
		" USD ":          "stripe_metered",
	}

	for raw, want := range tests {
		got, err := normalizePaymentMode(raw)
		if err != nil {
			t.Fatalf("normalizePaymentMode(%q) returned error: %v", raw, err)
		}
		if got != want {
			t.Fatalf("normalizePaymentMode(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizePaymentModeRejectsUnknownMode(t *testing.T) {
	if _, err := normalizePaymentMode("credits"); err == nil {
		t.Fatal("expected unknown payment mode to be rejected")
	}
}

func TestPaymentModeDisplay(t *testing.T) {
	tests := map[string]string{
		"stripe_metered": "USD",
		"bsv":            "Bitcoin SV",
		"":               "",
		"custom":         "custom",
	}

	for raw, want := range tests {
		if got := paymentModeDisplay(raw); got != want {
			t.Fatalf("paymentModeDisplay(%q) = %q, want %q", raw, got, want)
		}
	}
}
