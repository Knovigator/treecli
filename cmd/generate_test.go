package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/Knovigator/treectl/api"
)

func withGenerateGlobals(t *testing.T) {
	t.Helper()

	previousOut := generateOut
	previousSettingsRaw := generateSettingsRaw
	previousDuration := generateDuration
	previousJSONOutput := generateJSONOutput
	previousPollInterval := generatePollInterval
	previousTimeout := generateTimeout

	t.Cleanup(func() {
		generateOut = previousOut
		generateSettingsRaw = previousSettingsRaw
		generateDuration = previousDuration
		generateJSONOutput = previousJSONOutput
		generatePollInterval = previousPollInterval
		generateTimeout = previousTimeout
	})
}

func TestRunGenerateRejectsNonPositivePollIntervalBeforeAuth(t *testing.T) {
	withGenerateGlobals(t)
	generateOut = "image.png"
	generatePollInterval = 0
	generateTimeout = time.Minute

	err := runGenerate(nil, []string{"flux", "wide hero"})
	if err == nil {
		t.Fatal("expected invalid poll interval error")
	}
	if !strings.Contains(err.Error(), "--poll-interval") {
		t.Fatalf("expected poll interval error, got %v", err)
	}
}

func TestRunGenerateRejectsNonPositiveTimeoutBeforeAuth(t *testing.T) {
	withGenerateGlobals(t)
	generateOut = "image.png"
	generatePollInterval = time.Second
	generateTimeout = 0

	err := runGenerate(nil, []string{"flux", "wide hero"})
	if err == nil {
		t.Fatal("expected invalid timeout error")
	}
	if !strings.Contains(err.Error(), "--timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestGenerationActionRowsIncludesFullCatalogAndMarksDirectSupport(t *testing.T) {
	rows := generationActionRows(
		[]api.AIModelRef{
			{
				Name:             "flux-2-pro",
				HumanName:        "Flux 2 Pro",
				DescriptionShort: "Image generation",
				Provider:         "replicate",
				ModelType:        "image",
				ActionTagName:    "flux2",
			},
			{
				Name:          "veo-3",
				HumanName:     "Veo 3",
				Provider:      "google",
				ModelType:     "video",
				ActionTagName: "veo3",
			},
			{
				Name:          "openclaw-gateway",
				Provider:      "openclaw",
				ModelType:     "text",
				ActionTagName: "openclaw",
			},
		},
		[]api.TagInfo{
			{
				Tag:              "flux2",
				Provider:         "replicate",
				Kind:             "image",
				AcceptsReference: true,
				Inputs:           []string{"aspect_ratio"},
			},
			{
				Tag:                  "suno",
				Provider:             "suno",
				Kind:                 "audio",
				Async:                true,
				AcceptsReference:     true,
				SupportsInstrumental: true,
				DurationMin:          5,
				DurationMax:          240,
			},
			{
				Tag:      "openclaw",
				Provider: "openclaw",
				Kind:     "text",
			},
		},
		false,
	)

	byAction := map[string]generationActionRow{}
	for _, row := range rows {
		byAction[row.Action] = row
	}

	if _, ok := byAction["openclaw"]; ok {
		t.Fatal("expected hidden OpenClaw action to be omitted")
	}
	if got := byAction["flux2"]; !got.DirectGeneration || got.Name != "Flux 2 Pro" || !got.AcceptsReference {
		t.Fatalf("expected flux2 direct support with catalog metadata, got %#v", got)
	}
	if got := byAction["flux2"]; !hasSetting(got.Settings, "prompt") || !hasSetting(got.Settings, "aspect_ratio") {
		t.Fatalf("expected flux2 prompt and aspect_ratio setting help, got %#v", got.Settings)
	}
	if got := byAction["veo3"]; got.DirectGeneration || got.Kind != "video" {
		t.Fatalf("expected veo3 catalog row without direct support, got %#v", got)
	}
	if got := byAction["veo3"]; len(got.Notes) == 0 || !strings.Contains(got.Examples[0], "treectl action veo3") {
		t.Fatalf("expected veo3 post-backed guidance, got notes=%#v examples=%#v", got.Notes, got.Examples)
	}
	if got := byAction["suno"]; !got.DirectGeneration || !got.Async {
		t.Fatalf("expected direct-only suno row to be included, got %#v", got)
	}
	if got := byAction["suno"]; !hasSetting(got.Settings, "lyrics") || !hasSetting(got.Settings, "reference_url") {
		t.Fatalf("expected suno lyrics and reference_url setting help, got %#v", got.Settings)
	}
}

func TestGenerationActionRowsCanFilterToDirectOnly(t *testing.T) {
	rows := generationActionRows(
		[]api.AIModelRef{
			{HumanName: "Flux 2 Pro", Provider: "replicate", ModelType: "image", ActionTagName: "flux2"},
			{HumanName: "Veo 3", Provider: "google", ModelType: "video", ActionTagName: "veo3"},
		},
		[]api.TagInfo{{Tag: "flux2", Provider: "replicate", Kind: "image"}},
		true,
	)

	if len(rows) != 1 {
		t.Fatalf("expected only direct rows, got %#v", rows)
	}
	if rows[0].Action != "flux2" || !rows[0].DirectGeneration {
		t.Fatalf("expected flux2 direct row, got %#v", rows[0])
	}
}

func TestGenerateActionsCommandKeepsTagsCompatibilityAlias(t *testing.T) {
	found := false
	for _, alias := range generateActionsCmd.Aliases {
		if alias == "tags" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected generate actions to keep tags as a compatibility alias")
	}
}

func TestFindGenerationActionRowNormalizesBangPrefix(t *testing.T) {
	row, ok := findGenerationActionRow([]generationActionRow{{Action: "flux2"}}, "!FLUX2")
	if !ok {
		t.Fatal("expected to find action with bang-prefixed input")
	}
	if row.Action != "flux2" {
		t.Fatalf("expected flux2 row, got %#v", row)
	}
}

func TestCompleteGenerationActionRowNamesFiltersAndPreservesBangPrefix(t *testing.T) {
	rows := []generationActionRow{
		{Action: "flux2"},
		{Action: "veo3"},
		{Action: "suno"},
		{Action: "!flux2"},
	}

	got := completeGenerationActionRowNames(rows, "!f")

	if len(got) != 1 {
		t.Fatalf("expected one completion, got %#v", got)
	}
	if got[0] != "!flux2" {
		t.Fatalf("expected bang-prefixed flux2 completion, got %#v", got)
	}
}

func hasSetting(settings []settingHelp, name string) bool {
	for _, setting := range settings {
		if setting.Name == name {
			return true
		}
	}
	return false
}
