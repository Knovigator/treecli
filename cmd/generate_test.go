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
		[]api.GenerationActionInfo{
			{
				Action:           "flux2",
				Provider:         "replicate",
				Kind:             "image",
				AcceptsReference: true,
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
