package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackagedSkillsAreCompleteAndInstallable(t *testing.T) {
	skills, err := ListPackagedSkills()
	if err != nil {
		t.Fatalf("ListPackagedSkills returned error: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("expected at least one packaged skill")
	}

	seenKeys := map[string]bool{}
	installRoot := t.TempDir()
	for _, skill := range skills {
		if skill.Key == "" || skill.Name == "" || skill.Description == "" {
			t.Fatalf("packaged skill is missing metadata: %#v", skill)
		}
		if seenKeys[skill.Key] {
			t.Fatalf("duplicate packaged skill key %q", skill.Key)
		}
		seenKeys[skill.Key] = true

		loaded, err := GetPackagedSkill(skill.Key)
		if err != nil {
			t.Fatalf("GetPackagedSkill(%q): %v", skill.Key, err)
		}
		if loaded == nil || loaded.Name != skill.Name {
			t.Fatalf("unexpected packaged skill lookup for %q: %#v", skill.Key, loaded)
		}

		installedDir, err := InstallPackagedSkill(skill, installRoot)
		if err != nil {
			t.Fatalf("InstallPackagedSkill(%q): %v", skill.Key, err)
		}
		assertFileContent(t, filepath.Join(installedDir, "SKILL.md"), skill.SkillMD)
		assertFileContent(t, filepath.Join(installedDir, "agents", "openai.yaml"), skill.OpenAIYAML)
	}
}

func TestBuildOnboardContentEmbedsRequestedVariant(t *testing.T) {
	longBlock, err := OnboardLong()
	if err != nil {
		t.Fatalf("OnboardLong returned error: %v", err)
	}
	shortBlock, err := OnboardShort()
	if err != nil {
		t.Fatalf("OnboardShort returned error: %v", err)
	}

	longContent, err := BuildOnboardContent("long")
	if err != nil {
		t.Fatalf("BuildOnboardContent(long): %v", err)
	}
	shortContent, err := BuildOnboardContent("short")
	if err != nil {
		t.Fatalf("BuildOnboardContent(short): %v", err)
	}

	for name, result := range map[string]struct {
		content string
		block   string
	}{
		"long":  {content: longContent, block: longBlock},
		"short": {content: shortContent, block: shortBlock},
	} {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(result.content, "{{AGENTS_MD_BLOCK}}") {
				t.Fatal("onboarding placeholder was not replaced")
			}
			if !strings.Contains(result.content, result.block) {
				t.Fatal("onboarding output did not contain the requested block")
			}
		})
	}
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read installed file %s: %v", path, err)
	}
	if string(actual) != expected {
		t.Fatalf("installed file %s did not match packaged content", path)
	}
}
