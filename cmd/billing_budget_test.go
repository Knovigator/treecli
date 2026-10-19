package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBudgetDisplaysServerAccountingAndExplicitUnset(t *testing.T) {
	for _, tc := range []struct{ name, limit, remaining, want string }{
		{"unset", "null", "null", "Daily cap: unset (no daily limit)"},
		{"zero", "0", "0", "Daily cap: 0 USD cents"},
		{"set", "250", "220", "Daily cap: 250 USD cents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"structuredContent":{"daily_limit_cents":` + tc.limit + `,"remaining_cents":` + tc.remaining + `,"reserved_cents":30,"currency":"USD","admission_day":"2026-10-19","resets_at":"2026-10-20T00:00:00Z","scope":"account","accounting":"admission reservations","exclusions":["network fees"]}}`))
			}))
			defer server.Close()
			configureSharedCommandProfile(t, server.URL)
			old := billingBudgetJSON
			billingBudgetJSON = false
			t.Cleanup(func() { billingBudgetJSON = old })
			var output bytes.Buffer
			oldOut := billingBudgetCmd.OutOrStdout()
			billingBudgetCmd.SetOut(&output)
			t.Cleanup(func() { billingBudgetCmd.SetOut(oldOut) })
			err := runBillingBudget(billingBudgetCmd, "get_generation_budget", map[string]interface{}{})
			for _, want := range []string{tc.want, "Reserved: 30 USD cents", "2026-10-20T00:00:00Z", "admission reservations", "network fees"} {
				if err != nil || !strings.Contains(output.String(), want) {
					t.Fatalf("missing %q output=%q err=%v", want, output.String(), err)
				}
			}
			if tc.limit == "null" && !strings.Contains(output.String(), "Remaining: uncapped") {
				t.Fatal("unset became zero")
			}
		})
	}
}

func TestBudgetRejectsInvalidLimitsWithoutBackendCalls(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; t.Error("invalid budget reached backend") }))
	defer server.Close()
	configureSharedCommandProfile(t, server.URL)
	for _, value := range []string{"-1", "2147483648", "1.5", "not-a-number"} {
		if err := billingBudgetSetCmd.RunE(billingBudgetSetCmd, []string{value}); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	if calls != 0 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestBudgetPropagatesPermissionErrorWithoutOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"isError":true,"structuredContent":{"status":403,"body":{"error":"billing scope required"}}}`))
	}))
	defer server.Close()
	configureSharedCommandProfile(t, server.URL)
	var output bytes.Buffer
	oldOut := billingBudgetCmd.OutOrStdout()
	billingBudgetCmd.SetOut(&output)
	t.Cleanup(func() { billingBudgetCmd.SetOut(oldOut) })
	err := runBillingBudget(billingBudgetCmd, "set_generation_budget", map[string]interface{}{"daily_limit_cents": nil})
	if err == nil || !strings.Contains(err.Error(), "billing scope required") || output.Len() != 0 {
		t.Fatalf("output=%q err=%v", output.String(), err)
	}
}
