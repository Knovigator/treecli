package cmd

import (
	"sort"
	"strings"
)

var canonicalAIActionAliases = map[string]string{
	"11":         "eleven_tts",
	"chatterbox": "tts",
	"eleven":     "eleven_tts",
	"elevenlabs": "eleven_tts",
	"foley":      "video_sfx",
	"mmaudio":    "video_sfx",
	"sfx":        "video_sfx",
}

func canonicalAIActionName(action string) string {
	normalized := normalizedActionName(action)
	if canonical, ok := canonicalAIActionAliases[normalized]; ok {
		return canonical
	}

	return normalized
}

func aiActionAliasesForCanonicalAction(action string) []string {
	canonical := canonicalAIActionName(action)
	aliases := []string{}
	for alias, aliasCanonical := range canonicalAIActionAliases {
		if aliasCanonical == canonical {
			aliases = append(aliases, alias)
		}
	}

	sort.Strings(aliases)
	return aliases
}

func aiActionCompletionCandidates(action string) []string {
	trimmed := strings.TrimSpace(action)
	if trimmed == "" {
		return nil
	}

	candidates := []string{}
	seen := map[string]bool{}
	for _, candidate := range append([]string{trimmed, canonicalAIActionName(trimmed)}, aiActionAliasesForCanonicalAction(trimmed)...) {
		normalized := normalizedActionName(candidate)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		candidates = append(candidates, candidate)
	}
	return candidates
}

func prefixAIActionNames(actions []string, includeBang bool) []string {
	prefixed := make([]string, 0, len(actions))
	for _, action := range actions {
		if includeBang {
			prefixed = append(prefixed, "!"+action)
			continue
		}
		prefixed = append(prefixed, action)
	}

	return prefixed
}
