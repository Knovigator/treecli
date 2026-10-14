package cmd

import (
	"encoding/json"
	"github.com/spf13/viper"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerateQuoteForwardsAliasEmptyPromptReferenceAndCap(t *testing.T) {
	withGenerateGlobals(t)
	viper.Reset()
	t.Cleanup(viper.Reset)
	oldEnv, oldAccount, oldBackend, oldHost := SelectedEnvironment, SelectedAccount, BackendURLOverride, AppHostOverride
	oldCap, oldQuote, oldID := generateMaxPriceCents, generateQuoteID, generateRequestID
	oldChanged := GenerateCmd.Flags().Lookup("max-price-cents").Changed
	t.Cleanup(func() {
		SelectedEnvironment, SelectedAccount, BackendURLOverride, AppHostOverride = oldEnv, oldAccount, oldBackend, oldHost
		generateMaxPriceCents, generateQuoteID, generateRequestID = oldCap, oldQuote, oldID
		GenerateCmd.Flags().Lookup("max-price-cents").Changed = oldChanged
	})
	SelectedEnvironment, SelectedAccount, AppHostOverride = "", "", ""
	for _, name := range []string{"TREECLI_ENV", "TREECLI_ACCOUNT", "TREECLI_BACKEND_URL", "TREECTL_BACKEND_URL", "TREECLI_PROFILE", "TREECTL_PROFILE"} {
		t.Setenv(name, "")
	}
	t.Setenv("TREECLI_ALLOW_INSECURE_HTTP", "1")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var call struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		json.NewDecoder(r.Body).Decode(&call)
		if call.Name != "quote_generation" || call.Arguments["model"] != "!sfx" || call.Arguments["prompt"] != "" || call.Arguments["reference"] != "https://cdn.example/input.mp4" || call.Arguments["max_price_cents"] != float64(123) {
			t.Errorf("lost generation fields %#v", call)
		}
		settings, _ := call.Arguments["settings"].(map[string]interface{})
		if settings["duration_seconds"] != float64(6) || settings["future_setting"] != "preserved" {
			t.Errorf("lost settings %#v", settings)
		}
		w.Write([]byte(`{"structuredContent":{"quote_id":"q","price_cents":108}}`))
	}))
	defer server.Close()
	BackendURLOverride = server.URL
	viper.Set("active_profile", "prod")
	viper.Set("profiles.prod.backend_url", server.URL)
	viper.Set("profiles.prod.access_token", "test-token")
	viper.Set("profiles.prod.client", "test-client")
	viper.Set("profiles.prod.uid", "test-uid")
	generateQuote = true
	generateQuoteID = ""
	generateRequestID = ""
	generateMaxPriceCents = 123
	GenerateCmd.Flags().Lookup("max-price-cents").Changed = true
	generateReference = "https://cdn.example/input.mp4"
	generateSettingsRaw = `{"future_setting":"preserved"}`
	generateDuration = 6
	generateInputs = nil
	generatePaymentMode = ""
	generatePollInterval = time.Second
	generateTimeout = time.Minute
	if err := runGenerate(GenerateCmd, []string{"!sfx"}); err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestGenerateReviewedQuoteRetryDoesNotUploadOrRequote(t *testing.T) {
	withGenerateGlobals(t)
	oldQuote, oldID := generateQuoteID, generateRequestID
	t.Cleanup(func() { generateQuoteID, generateRequestID = oldQuote, oldID })
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var call struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		json.NewDecoder(r.Body).Decode(&call)
		if call.Name != "create_generation" || call.Arguments["quote_id"] != "reviewed-quote" || call.Arguments["idempotency_key"] != "retry-key" || len(call.Arguments) != 2 {
			t.Errorf("unexpected retry RPC %#v", call)
		}
		w.Write([]byte(`{"structuredContent":{"generation_id":"g","status":"failed","error":{"message":"fixture terminal failure"}}}`))
	}))
	defer server.Close()
	configureSharedCommandProfile(t, server.URL)
	generateQuote = false
	generateQuoteID = "reviewed-quote"
	generateRequestID = "retry-key"
	generateReference = "@/missing/reference.png"
	generateSettingsRaw = `{"nested":{"image":"@/missing/other.png"}}`
	generateInputs = []string{"image=@/missing/input.png"}
	generateDuration = 0
	generatePaymentMode = ""
	generatePollInterval = time.Second
	generateTimeout = time.Minute
	generateOut = t.TempDir() + "/output.png"
	err := runGenerate(GenerateCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "fixture terminal failure") || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestGenerateQuoteAndReviewedQuoteNeverExecutes(t *testing.T) {
	withGenerateGlobals(t)
	oldQuote := generateQuoteID
	t.Cleanup(func() { generateQuoteID = oldQuote })
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Error("quote conflict must not request backend")
	}))
	defer server.Close()
	configureSharedCommandProfile(t, server.URL)
	generateQuote = true
	generateQuoteID = "reviewed-quote"
	err := runGenerate(GenerateCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") || calls != 0 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func configureSharedCommandProfile(t *testing.T, backend string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	oldEnv, oldAccount, oldBackend, oldHost := SelectedEnvironment, SelectedAccount, BackendURLOverride, AppHostOverride
	t.Cleanup(func() {
		SelectedEnvironment, SelectedAccount, BackendURLOverride, AppHostOverride = oldEnv, oldAccount, oldBackend, oldHost
	})
	SelectedEnvironment, SelectedAccount, AppHostOverride = "", "", ""
	BackendURLOverride = backend
	for _, name := range []string{"TREECLI_ENV", "TREECLI_ACCOUNT", "TREECLI_BACKEND_URL", "TREECTL_BACKEND_URL", "TREECLI_PROFILE", "TREECTL_PROFILE"} {
		t.Setenv(name, "")
	}
	t.Setenv("TREECLI_ALLOW_INSECURE_HTTP", "1")
	viper.Set("active_profile", "prod")
	viper.Set("profiles.prod.backend_url", backend)
	viper.Set("profiles.prod.access_token", "test-token")
	viper.Set("profiles.prod.client", "test-client")
	viper.Set("profiles.prod.uid", "test-uid")
}
