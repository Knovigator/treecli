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
		{"unset", "null", "null", "Daily cap: unset"},
		{"zero", "0", "0", "Daily cap: 0 USD cents"},
		{"set", "250", "220", "Daily cap: 250 USD cents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"structuredContent":{"daily_limit_cents":` + tc.limit + `,"remaining_cents":` + tc.remaining + `,"reserved_cents":30,"currency":"USD","window":"rolling_24h","window_seconds":86400,"timezone":"UTC","resets_at":null,"next_release_at":"2026-10-20T05:30:00Z","scope":"application","accounting":"admission reservations","exclusions":["network fees"]}}`))
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
			err := runBillingBudget(billingBudgetCmd)
			for _, want := range []string{tc.want, "Reserved: 30 USD cents", "2026-10-20T05:30:00Z", "Window: rolling 24 hours (no fixed midnight reset)", "admission reservations", "network fees"} {
				if err != nil || !strings.Contains(output.String(), want) {
					t.Fatalf("missing %q output=%q err=%v", want, output.String(), err)
				}
			}
			if tc.limit == "null" && !strings.Contains(output.String(), "Remaining: unavailable") {
				t.Fatal("unset became zero")
			}
		})
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
	err := runBillingBudget(billingBudgetCmd)
	if err == nil || !strings.Contains(err.Error(), "billing scope required") || output.Len() != 0 {
		t.Fatalf("output=%q err=%v", output.String(), err)
	}
}

func TestBudgetWithoutConnectionDoesNotImplyUnlimitedCapacity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"structuredContent":{"status":"not_connected","daily_limit_cents":null,"remaining_cents":null,"next_release_at":null,"currency":"USD"}}`))
	}))
	defer server.Close()
	configureSharedCommandProfile(t, server.URL)
	old := billingBudgetJSON
	billingBudgetJSON = false
	t.Cleanup(func() { billingBudgetJSON = old })
	var out bytes.Buffer
	previous := billingBudgetCmd.OutOrStdout()
	billingBudgetCmd.SetOut(&out)
	t.Cleanup(func() { billingBudgetCmd.SetOut(previous) })
	if err := runBillingBudget(billingBudgetCmd); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Connection: not connected", "Daily cap: unset", "Remaining: unavailable", "Next capacity release: none scheduled", "treecli connect"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out.String())
		}
	}
	for _, bad := range []string{"uncapped", "unlimited", "Resets at", "Admission day"} {
		if strings.Contains(out.String(), bad) {
			t.Fatalf("misleading %q: %s", bad, out.String())
		}
	}
}
