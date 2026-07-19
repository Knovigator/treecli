package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chdirToTempDir(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting working directory: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("changing to temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	return tempDir
}

func TestUpsertOnboardBlockInFileCreatesAndStaysIdempotent(t *testing.T) {
	tempDir := chdirToTempDir(t)
	target := filepath.Join(tempDir, "AGENTS.md")

	action, err := upsertOnboardBlockInFile(target, "long")
	if err != nil {
		t.Fatalf("first write returned error: %v", err)
	}
	if action != "created" {
		t.Fatalf("expected created, got %q", action)
	}

	firstContent, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if !strings.Contains(string(firstContent), onboardBeginMarkerPrefix) || !strings.Contains(string(firstContent), onboardEndMarker) {
		t.Fatal("expected written file to contain begin and end markers")
	}

	action, err = upsertOnboardBlockInFile(target, "long")
	if err != nil {
		t.Fatalf("second write returned error: %v", err)
	}
	if action != "unchanged" {
		t.Fatalf("expected unchanged on re-run, got %q", action)
	}

	secondContent, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("re-reading written file: %v", err)
	}
	if string(firstContent) != string(secondContent) {
		t.Fatal("expected re-run to leave file byte-identical")
	}
	if strings.Count(string(secondContent), onboardBeginMarkerPrefix) != 1 {
		t.Fatal("expected exactly one guidance block after re-run")
	}
}

