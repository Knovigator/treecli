package cmd

import (
	"testing"

	"github.com/Knovigator/treecli/api"
)

func TestUpvalueDirection(t *testing.T) {
	tests := []struct {
		name      string
		upvalue   api.BsvUpvalue
		currentID string
		expected  string
	}{
		{name: "received", upvalue: api.BsvUpvalue{FromUserID: "other", ToUserID: "me"}, currentID: "me", expected: "received"},
		{name: "sent", upvalue: api.BsvUpvalue{FromUserID: "me", ToUserID: "other"}, currentID: "me", expected: "sent"},
		{name: "boosted", upvalue: api.BsvUpvalue{FromUserID: "me", ToUserID: "system", IsBoost: true}, currentID: "me", expected: "boosted"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := upvalueDirection(test.upvalue, test.currentID); actual != test.expected {
				t.Fatalf("expected %q, got %q", test.expected, actual)
			}
		})
	}
}

func TestFormatSats(t *testing.T) {
	tests := map[string]string{
		"":          "0",
		"9696.0":    "9,696",
		"40000.000": "40,000",
		"12.5":      "12.5",
	}

	for input, expected := range tests {
		if actual := formatSats(input); actual != expected {
			t.Fatalf("formatSats(%q): expected %q, got %q", input, expected, actual)
		}
	}
}
