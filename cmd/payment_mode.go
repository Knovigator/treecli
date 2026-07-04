package cmd

import (
	"fmt"
	"strings"
)

func normalizePaymentMode(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "default":
		return "", nil
	case "bsv", "bitcoin_sv", "bitcoin-sv", "bitcoinsv":
		return "bsv", nil
	case "usd", "stripe", "stripe_metered", "stripe-metered":
		return "stripe_metered", nil
	default:
		return "", fmt.Errorf("invalid --payment %q; use usd or bsv/bitcoinsv", raw)
	}
}

func paymentModeDisplay(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "stripe_metered":
		return "USD"
	case "bsv":
		return "Bitcoin SV"
	default:
		return strings.TrimSpace(mode)
	}
}
