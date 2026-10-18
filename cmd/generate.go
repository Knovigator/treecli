package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var (
	generateOut          string
	generateSettingsRaw  string
	generateDuration     int
	generateJSONOutput   bool
	generatePollInterval time.Duration
	generateTimeout      time.Duration
	generateInputs       []string
	generateReference    string
	generatePaymentMode  string
	generateInstrumental bool
	generateQuote        bool
)

// GenerateCmd generates AI media via the direct (post-less) API and saves it locally.
var GenerateCmd = &cobra.Command{
	Use:   "generate <ai-action> [prompt...]",
	Short: "Generate AI media directly and download it locally (never creates a post)",
	Long: "Generate AI media through the direct generation API and save it to a local file.\n\n" +
		"This charges your account (USD/BSV) and NEVER touches the posting infra — no Answer, " +
		"no Quest, no thread, nothing on your feed.\n\n" +
		"Pass arbitrary model inputs with repeatable --input key=value (values are parsed as JSON " +
		"when possible, else treated as a string). Use @path in --input values to upload local " +
		"images, video, or audio before generation. Chain generations or steer a model with " +
		"--reference (run:<id> reuses a prior generation's output as the model's reference; " +
		"@path uploads a local file; a public URL is passed through). Music models accept " +
		"--instrumental and --duration. Use --payment usd or --payment bsv to choose the " +
		"payment rail for this generation; omit it to use your account default.\n\n" +
		"Run `treecli generate actions --verbose` or `treecli generate describe <ai-action>` " +
		"to see available AI actions, descriptions, settings, and examples.",
	Example: "  treecli generate flux \"soft-gradient app icon, violet to indigo\" --out icon.png\n" +
		"  treecli generate flux2 \"wide hero banner\" --out banner.webp --input aspect_ratio=3:1\n" +
		"  treecli generate kling2 \"slow handheld push-in\" --reference @image.png --out animated.mp4\n" +
		"  treecli generate qwen \"replace the sky with stars\" --reference @image.png --out edited.png\n" +
		"  treecli generate topazimg --reference @photo.jpg --input upscale_factor=4x --out photo-4x.jpg\n" +
		"  treecli generate topazvid --reference @clip.mp4 --input target_resolution=4k --quote --max-price-cents 100\n" +
		"  treecli generate tts \"Abigail read this in a crisp narration voice\" --out chatterbox.mp3\n" +
		"  treecli generate clone \"read this in the sampled voice\" --reference @voice.mp3 --out clone.mp3\n" +
		"  treecli generate eleven_tts \"read this in a crisp narration voice\" --out narration.mp3\n" +
		"  treecli generate sfx \"rain, tires on wet asphalt, distant thunder\" --reference @clip.mp4 --out sfx.mp3\n" +
		"  treecli generate suno \"warm ambient build, 122 BPM\" --duration 20 --out sketch.mp3\n" +
		"  treecli generate suno \"cinematic electronic, builds to a drop\" --instrumental --duration 22 \\\n" +
		"      --reference run:abc123 --out track.mp3\n" +
		"  treecli generate suno \"...\" --duration 22 --quote\n" +
		"  treecli generate actions --direct-only\n" +
		"  treecli generate describe flux2",
	Args: func(command *cobra.Command, args []string) error {
		if generateQuoteID != "" {
			return nil
		}
		return cobra.MinimumNArgs(1)(command, args)
	},
	RunE:              runGenerate,
	ValidArgsFunction: completeGenerateArgs,
}

var generateMaxPriceCents int
var generateQuoteID, generateRequestID string

func init() {
	GenerateCmd.Flags().IntVar(&generateMaxPriceCents, "max-price-cents", 0, "Maximum generation fee in USD cents; required")
	GenerateCmd.Flags().StringVar(&generateQuoteID, "quote-id", "", "Execute this reviewed quote; reuse with --id on retries")
	GenerateCmd.Flags().StringVar(&generateRequestID, "id", "", "Generation idempotency key; reuse with --quote-id on retries")
	GenerateCmd.Flags().StringVarP(&generateOut, "out", "o", "", "Path to write the generated file (required unless --quote). Extra outputs get a -N suffix.")
	GenerateCmd.Flags().StringVar(&generateSettingsRaw, "settings", "", "Optional JSON object of model settings (e.g. '{\"aspect_ratio\":\"1:1\"}')")
	GenerateCmd.Flags().StringArrayVar(&generateInputs, "input", nil, "Model input as key=value (repeatable). Value parsed as JSON if possible, else string. Use @path to upload a local media file.")
	GenerateCmd.Flags().StringVar(&generateReference, "reference", "", "Reference media: run:<id> (chain a prior generation's output), a public URL, or @path (upload a local file)")
	GenerateCmd.Flags().StringVar(&generatePaymentMode, "payment", "", "Payment rail for this generation: usd or bsv/bitcoinsv (default: account setting)")
	GenerateCmd.Flags().BoolVar(&generateInstrumental, "instrumental", false, "For music models: generate instrumental (no vocals)")
	GenerateCmd.Flags().IntVar(&generateDuration, "duration", 0, "Duration in seconds for audio/video models (sets settings.duration_seconds; the backend clamps to the model's range)")
	GenerateCmd.Flags().BoolVar(&generateQuote, "quote", false, "Print the price for this generation without generating anything")
	GenerateCmd.Flags().BoolVar(&generateJSONOutput, "json", false, "Print the result as JSON")
	GenerateCmd.Flags().DurationVar(&generatePollInterval, "poll-interval", 3*time.Second, "Polling interval if the generation runs async")
	GenerateCmd.Flags().DurationVar(&generateTimeout, "timeout", 5*time.Minute, "Maximum time to wait for generated media")
	GenerateCmd.AddCommand(generateActionsCmd)
	GenerateCmd.AddCommand(generateTagsCompatCmd)
	GenerateCmd.AddCommand(generateDescribeCmd)
}

