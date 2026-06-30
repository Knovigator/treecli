package cmd

import (
	"strings"
	"testing"
	"time"
)

func withGenerateGlobals(t *testing.T) {
	t.Helper()

	previousOut := generateOut
	previousSettingsRaw := generateSettingsRaw
	previousDuration := generateDuration
	previousJSONOutput := generateJSONOutput
	previousPollInterval := generatePollInterval
	previousTimeout := generateTimeout

	t.Cleanup(func() {
		generateOut = previousOut
		generateSettingsRaw = previousSettingsRaw
		generateDuration = previousDuration
		generateJSONOutput = previousJSONOutput
		generatePollInterval = previousPollInterval
		generateTimeout = previousTimeout
	})
}

func TestRunGenerateRejectsNonPositivePollIntervalBeforeAuth(t *testing.T) {
	withGenerateGlobals(t)
	generateOut = "image.png"
	generatePollInterval = 0
	generateTimeout = time.Minute

	err := runGenerate(nil, []string{"flux", "wide hero"})
	if err == nil {
		t.Fatal("expected invalid poll interval error")
	}
	if !strings.Contains(err.Error(), "--poll-interval") {
		t.Fatalf("expected poll interval error, got %v", err)
	}
}

func TestRunGenerateRejectsNonPositiveTimeoutBeforeAuth(t *testing.T) {
	withGenerateGlobals(t)
	generateOut = "image.png"
	generatePollInterval = time.Second
	generateTimeout = 0

	err := runGenerate(nil, []string{"flux", "wide hero"})
	if err == nil {
		t.Fatal("expected invalid timeout error")
	}
	if !strings.Contains(err.Error(), "--timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}
