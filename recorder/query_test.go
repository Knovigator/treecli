package recorder

import "testing"

func TestFTSQueryTurnsPromptsIntoORTerms(t *testing.T) {
	cases := map[string]string{
		"where does the share reward report live?":      "share OR reward OR report OR live",
		"Fix the failing spec in answers_controller.rb": "fix OR failing OR spec OR answers_controller",
		"the the a an":            "",
		"":                        "",
		"deploy":                  "deploy",
		`"share reward" OR daily`: `"share reward" OR daily`,
		"upvalue AND settlement":  "upvalue AND settlement",
	}
	for input, expected := range cases {
		if got := FTSQuery(input); got != expected {
			t.Errorf("FTSQuery(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestFTSQueryCapsTermCount(t *testing.T) {
	query := FTSQuery("alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron")
	if got := len(splitOR(query)); got != 12 {
		t.Fatalf("expected 12 terms, got %d (%q)", got, query)
	}
}

func splitOR(query string) []string {
	parts := []string{}
	for _, part := range []byte(query) {
		_ = part
	}
	current := ""
	for _, token := range splitFields(query) {
		if token == "OR" {
			parts = append(parts, current)
			current = ""
			continue
		}
		current = token
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func splitFields(value string) []string {
	fields := []string{}
	current := ""
	for _, r := range value {
		if r == ' ' {
			if current != "" {
				fields = append(fields, current)
				current = ""
			}
			continue
		}
		current += string(r)
	}
	if current != "" {
		fields = append(fields, current)
	}
	return fields
}
