package recorder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoverTranscriptsFindsClaudeAndCodexFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	claudeDir := filepath.Join(home, ".claude", "projects", "-Users-me-src-app")
	codexDir := filepath.Join(home, ".codex", "sessions", "2026", "09", "09")
	for _, dir := range []string{claudeDir, codexDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	old := filepath.Join(claudeDir, "old.jsonl")
	fresh := filepath.Join(claudeDir, "fresh.jsonl")
	rollout := filepath.Join(codexDir, "rollout-2026-09-09T10-00-00-abc.jsonl")
	for _, path := range []string{old, fresh, rollout} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stale := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(old, stale, stale); err != nil {
		t.Fatal(err)
	}
	claude, err := DiscoverTranscripts(AgentClaudeCode, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(claude) != 1 || claude[0].Path != fresh {
		t.Fatalf("expected only the fresh transcript, got %+v", claude)
	}
	all, err := DiscoverTranscripts(AgentClaudeCode, time.Time{})
	if err != nil || len(all) != 2 || all[0].Path != fresh {
		t.Fatalf("expected both transcripts newest first, got %+v (%v)", all, err)
	}
	codex, err := DiscoverTranscripts(AgentCodex, time.Time{})
	if err != nil || len(codex) != 1 || codex[0].Path != rollout {
		t.Fatalf("expected the rollout, got %+v (%v)", codex, err)
	}
	if _, err := DiscoverTranscripts(AgentGrok, time.Time{}); err == nil {
		t.Fatal("grok has no discoverable transcripts")
	}
}

func TestStoreMarkersAndTreechatStateRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMarker(Marker{Agent: AgentClaudeCode, SessionID: "a", CWD: "/x"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMarker(Marker{Agent: AgentCodex, SessionID: "b", CWD: "/y"}); err != nil {
		t.Fatal(err)
	}
	marker, ok, err := store.LoadMarker(AgentClaudeCode, "/x")
	if err != nil || !ok || marker.SessionID != "a" || marker.StartedAt.IsZero() {
		t.Fatalf("marker round trip failed: %+v %v %v", marker, ok, err)
	}
	if _, ok, _ := store.LoadMarker(AgentClaudeCode, "/nope"); ok {
		t.Fatal("unknown cwd must have no marker")
	}
	markers, err := store.ListMarkers()
	if err != nil || len(markers) != 2 {
		t.Fatalf("expected 2 markers, got %v (%v)", markers, err)
	}
	state, ok, err := store.LoadTreechatState(AgentCodex, "b")
	if err != nil || ok || state.Agent != AgentCodex || state.SessionID != "b" {
		t.Fatalf("missing state must return an empty seeded state: %+v %v %v", state, ok, err)
	}
	state.QuestID = "q"
	state.PostedTurns = 3
	if err := store.SaveTreechatState(state); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := store.LoadTreechatState(AgentCodex, "b")
	if err != nil || !ok || loaded.QuestID != "q" || loaded.PostedTurns != 3 {
		t.Fatalf("treechat state round trip failed: %+v %v %v", loaded, ok, err)
	}
	if store.RecordingPath(AgentGrok, "../evil") != filepath.Join(store.DataDir, "recordings", "grok", ".._evil.jsonl") {
		t.Fatalf("session ids must be sanitized in paths: %s", store.RecordingPath(AgentGrok, "../evil"))
	}
}
