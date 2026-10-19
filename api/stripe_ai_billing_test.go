package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStripeAiBillingErrorsUseSafeResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/treecli/call" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var call struct {
			Name      string `json:"name"`
			Arguments struct {
				Operation string                 `json:"operation"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Error(err)
		}
		if call.Name != "treechat_billing" {
			t.Errorf("wrong tool: %s", call.Name)
		}
		switch call.Arguments.Operation {
		case "checkout":
		case "status":
			if call.Arguments.Arguments["refresh"] != true {
				t.Error("lost refresh")
			}
		case "mode":
			if call.Arguments.Arguments["mode"] != "bsv" {
				t.Error("lost mode")
			}
		case "sync":
			if call.Arguments.Arguments["session_id"] != "cs_test_123" {
				t.Error("lost session")
			}
		default:
			t.Errorf("wrong operation %s", call.Arguments.Operation)
		}
		if r.Header.Get("access-token") != "token" || r.Header.Get("client") != "client" || r.Header.Get("uid") != "uid" {
			t.Error("missing auth")
		}
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
			_, err := CreateStripeAiBillingCheckoutSession(url, "token", "client", "uid", 0)
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

func TestBillingStatusAcceptsStringAndNumericEventIDs(t *testing.T) {
	var status StripeAiBillingStatusResponse
	raw := `{"stripe":{"subscription_status":"active"},"usage":{"events":[{"id":"0f6c2a1e-uuid","amount_cents":4,"status":"settled"},{"id":42,"amount_cents":1,"status":"pending"}]}}`
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.Usage.Events[0].ID != "0f6c2a1e-uuid" || status.Usage.Events[1].ID != "42" {
		t.Fatalf("ids: %#v", status.Usage.Events)
	}
}
