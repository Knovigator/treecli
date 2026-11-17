package cmd

import (
	"testing"
	"time"
)

func TestNormalizeTeamTargetAcceptsIDsAndLinks(t *testing.T) {
	id := "7a5e85c9-9dca-4140-ba9a-f5db0030afca"
	for _, input := range []string{id, "https://app.treechat.com/stream/" + id, "https://app.treechat.com/stream/" + id + "/", "https://app.treechat.com/teams/" + id + "?tab=members"} {
		got, err := normalizeTeamTarget(input)
		if err != nil || got != id {
			t.Errorf("normalizeTeamTarget(%q) = %q, %v", input, got, err)
		}
	}
	if _, err := normalizeTeamTarget("my-stream"); err == nil {
		t.Fatal("non-uuid targets must be rejected")
	}
}

func TestParseSinceFlag(t *testing.T) {
	if since, err := parseSinceFlag(""); err != nil || !since.IsZero() {
		t.Fatalf("empty means no cutoff, got %v %v", since, err)
	}
	if since, err := parseSinceFlag("2026-09-01"); err != nil || since.Format("2006-01-02") != "2026-09-01" {
		t.Fatalf("date parse failed: %v %v", since, err)
	}
	since, err := parseSinceFlag("30d")
	if err != nil {
		t.Fatal(err)
	}
	if delta := time.Since(since); delta < 29*24*time.Hour || delta > 31*24*time.Hour {
		t.Fatalf("30d should be about a month ago, got %v", delta)
	}
	if _, err := parseSinceFlag("soon"); err == nil {
		t.Fatal("garbage must be rejected")
	}
}
