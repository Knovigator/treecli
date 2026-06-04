package cmd

import "testing"

func TestResolveLeaderboardDateRangePrefersExplicitDates(t *testing.T) {
	startDate, endDate, err := resolveLeaderboardDateRange("last-week", "2026-05-25", "2026-06-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if startDate != "2026-05-25" || endDate != "2026-06-01" {
		t.Fatalf("expected explicit dates, got start=%q end=%q", startDate, endDate)
	}
}

func TestResolveLeaderboardDateRangeNormalizesCompactExplicitDates(t *testing.T) {
	startDate, endDate, err := resolveLeaderboardDateRange("last-week", "20260525", "20260601")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if startDate != "2026-05-25" || endDate != "2026-06-01" {
		t.Fatalf("expected normalized dates, got start=%q end=%q", startDate, endDate)
	}
}

func TestResolveLeaderboardDateRangeRejectsInvalidExplicitDate(t *testing.T) {
	_, _, err := resolveLeaderboardDateRange("last-week", "2026-99-99", "")
	if err == nil {
		t.Fatal("expected invalid explicit date to return an error")
	}
}

func TestResolveLeaderboardDateRangeRejectsUnknownRange(t *testing.T) {
	_, _, err := resolveLeaderboardDateRange("quarter-ish", "", "")
	if err == nil {
		t.Fatal("expected unknown range to return an error")
	}
}
