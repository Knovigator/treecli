package cmd

import "testing"

func TestCanonicalAIActionNameMapsReplicateIntegrationAliases(t *testing.T) {
	tests := map[string]string{
		"11":         "eleven_tts",
		"!eleven":    "eleven_tts",
		"ElevenLabs": "eleven_tts",
		"sfx":        "video_sfx",
		"!mmaudio":   "video_sfx",
		"Foley":      "video_sfx",
		"!flux2":     "flux2",
	}

	for input, expected := range tests {
		if got := canonicalAIActionName(input); got != expected {
			t.Fatalf("canonicalAIActionName(%q) = %q, expected %q", input, got, expected)
		}
	}
}

func TestAIActionCompletionCandidatesIncludesReplicateIntegrationAliases(t *testing.T) {
	tests := []struct {
		action   string
		expected []string
	}{
		{
			action:   "eleven_tts",
			expected: []string{"eleven_tts", "11", "eleven", "elevenlabs"},
		},
		{
			action:   "video_sfx",
			expected: []string{"video_sfx", "foley", "mmaudio", "sfx"},
		},
		{
			action:   "sfx",
			expected: []string{"sfx", "video_sfx", "foley", "mmaudio"},
		},
	}

	for _, test := range tests {
		t.Run(test.action, func(t *testing.T) {
			candidates := aiActionCompletionCandidates(test.action)
			for _, expected := range test.expected {
				if !stringSliceContains(candidates, expected) {
					t.Fatalf("expected %q in completion candidates %v", expected, candidates)
				}
			}
		})
	}
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}

	return false
}