func runGenerate(cmd *cobra.Command, args []string) error {
	if generateQuoteID != "" && generateQuote {
		return usageErrorf("--quote and --quote-id cannot be combined; --quote never executes a generation")
	}
	action, prompt := "", ""
	var err error
	if generateQuoteID == "" {
		action, prompt, err = parseGenerateInvocation(args)
		if err != nil {
			return err
		}
	}
	// Media-only actions (upscalers) run from --reference alone; the backend still requires a
	// prompt for every other action.
	if !generateQuote && strings.TrimSpace(generateOut) == "" {
		return usageErrorf("--out <path> is required (or use --quote to just see the price)")
	}
	if generateDuration < 0 {
		return usageErrorf("--duration must be zero or greater")
	}
	if generatePollInterval <= 0 {
		return usageErrorf("--poll-interval must be greater than zero")
	}
	if generateTimeout <= 0 {
		return usageErrorf("--timeout must be greater than zero")
	}
	paymentMode, err := normalizePaymentMode(generatePaymentMode)
	if err != nil {
		return err
	}

	settings := map[string]interface{}{}
	if strings.TrimSpace(generateSettingsRaw) != "" {
		if err := json.Unmarshal([]byte(generateSettingsRaw), &settings); err != nil {
			return usageErrorf("invalid --settings JSON: %w", err)
		}
	}
	if err := applyInputs(settings, generateInputs); err != nil {
		return err
	}
	if generateDuration > 0 {
		settings["duration_seconds"] = generateDuration
	}
	if cmd != nil && cmd.Flags().Changed("instrumental") {
		settings["instrumental"] = generateInstrumental
	}

	if generateQuoteID == "" && ((cmd == nil || !cmd.Flags().Changed("max-price-cents")) || generateMaxPriceCents < 0) {
		return usageErrorf("--max-price-cents is required and must be nonnegative")
	}
	if generateQuoteID != "" && generateRequestID == "" {
		return usageErrorf("--id is required when executing --quote-id")
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	quoteID := generateQuoteID
	if quoteID == "" {
		if err := resolveFileInputSettings(profile, settings); err != nil {
			return err
		}
		reference := generateReference
		if strings.HasPrefix(reference, "@") {
			uploaded, err := uploadSharedFile(profile, strings.TrimPrefix(reference, "@"), "reference")
			if err != nil {
				return err
			}
			if stringValue(uploaded["id"]) == "" {
				return fmt.Errorf("reference upload returned no run ID")
			}
			reference = "run:" + stringValue(uploaded["id"])
		}
		arguments := map[string]interface{}{"model": action, "prompt": prompt, "settings": settings, "max_price_cents": generateMaxPriceCents}
		optional(arguments, "reference", reference)
		optional(arguments, "payment_mode", paymentMode)
		raw, err := api.SharedOperation(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, "quote_generation", arguments)
		if err != nil {
			return err
		}
		var quote map[string]interface{}
		if err := json.Unmarshal(raw, &quote); err != nil {
			return err
		}
		quoteID = stringValue(quote["quote_id"])
		if quoteID == "" {
			return fmt.Errorf("backend omitted generation quote ID")
		}
		if generateQuote {
			fmt.Println(string(raw))
			return nil
		}
		fmt.Fprintf(os.Stderr, "Generation quote %s: %s\n", quoteID, string(raw))
	}
	key := generateRequestID
	if key == "" {
		key, err = newUUID()
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "Retry identifiers: --quote-id %s --id %s\n", quoteID, key)
	raw, err := api.SharedOperation(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, "create_generation", map[string]interface{}{"quote_id": quoteID, "idempotency_key": key})
	if err != nil {
		return err
	}
	result, err := api.DecodeGeneration(raw)
	if err != nil {
		return err
	}

	// The endpoint may return a finished run or one still in flight; poll until it settles.
	switch result.Status {
	case "pending", "running", "submitted":
		result, err = pollGeneration(profile, result.ID)
		if err != nil {
			return err
		}
	}

	if result.Status != "succeeded" || len(result.MediaURLs) == 0 {
		reason := ""
		if result.Failure != nil {
			if encoded, marshalErr := json.Marshal(result.Failure); marshalErr == nil {
				reason = string(encoded)
			}
		}
		return fmt.Errorf("generation did not succeed (status %q) %s", result.Status, reason)
	}

	written, totalBytes, err := writeOutputs(generateOut, result.MediaURLs, profile)
	if err != nil {
		return err
	}

	if generateJSONOutput {
		actionName := firstNonBlank(result.Action, result.Tag, action)
		payload := map[string]interface{}{
			"id":         result.ID,
			"status":     result.Status,
			"action":     actionName,
			"tag":        actionName,
			"provider":   result.Provider,
			"out":        written,
			"bytes":      totalBytes,
			"media_urls": result.MediaURLs,
		}
		if len(result.MediaOutputs) > 0 {
			payload["media_outputs"] = result.MediaOutputs
		}
		if result.AmountSats > 0 || result.AmountUSD > 0 {
			payload["amount_sats"] = result.AmountSats
			payload["amount_usd"] = result.AmountUSD
		}
		if result.PaymentMode != "" {
			payload["payment_mode"] = result.PaymentMode
		}
		encoded, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(string(encoded))
	} else {
		details := []string{}
		if result.AmountSats > 0 || result.AmountUSD > 0 {
			details = append(details, fmt.Sprintf("charged %d sats ($%.4f)", result.AmountSats, result.AmountUSD))
		}
		if result.PaymentMode != "" {
			details = append(details, "payment: "+paymentModeDisplay(result.PaymentMode))
		}
		suffix := ""
		if len(details) > 0 {
			suffix = " — " + strings.Join(details, ", ")
		}
		fmt.Printf("Saved %d bytes to %s (run %s)%s\n", totalBytes, strings.Join(written, ", "), result.ID, suffix)
	}
	return nil
}

func parseGenerateInvocation(args []string) (string, string, error) {
	if len(args) == 0 {
		return "", "", usageErrorf("an AI action is required")
	}

	action := strings.TrimSpace(args[0])
	prompt := strings.TrimSpace(strings.Join(args[1:], " "))
	if action == "" {
		return "", "", usageErrorf("an AI action is required")
	}

	return action, prompt, nil
}

func completeGenerateArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return completeGenerationActionNames(toComplete, true)
}

