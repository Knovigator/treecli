package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStripeAiBillingErrorsUseSafeResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"error": "failed for alice@example.test",
			"access_token": "secret-token"
		}`))
	}))
	t.Cleanup(server.Close)

	cases := map[string]func(string) error{
		"checkout": func(url string) error {
			_, err := CreateStripeAiBillingCheckoutSession(url, "token", "client", "uid")
			return err
		},
		"status": func(url string) error {
			_, err := GetStripeAiBillingStatus(url, "token", "client", "uid", true)
			return err
		},
		"mode": func(url string) error {
			_, err := SetStripeAiPaymentMode(url, "token", "client", "uid", "bsv")
			return err
		},
		"sync": func(url string) error {
			_, err := SyncStripeAiBillingCheckoutSession(url, "token", "client", "uid", "cs_test_123")
			return err
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			err := call(server.URL)
			if err == nil {
				t.Fatal("expected error")
			}
			message := err.Error()
			if strings.Contains(message, "alice@example.test") {
				t.Fatalf("expected email to be redacted, got %q", message)
			}
			if strings.Contains(message, "secret-token") {
				t.Fatalf("expected access token to be redacted, got %q", message)
			}
			if !strings.Contains(message, redactedValue) {
				t.Fatalf("expected redacted marker in error, got %q", message)
			}
		})
	}
}