func TestUpsertOnboardBlockAppendsToExistingContent(t *testing.T) {
	tempDir := chdirToTempDir(t)
	target := filepath.Join(tempDir, "AGENTS.md")

	existing := "# My project\n\nHouse rules here.\n\n\n"
	if err := os.WriteFile(target, []byte(existing), 0644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	action, err := upsertOnboardBlockInFile(target, "short")
	if err != nil {
		t.Fatalf("write returned error: %v", err)
	}
	if action != "appended" {
		t.Fatalf("expected appended, got %q", action)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if !strings.HasPrefix(string(content), "# My project") {
		t.Fatal("expected existing content to be preserved at the top")
	}
	if !strings.HasPrefix(string(content), existing) {
		t.Fatal("expected append to preserve every existing byte")
	}
	if !strings.Contains(string(content), "variant=short") {
		t.Fatal("expected short variant marker")
	}
}

func TestValidateOnboardAgentsFlagsRejectsIgnoredOptions(t *testing.T) {
	testCases := []struct {
		name    string
		variant string
		write   bool
		check   bool
		file    string
	}{
		{name: "file without operation", file: "AGENTS.md"},
		{name: "variant with check", variant: "short", check: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := validateOnboardAgentsFlags(testCase.variant, testCase.write, testCase.check, testCase.file); err == nil {
				t.Fatal("expected invalid flag combination to return an error")
			}
		})
	}

	if err := validateOnboardAgentsFlags("short", true, false, "AGENTS.md"); err != nil {
		t.Fatalf("expected --write --short --file to be valid, got %v", err)
	}
	if err := validateOnboardAgentsFlags("", false, true, "AGENTS.md"); err != nil {
		t.Fatalf("expected --check --file to be valid, got %v", err)
	}
}

func TestValidateOnboardRootFlagsRejectsJSONWithLegacyMode(t *testing.T) {
	if err := validateOnboardRootFlags(true, true); err == nil {
		t.Fatal("expected --json with legacy onboarding flags to return an error")
	}
	if err := validateOnboardRootFlags(true, false); err != nil {
		t.Fatalf("expected standalone --json to be valid, got %v", err)
	}
}

func TestUpsertOnboardBlockUpdatesInPlaceAndSwitchesVariant(t *testing.T) {
	tempDir := chdirToTempDir(t)
	target := filepath.Join(tempDir, "CLAUDE.md")

	prefix := "before block\n\n"
	suffix := "\n\nafter block\n"
	staleBlock := onboardBeginMarkerPrefix + " variant=long (managed) -->\nold content\n" + onboardEndMarker
	if err := os.WriteFile(target, []byte(prefix+staleBlock+suffix), 0644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	action, err := upsertOnboardBlockInFile(target, "short")
	if err != nil {
		t.Fatalf("write returned error: %v", err)
	}
	if action != "updated" {
		t.Fatalf("expected updated, got %q", action)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if !strings.HasPrefix(string(content), "before block") || !strings.Contains(string(content), "after block") {
		t.Fatal("expected surrounding content to be preserved")
	}
	if strings.Contains(string(content), "old content") {
		t.Fatal("expected stale block content to be replaced")
	}
	if !strings.Contains(string(content), "variant=short") {
		t.Fatal("expected explicit --short to switch the recorded variant")
	}
	if strings.Count(string(content), onboardBeginMarkerPrefix) != 1 {
		t.Fatal("expected exactly one guidance block after update")
	}
}

func TestUpsertOnboardBlockPreservesRecordedVariantWhenUnspecified(t *testing.T) {
	tempDir := chdirToTempDir(t)
	target := filepath.Join(tempDir, "AGENTS.md")

	if _, err := upsertOnboardBlockInFile(target, "short"); err != nil {
		t.Fatalf("initial short write returned error: %v", err)
	}

	action, err := upsertOnboardBlockInFile(target, "")
	if err != nil {
		t.Fatalf("variant-less re-run returned error: %v", err)
	}
	if action != "unchanged" {
		t.Fatalf("expected variant-less re-run to preserve short variant, got %q", action)
	}
}

func TestUpsertOnboardBlockRejectsMissingEndMarker(t *testing.T) {
	tempDir := chdirToTempDir(t)
	target := filepath.Join(tempDir, "AGENTS.md")

	if err := os.WriteFile(target, []byte(onboardBeginMarkerPrefix+" variant=long -->\nno end marker\n"), 0644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	if _, err := upsertOnboardBlockInFile(target, "long"); err == nil {
		t.Fatal("expected error for begin marker without end marker")
	}
}

func TestOnboardBlockStateInFile(t *testing.T) {
	tempDir := chdirToTempDir(t)
	target := filepath.Join(tempDir, "AGENTS.md")

	state, _, err := onboardBlockStateInFile(target)
	if err != nil {
		t.Fatalf("state check on missing file returned error: %v", err)
	}
	if state != onboardBlockAbsent {
		t.Fatalf("expected absent for missing file, got %q", state)
	}

	if err := os.WriteFile(target, []byte("just some notes\n"), 0644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}
	state, _, err = onboardBlockStateInFile(target)
	if err != nil {
		t.Fatalf("state check without block returned error: %v", err)
	}
	if state != onboardBlockAbsent {
		t.Fatalf("expected absent without markers, got %q", state)
	}

	if _, err := upsertOnboardBlockInFile(target, "long"); err != nil {
		t.Fatalf("installing block returned error: %v", err)
	}
	state, variant, err := onboardBlockStateInFile(target)
	if err != nil {
		t.Fatalf("state check after install returned error: %v", err)
	}
	if state != onboardBlockCurrent || variant != "long" {
		t.Fatalf("expected current long block, got state %q variant %q", state, variant)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	tampered := strings.Replace(string(content), "## treecli CLI Guidance", "## tampered heading", 1)
	if tampered == string(content) {
		t.Fatal("expected block body to contain the guidance heading")
	}
	if err := os.WriteFile(target, []byte(tampered), 0644); err != nil {
		t.Fatalf("tampering file: %v", err)
	}
	state, _, err = onboardBlockStateInFile(target)
	if err != nil {
		t.Fatalf("state check after tampering returned error: %v", err)
	}
	if state != onboardBlockStale {
		t.Fatalf("expected stale after tampering, got %q", state)
	}
}

func TestOnboardAgentsCheckFailsWhenAnyManagedBlockIsStale(t *testing.T) {
	chdirToTempDir(t)

	if _, err := upsertOnboardBlockInFile("AGENTS.md", "long"); err != nil {
		t.Fatalf("installing current AGENTS.md block: %v", err)
	}
	if err := os.WriteFile(
		"CLAUDE.md",
		[]byte(onboardBeginMarkerPrefix+" variant=short -->\nstale\n"+onboardEndMarker+"\n"),
		0644,
	); err != nil {
		t.Fatalf("installing stale CLAUDE.md block: %v", err)
	}

	if err := runOnboardAgentsCheck(); err == nil {
		t.Fatal("expected check to fail while any managed guidance block is stale")
	}
}

func TestResolveOnboardWriteTargets(t *testing.T) {
	chdirToTempDir(t)

	targets, err := resolveOnboardWriteTargets("custom/path.md")
	if err != nil {
		t.Fatalf("explicit file returned error: %v", err)
	}
	if len(targets) != 1 || targets[0] != "custom/path.md" {
		t.Fatalf("expected explicit file to win, got %v", targets)
	}

	targets, err = resolveOnboardWriteTargets("")
	if err != nil {
		t.Fatalf("empty dir returned error: %v", err)
	}
	if len(targets) != 1 || targets[0] != "AGENTS.md" {
		t.Fatalf("expected default AGENTS.md in empty dir, got %v", targets)
	}

	if err := os.WriteFile("CLAUDE.md", []byte("# claude notes\n"), 0644); err != nil {
		t.Fatalf("seeding CLAUDE.md: %v", err)
	}
	targets, err = resolveOnboardWriteTargets("")
	if err != nil {
		t.Fatalf("existing CLAUDE.md returned error: %v", err)
	}
	if len(targets) != 1 || targets[0] != "CLAUDE.md" {
		t.Fatalf("expected existing CLAUDE.md to be preferred over creating AGENTS.md, got %v", targets)
	}

	if _, err := upsertOnboardBlockInFile("AGENTS.md", "long"); err != nil {
		t.Fatalf("installing block in AGENTS.md: %v", err)
	}
	if _, err := upsertOnboardBlockInFile("CLAUDE.md", "short"); err != nil {
		t.Fatalf("installing block in CLAUDE.md: %v", err)
	}
	targets, err = resolveOnboardWriteTargets("")
	if err != nil {
		t.Fatalf("both-with-block returned error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected both marked files to be refreshed, got %v", targets)
	}
}

func TestBuildOnboardNextStepsDropsCompletedItems(t *testing.T) {
	status := onboardStatus{
		Profile: onboardProfileStatus{Name: "prod", SignedIn: true},
		AgentFiles: []onboardFileStatus{
			{Path: "AGENTS.md", Exists: true, Block: string(onboardBlockCurrent), Variant: "long"},
		},
		Skills: onboardSkillsStatus{
			Packaged:  2,
			Installed: []onboardSkillTarget{{Target: "claude", Count: 2}},
		},
	}

	if steps := buildOnboardNextSteps(status); len(steps) != 0 {
		t.Fatalf("expected no next steps when everything is set up, got %v", steps)
	}

	status.Profile.SignedIn = false
	status.AgentFiles[0].Block = string(onboardBlockStale)
	status.Skills.Installed[0].Count = 0

	steps := buildOnboardNextSteps(status)
	if len(steps) != 3 {
		t.Fatalf("expected login, refresh, and skills steps, got %v", steps)
	}
	if !strings.Contains(steps[0].Command, "treecli login --profile prod") {
		t.Fatalf("expected login step first, got %q", steps[0].Command)
	}
	if !strings.Contains(steps[1].Command, "onboard agents --write") {
		t.Fatalf("expected guidance refresh step, got %q", steps[1].Command)
	}

	status.Profile.SignedIn = true
	status.AgentFiles[0].Block = string(onboardBlockCurrent)
	status.Skills.Installed[0].Count = 1
	steps = buildOnboardNextSteps(status)
	if len(steps) != 1 || !strings.Contains(steps[0].Command, "skills install all") {
		t.Fatalf("expected a partial skill install to remain incomplete, got %v", steps)
	}
}
