package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knovigator/treecli/api"
)

func withGenerateGlobals(t *testing.T) {
	t.Helper()

	previousOut := generateOut
	previousSettingsRaw := generateSettingsRaw
	previousDuration := generateDuration
	previousJSONOutput := generateJSONOutput
	previousPollInterval := generatePollInterval
	previousTimeout := generateTimeout
	previousInputs := generateInputs
	previousReference := generateReference
	previousPaymentMode := generatePaymentMode
	previousInstrumental := generateInstrumental
	previousQuote := generateQuote

	t.Cleanup(func() {
		generateOut = previousOut
		generateSettingsRaw = previousSettingsRaw
		generateDuration = previousDuration
		generateJSONOutput = previousJSONOutput
		generatePollInterval = previousPollInterval
		generateTimeout = previousTimeout
		generateInputs = previousInputs
		generateReference = previousReference
		generatePaymentMode = previousPaymentMode
		generateInstrumental = previousInstrumental
		generateQuote = previousQuote
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

func TestRunGenerateRejectsLegacyReferenceActionsBeforeAuth(t *testing.T) {
	withGenerateGlobals(t)
	generatePollInterval = time.Second
	generateTimeout = time.Minute

	err := runGenerate(nil, []string{"animate_kling", "slow", "push-in"})
	if err == nil {
		t.Fatal("expected legacy action error")
	}
	for _, want := range []string{"animate_kling", "legacy image-to-video action", "treecli generate kling2", "--reference @image.png"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "--out <path> is required") {
		t.Fatalf("expected legacy action guidance before output validation, got %v", err)
	}
}

func TestParseGenerateInvocationCanonicalizesReplicateIntegrationAliases(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedAction string
		expectedPrompt string
	}{
		{
			name:           "video sfx shorthand",
			args:           []string{"sfx", "rain", "on", "wet", "asphalt"},
			expectedAction: "video_sfx",
			expectedPrompt: "rain on wet asphalt",
		},
		{
			name:           "eleven labs numeric shorthand",
			args:           []string{"!11", "read", "this"},
			expectedAction: "eleven_tts",
			expectedPrompt: "read this",
		},
		{
			name:           "chatterbox shorthand",
			args:           []string{"chatterbox", "Brian", "read", "this"},
			expectedAction: "tts",
			expectedPrompt: "Brian read this",
		},
		{
			name:           "canonical action",
			args:           []string{"!video_sfx", "foley", "pass"},
			expectedAction: "video_sfx",
			expectedPrompt: "foley pass",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, prompt, err := parseGenerateInvocation(test.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if action != test.expectedAction {
				t.Fatalf("expected action %q, got %q", test.expectedAction, action)
			}
			if prompt != test.expectedPrompt {
				t.Fatalf("expected prompt %q, got %q", test.expectedPrompt, prompt)
			}
		})
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
			{
				Name:          "qwen-image-edit",
				Provider:      "replicate",
				ModelType:     "image",
				ActionTagName: "edit_qwen",
			},
		},
		[]api.GenerationActionInfo{
			{
				Action:           "flux2",
				Provider:         "replicate",
				Kind:             "image",
				AcceptsReference: true,
				ReferenceKinds:   []string{"image"},
				Inputs:           []string{"aspect_ratio"},
			},
			{
				Action:               "suno",
				Provider:             "suno",
				Kind:                 "audio",
				Async:                true,
				AcceptsReference:     true,
				SupportsInstrumental: true,
				DurationMin:          5,
				DurationMax:          240,
			},
			{
				Action:   "openclaw",
				Provider: "openclaw",
				Kind:     "text",
			},
			{
				Action:   "animate_kling",
				Provider: "replicate",
				Kind:     "video",
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
	if _, ok := byAction["edit_qwen"]; ok {
		t.Fatal("expected legacy edit action to be omitted from generate rows")
	}
	if _, ok := byAction["animate_kling"]; ok {
		t.Fatal("expected legacy animate action to be omitted from generate rows")
	}
	if got := byAction["flux2"]; !got.DirectGeneration || got.Name != "Flux 2 Pro" || !got.AcceptsReference {
		t.Fatalf("expected flux2 direct support with catalog metadata, got %#v", got)
	}
	if got := byAction["flux2"]; got.RequiresReference || len(got.ReferenceKinds) != 1 || got.ReferenceKinds[0] != "image" {
		t.Fatalf("expected flux2 optional image reference metadata, got %#v", got)
	}
	if got := byAction["flux2"]; !hasSetting(got.Settings, "prompt") || !hasSetting(got.Settings, "aspect_ratio") {
		t.Fatalf("expected flux2 prompt and aspect_ratio setting help, got %#v", got.Settings)
	}
	if got := byAction["veo3"]; got.DirectGeneration || got.Kind != "video" {
		t.Fatalf("expected veo3 catalog row without direct support, got %#v", got)
	}
	if got := byAction["veo3"]; len(got.Notes) == 0 || !strings.Contains(got.Examples[0], "treecli action veo3") {
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
		[]api.GenerationActionInfo{{Action: "flux2", Provider: "replicate", Kind: "image"}},
		true,
	)

	if len(rows) != 1 {
		t.Fatalf("expected only direct rows, got %#v", rows)
	}
	if rows[0].Action != "flux2" || !rows[0].DirectGeneration {
		t.Fatalf("expected flux2 direct row, got %#v", rows[0])
	}
}

func TestVideoSFXDirectGenerationHelpRequiresVideoReference(t *testing.T) {
	row := enrichGenerationActionRow(generationActionRow{
		Action:           "video_sfx",
		Provider:         "replicate",
		Kind:             "audio",
		DirectGeneration: true,
		Async:            true,
		AcceptsReference: true,
	})

	if len(row.Examples) == 0 {
		t.Fatal("expected video_sfx examples")
	}
	for _, example := range row.Examples {
		if strings.Contains(example, "treecli generate video_sfx \"prompt\" --out output.mp3") {
			t.Fatalf("video_sfx should not advertise prompt-only generation, got examples %#v", row.Examples)
		}
	}
	if !strings.Contains(row.Examples[0], "--reference @clip.mp4") || !strings.Contains(row.Examples[0], "--out sfx.mp3") {
		t.Fatalf("expected first video_sfx example to use a video reference and audio output, got %#v", row.Examples)
	}
	if !hasSetting(row.Settings, "negative_prompt") || !hasSetting(row.Settings, "cfg_strength") {
		t.Fatalf("expected video_sfx provider setting help, got %#v", row.Settings)
	}
	for _, setting := range row.Settings {
		if setting.Name == "prompt" && !strings.Contains(setting.Example, "--reference @clip.mp4") {
			t.Fatalf("expected video_sfx prompt example to include a video reference, got %#v", setting)
		}
	}
	if len(row.Notes) == 0 || !strings.Contains(strings.Join(row.Notes, "\n"), "requires --reference with video media") {
		t.Fatalf("expected video_sfx reference note, got %#v", row.Notes)
	}
}

func TestChatterboxTTSDirectGenerationHelpIncludesProviderSettings(t *testing.T) {
	row := enrichGenerationActionRow(generationActionRow{
		Action:           "tts",
		Provider:         "replicate",
		Kind:             "audio",
		DirectGeneration: true,
		Async:            true,
	})

	if len(row.Examples) == 0 || !strings.Contains(row.Examples[0], "Abigail read this") {
		t.Fatalf("expected Chatterbox TTS example, got %#v", row.Examples)
	}
	if !hasSetting(row.Settings, "voice") || !hasSetting(row.Settings, "exaggeration") || !hasSetting(row.Settings, "cfg_weight") || !hasSetting(row.Settings, "temperature") {
		t.Fatalf("expected Chatterbox provider setting help, got %#v", row.Settings)
	}
}

func TestChatterboxCloneDirectGenerationHelpRequiresAudioReference(t *testing.T) {
	row := enrichGenerationActionRow(generationActionRow{
		Action:           "clone",
		Provider:         "replicate",
		Kind:             "audio",
		DirectGeneration: true,
		Async:            true,
		AcceptsReference: true,
	})

	if len(row.Examples) == 0 {
		t.Fatal("expected clone examples")
	}
	if !strings.Contains(row.Examples[0], "--reference @voice.mp3") || !strings.Contains(row.Examples[0], "--out clone.mp3") {
		t.Fatalf("expected first clone example to use an audio reference and audio output, got %#v", row.Examples)
	}
	for _, example := range row.Examples {
		if strings.Contains(example, "@reference.png") {
			t.Fatalf("clone should not advertise a generic image reference, got examples %#v", row.Examples)
		}
	}
	if hasSetting(row.Settings, "voice") || !hasSetting(row.Settings, "reference_url") || !hasSetting(row.Settings, "cfg_weight") {
		t.Fatalf("expected clone reference and provider setting help without voice, got %#v", row.Settings)
	}
	for _, setting := range row.Settings {
		if setting.Name == "prompt" && !strings.Contains(setting.Example, "--reference @voice.mp3") {
			t.Fatalf("expected clone prompt example to include an audio reference, got %#v", setting)
		}
	}
	if len(row.Notes) == 0 || !strings.Contains(strings.Join(row.Notes, "\n"), "requires --reference with audio media") {
		t.Fatalf("expected clone reference note, got %#v", row.Notes)
	}
}

func TestOptionalImageReferenceDirectGenerationHelpUsesBaseActionExamples(t *testing.T) {
	row := enrichGenerationActionRow(generationActionRow{
		Action:           "qwen",
		Provider:         "replicate",
		Kind:             "image",
		DirectGeneration: true,
		AcceptsReference: true,
		ReferenceKinds:   []string{"image"},
	})

	if len(row.Examples) < 2 {
		t.Fatalf("expected qwen prompt-only and image reference examples, got %#v", row.Examples)
	}
	if !strings.Contains(row.Examples[0], "treecli generate qwen \"prompt\" --out output.png") {
		t.Fatalf("expected first qwen example to allow prompt-only generation, got %#v", row.Examples)
	}
	if !strings.Contains(row.Examples[1], "--reference @image.png") {
		t.Fatalf("expected second qwen example to use an image reference, got %#v", row.Examples)
	}
	if !hasSetting(row.Settings, "reference_url") {
		t.Fatalf("expected qwen reference setting help, got %#v", row.Settings)
	}
	if got := referenceSummary(row); got != "yes:image" {
		t.Fatalf("expected optional image reference summary, got %q", got)
	}
	if strings.Contains(strings.Join(row.Notes, "\n"), "requires --reference with image media") {
		t.Fatalf("did not expect required-reference note for qwen, got %#v", row.Notes)
	}
}

func TestGenerateActionsCommandKeepsHiddenTagsCompatibilityCommand(t *testing.T) {
	if generateTagsCompatCmd.Use != "tags" {
		t.Fatalf("expected hidden compatibility command to use tags, got %q", generateTagsCompatCmd.Use)
	}
	if !generateTagsCompatCmd.Hidden {
		t.Fatal("expected tags compatibility command to be hidden from help")
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

func TestFindGenerationActionRowCanonicalizesReplicateIntegrationAliases(t *testing.T) {
	tests := []struct {
		query    string
		expected string
	}{
		{query: "sfx", expected: "video_sfx"},
		{query: "!11", expected: "eleven_tts"},
		{query: "chatterbox", expected: "tts"},
	}

	rows := []generationActionRow{
		{Action: "tts"},
		{Action: "eleven_tts"},
		{Action: "video_sfx"},
	}

	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			row, ok := findGenerationActionRow(rows, test.query)
			if !ok {
				t.Fatalf("expected to find %q", test.query)
			}
			if row.Action != test.expected {
				t.Fatalf("expected %q row, got %#v", test.expected, row)
			}
		})
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

func TestCompleteGenerationActionRowNamesIncludesReplicateIntegrationAliases(t *testing.T) {
	rows := []generationActionRow{
		{Action: "tts"},
		{Action: "eleven_tts"},
		{Action: "video_sfx"},
	}

	got := completeGenerationActionRowNames(rows, "s")

	if len(got) != 1 || got[0] != "sfx" {
		t.Fatalf("expected sfx completion, got %#v", got)
	}

	got = completeGenerationActionRowNames(rows, "chat")
	if len(got) != 1 || got[0] != "chatterbox" {
		t.Fatalf("expected chatterbox completion, got %#v", got)
	}

	got = completeGenerationActionRowNames(rows, "video")
	if len(got) != 1 || got[0] != "video_sfx" {
		t.Fatalf("expected video_sfx completion, got %#v", got)
	}

	got = completeGenerationActionRowNames(rows, "!11")
	if len(got) != 1 || got[0] != "!11" {
		t.Fatalf("expected bang-prefixed 11 completion, got %#v", got)
	}
}

func TestFirstGenerationMediaPrefersTypedMediaOutputs(t *testing.T) {
	ref, ok := firstGenerationMedia(api.GenerationResponse{
		MediaURLs: []string{"https://cdn.example.test/fallback.png"},
		MediaOutputs: []api.GenerationMedia{
			{
				URL:         "https://cdn.example.test/source.mp4",
				ContentType: "video/mp4",
			},
		},
	})

	if !ok {
		t.Fatal("expected media reference")
	}
	if ref.URL != "https://cdn.example.test/source.mp4" || ref.Kind != "video" || ref.ContentType != "video/mp4" {
		t.Fatalf("unexpected reference: %#v", ref)
	}
}

func TestResolveFileInputSettingsUploadsReferenceLikeInput(t *testing.T) {
	uploadSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ai/generations/references/direct_upload" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/api/v1/ai/generations/references" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("parsing multipart: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected file upload: %v", err)
		}
		defer file.Close()
		uploadSeen = true
		if got := header.Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("expected image/png upload content type, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ref-1","url":"https://cdn.example.test/ref.png","content_type":"image/png","kind":"image"}`))
	}))
	defer server.Close()

	referenceFile := t.TempDir() + "/frame.png"
	if err := os.WriteFile(referenceFile, []byte("\x89PNG\r\n\x1a\n"), 0o644); err != nil {
		t.Fatalf("writing temp reference: %v", err)
	}

	settings := map[string]interface{}{"image": "@" + referenceFile}
	err := resolveFileInputSettings(profileConfig{BackendURL: server.URL}, settings)
	if err != nil {
		t.Fatalf("resolveFileInputSettings returned error: %v", err)
	}
	if !uploadSeen {
		t.Fatal("expected upload endpoint to be called")
	}
	if settings["image"] != "https://cdn.example.test/ref.png" {
		t.Fatalf("expected image input URL, got %#v", settings["image"])
	}
	if settings["reference_url"] != "https://cdn.example.test/ref.png" {
		t.Fatalf("expected reference_url to be seeded, got %#v", settings["reference_url"])
	}
	if settings["reference_content_type"] != "image/png" || settings["reference_kind"] != "image" {
		t.Fatalf("expected reference media metadata, got %#v", settings)
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