func resolveFileInputSettings(profile profileConfig, settings map[string]interface{}) error {
	for key, value := range settings {
		resolved, err := uploadLocalInputs(profile, value)
		if err != nil {
			return err
		}
		settings[key] = resolved
	}
	return nil
}

// applyInputs merges repeatable --input key=value pairs into settings, parsing each value as JSON
// when possible (so numbers, bools, arrays and objects work) and falling back to a string.
func applyInputs(settings map[string]interface{}, inputs []string) error {
	for _, kv := range inputs {
		i := strings.Index(kv, "=")
		if i <= 0 {
			return usageErrorf("invalid --input %q: expected key=value", kv)
		}
		key := strings.TrimSpace(kv[:i])
		if key == "" {
			return usageErrorf("invalid --input %q: empty key", kv)
		}
		raw := kv[i+1:]
		var parsed interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			settings[key] = parsed
		} else {
			settings[key] = raw
		}
	}
	return nil
}

// writeOutputs downloads every media URL to disk. The first goes to `base`; extras get a -N suffix
// before the extension (e.g. track.mp3 -> track-2.mp3).
func writeOutputs(base string, urls []string, profile profileConfig) ([]string, int, error) {
	written := []string{}
	total := 0
	for idx, u := range urls {
		data, err := api.DownloadMedia(u, profile.BackendURL, profile.AccessToken, profile.Client, profile.UID)
		if err != nil {
			return written, total, err
		}
		path := base
		if idx > 0 {
			ext := filepath.Ext(base)
			path = strings.TrimSuffix(base, ext) + fmt.Sprintf("-%d", idx+1) + ext
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return written, total, fmt.Errorf("writing %s: %w", path, err)
		}
		written = append(written, path)
		total += len(data)
	}
	return written, total, nil
}

func pollGeneration(profile profileConfig, id string) (api.GenerationResponse, error) {
	started := time.Now()
	deadline := started.Add(generateTimeout)
	lastStatus, lastReport := "", time.Time{}
	for {
		res, err := api.GetGeneration(profile.BackendURL, id, profile.AccessToken, profile.Client, profile.UID)
		if err != nil {
			return api.GenerationResponse{}, err
		}
		switch res.Status {
		case "succeeded", "failed", "canceled":
			return res, nil
		}
		// Progress goes to stderr so stdout stays clean JSON for agents.
		if res.Status != lastStatus || time.Since(lastReport) >= 15*time.Second {
			fmt.Fprintf(os.Stderr, "Generation %s: %s (%ds)\n", id, res.Status, int(time.Since(started).Seconds()))
			lastStatus, lastReport = res.Status, time.Now()
		}
		if time.Now().After(deadline) {
			return res, fmt.Errorf("timed out waiting for generation %s", id)
		}
		time.Sleep(generatePollInterval)
	}
}
