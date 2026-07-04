package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestActionRequestsJSONStringBuildsStructuredModelRequest(t *testing.T) {
	payloadJSON, err := actionRequestsJSONString(actionInvocation{
		Tag:    "fluxfast",
		Prompt: "wide cover image",
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload []map[string]interface{}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatalf("invalid JSON payload: %v", err)
	}

	if len(payload) != 1 {
		t.Fatalf("expected one action request, got %d", len(payload))
	}

	action := payload[0]
	if action["id"] == "" {
		t.Fatal("expected generated id")
	}
	if action["client_id"] != action["id"] {
		t.Fatalf("expected client_id to match id, got id=%v client_id=%v", action["id"], action["client_id"])
	}
	if action["tag"] != "fluxfast" {
		t.Fatalf("expected tag fluxfast, got %v", action["tag"])
	}
	if action["prompt"] != "wide cover image" {
		t.Fatalf("expected prompt, got %v", action["prompt"])
	}
	if action["kind"] != "model" {
		t.Fatalf("expected model kind, got %v", action["kind"])
	}
	if action["generation_count"] != float64(1) {
		t.Fatalf("expected generation_count 1, got %v", action["generation_count"])
	}
	if _, present := action["settings"]; present {
		t.Fatalf("expected no settings when duration is unset, got %v", action["settings"])
	}
}

func TestActionRequestsJSONStringIncludesDurationSetting(t *testing.T) {
	payloadJSON, err := actionRequestsJSONString(actionInvocation{
		Tag:    "stableaudio",
		Prompt: "ambient build, 120 BPM",
	}, 90)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload []map[string]interface{}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatalf("invalid JSON payload: %v", err)
	}

	settings, ok := payload[0]["settings"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected settings object, got %v", payload[0]["settings"])
	}
	if settings["duration_seconds"] != float64(90) {
		t.Fatalf("expected duration_seconds 90, got %v", settings["duration_seconds"])
	}
}

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

func TestActionCommandKeepsHiddenTagsCompatibilityCommand(t *testing.T) {
	if actionTagsCompatCmd.Use != "tags" {
		t.Fatalf("expected hidden compatibility command to use tags, got %q", actionTagsCompatCmd.Use)
	}
	if !actionTagsCompatCmd.Hidden {
		t.Fatal("expected tags compatibility command to be hidden from help")
	}
}

func TestResolveRootThreadTargetRejectsAmbiguousPlacement(t *testing.T) {
	public := true

	_, err := resolveRootThreadTarget(profileConfig{}, rootThreadCreateOptions{
		Stream: "public",
		Public: &public,
	})
	if err == nil {
		t.Fatal("expected ambiguous placement flags to return an error")
	}
	if !strings.Contains(err.Error(), "--stream") || !strings.Contains(err.Error(), "--public") {
		t.Fatalf("expected error to mention conflicting flags, got %v", err)
	}
}
