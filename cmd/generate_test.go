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
	previousInputs := generateInputs
	previousReference := generateReference
	previousPaymentMode := generatePaymentMode
	previousInstrumental := generateInstrumental
	previousQuote := generateQuote

	t.Cleanup(func() {
		generateOut = previousOut
		generateSettingsRaw = previousSettingsRaw
		generateDuration = previousDuration
		generateJSONOutput = previousJSONOutput
		generatePollInterval = previousPollInterval
		generateTimeout = previousTimeout
		generateInputs = previousInputs
		generateReference = previousReference
		generatePaymentMode = previousPaymentMode
		generateInstrumental = previousInstrumental
		generateQuote = previousQuote
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

func TestParseGenerateInvocationPreservesAliasesForBackend(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedAction string
		expectedPrompt string
	}{
		{
			name:           "video sfx shorthand",
			args:           []string{"sfx", "rain", "on", "wet", "asphalt"},
			expectedAction: "sfx",
			expectedPrompt: "rain on wet asphalt",
		},
		{
			name:           "eleven labs numeric shorthand",
			args:           []string{"!11", "read", "this"},
			expectedAction: "!11",
			expectedPrompt: "read this",
		},
		{
			name:           "chatterbox shorthand",
			args:           []string{"chatterbox", "Brian", "read", "this"},
			expectedAction: "chatterbox",
			expectedPrompt: "Brian read this",
		},
		{
			name:           "canonical action",
			args:           []string{"!video_sfx", "foley", "pass"},
			expectedAction: "!video_sfx",
			expectedPrompt: "foley pass",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, prompt, err := parseGenerateInvocation(test.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if action != test.expectedAction {
				t.Fatalf("expected action %q, got %q", test.expectedAction, action)
			}
			if prompt != test.expectedPrompt {
				t.Fatalf("expected prompt %q, got %q", test.expectedPrompt, prompt)
			}
		})
	}
}

func TestGenerateActionsCommandKeepsHiddenTagsCompatibilityCommand(t *testing.T) {
	if generateTagsCompatCmd.Use != "tags" {
		t.Fatalf("expected hidden compatibility command to use tags, got %q", generateTagsCompatCmd.Use)
	}
	if !generateTagsCompatCmd.Hidden {
		t.Fatal("expected tags compatibility command to be hidden from help")
	}
}

func TestCompleteGenerationActionRowNamesFiltersAndPreservesBangPrefix(t *testing.T) {
	rows := []generationActionRow{
		{Action: "flux2"},
		{Action: "veo3"},
		{Action: "suno"},
		{Action: "!flux2"},
	}

	got := completeGenerationActionRowNames(rows, "!f")

	if len(got) != 1 {
		t.Fatalf("expected one completion, got %#v", got)
	}
	if got[0] != "!flux2" {
		t.Fatalf("expected bang-prefixed flux2 completion, got %#v", got)
	}
}

func TestCompleteGenerationActionRowNamesIncludesReplicateIntegrationAliases(t *testing.T) {
	rows := []generationActionRow{
		{Action: "tts", Aliases: []string{"chatterbox"}},
		{Action: "eleven_tts", Aliases: []string{"11"}},
		{Action: "video_sfx", Aliases: []string{"sfx"}},
	}

	got := completeGenerationActionRowNames(rows, "s")

	if len(got) != 1 || got[0] != "sfx" {
		t.Fatalf("expected sfx completion, got %#v", got)
	}

	got = completeGenerationActionRowNames(rows, "chat")
	if len(got) != 1 || got[0] != "chatterbox" {
		t.Fatalf("expected chatterbox completion, got %#v", got)
	}

	got = completeGenerationActionRowNames(rows, "video")
	if len(got) != 1 || got[0] != "video_sfx" {
		t.Fatalf("expected video_sfx completion, got %#v", got)
	}

	got = completeGenerationActionRowNames(rows, "!11")
	if len(got) != 1 || got[0] != "!11" {
		t.Fatalf("expected bang-prefixed 11 completion, got %#v", got)
	}
}
