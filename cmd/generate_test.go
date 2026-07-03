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
				Tag:      "suno",
				Provider: "suno",
				Kind:     "audio",
				Async:    true,
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
	if got := byAction["veo3"]; got.DirectGeneration || got.Kind != "video" {
		t.Fatalf("expected veo3 catalog row without direct support, got %#v", got)
	}
	if got := byAction["suno"]; !got.DirectGeneration || !got.Async {
		t.Fatalf("expected direct-only suno row to be included, got %#v", got)
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
