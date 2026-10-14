package cmd

import "testing"

func TestParseActionInvocationSeparatesTagAndPrompt(t *testing.T) {
	invocation, err := parseActionInvocation([]string{"!nb", "make", "this", "warmer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if invocation.Tag != "nb" {
		t.Fatalf("expected tag nb, got %q", invocation.Tag)
	}
	if invocation.Prompt != "make this warmer" {
		t.Fatalf("expected prompt, got %q", invocation.Prompt)
	}
	if invocation.NormalizedContent != "!nb make this warmer" {
		t.Fatalf("expected normalized content, got %q", invocation.NormalizedContent)
	}
}

func TestParseActionInvocationPreservesAliasesForBackend(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedTag     string
		expectedPrompt  string
		expectedContent string
	}{
		{
			name:            "video sfx shorthand",
			args:            []string{"sfx", "rain", "on", "wet", "asphalt"},
			expectedTag:     "sfx",
			expectedPrompt:  "rain on wet asphalt",
			expectedContent: "!sfx rain on wet asphalt",
		},
		{
			name:            "eleven labs numeric shorthand",
			args:            []string{"!11 read this in a warm voice"},
			expectedTag:     "11",
			expectedPrompt:  "read this in a warm voice",
			expectedContent: "!11 read this in a warm voice",
		},
		{
			name:            "eleven labs word shorthand",
			args:            []string{"elevenlabs", "short", "narration"},
			expectedTag:     "elevenlabs",
			expectedPrompt:  "short narration",
			expectedContent: "!elevenlabs short narration",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocation, err := parseActionInvocation(test.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if invocation.Tag != test.expectedTag {
				t.Fatalf("expected tag %q, got %q", test.expectedTag, invocation.Tag)
			}
			if invocation.Prompt != test.expectedPrompt {
				t.Fatalf("expected prompt %q, got %q", test.expectedPrompt, invocation.Prompt)
			}
			if invocation.NormalizedContent != test.expectedContent {
				t.Fatalf("expected normalized content %q, got %q", test.expectedContent, invocation.NormalizedContent)
			}

		})
	}
}

func TestActionCommandKeepsHiddenTagsCompatibilityCommand(t *testing.T) {
	if actionTagsCompatCmd.Use != "tags" {
		t.Fatalf("expected hidden compatibility command to use tags, got %q", actionTagsCompatCmd.Use)
	}
	if !actionTagsCompatCmd.Hidden {
		t.Fatal("expected tags compatibility command to be hidden from help")
	}
}
