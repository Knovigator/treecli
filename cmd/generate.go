package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
		"Set --max-price-cents for each action. An idempotency key is generated when --id is omitted; reuse the printed key on retries. The shared backend enforces the action cap and connected application rolling 24-hour budget.\n" +
		"Use `treecli billing budget` to inspect the daily cap and next capacity release. --quote is an optional preview.\n\n" +
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
	Example: "  treecli generate flux2 \"wide hero banner\" --out banner.webp --max-price-cents 100 --id banner-001\n" +
		"  treecli generate topazimg --reference @photo.jpg --out photo-4x.jpg --max-price-cents 100 --id upscale-001\n" +
		"  treecli generate suno \"ambient build\" --duration 20 --quote --max-price-cents 100\n" +
		"  treecli generate --quote-id REVIEWED_QUOTE_ID --id music-001 --out music.mp3\n" +
		"  treecli billing budget\n" +
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
	GenerateCmd.Flags().StringVar(&generateRequestID, "id", "", "Optional generation idempotency key (default: generated UUID); reuse the printed key with identical inputs on retries")
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
	if settings == nil {
		return usageErrorf("--settings must be a JSON object")
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

	if generateQuoteID == "" && ((cmd == nil || !cmd.Flags().Changed("max-price-cents")) || generateMaxPriceCents < 0 || int64(generateMaxPriceCents) > 2147483647) {
		return usageErrorf("--max-price-cents is required and must be between 0 and 2147483647 USD cents")
	}
	if generateRequestID != "" && !regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`).MatchString(generateRequestID) {
		return usageErrorf("invalid --id: use 1-128 ASCII letters, digits, dots, underscores, colons or hyphens; reuse it with identical inputs on retries")
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	key := generateRequestID
	if !generateQuote && key == "" {
		key, err = newUUID()
		if err != nil {
			return err
		}
	}
	tool := "generate_media"
	arguments := map[string]interface{}{"idempotency_key": key}
	if generateQuoteID != "" {
		tool = "create_generation"
		arguments["quote_id"] = generateQuoteID
	} else {
		originalSettings, _ := json.Marshal(settings)
		if err := resolveFileInputSettings(profile, settings); err != nil {
			return err
		}
		resolvedSettings, _ := json.Marshal(settings)
		if string(originalSettings) != string(resolvedSettings) {
			fmt.Fprintf(os.Stderr, "Uploaded input settings (reuse with --settings on retries): %s\n", resolvedSettings)
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
			fmt.Fprintf(os.Stderr, "Uploaded reference: %s (reuse this reference with --id on retries)\n", reference)
		}
		arguments["model"] = action
		arguments["prompt"] = prompt
		arguments["settings"] = settings
		arguments["max_price_cents"] = generateMaxPriceCents
		optional(arguments, "reference", reference)
		optional(arguments, "payment_mode", paymentMode)
		if generateQuote {
			tool = "quote_generation"
			delete(arguments, "idempotency_key")
		}
	}
	if !generateQuote {
		fmt.Fprintf(os.Stderr, "Generation idempotency key: %s; reuse with identical inputs on retries.\n", key)
	}
	raw, err := api.SharedOperation(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, tool, arguments)
	if err != nil {
		return err
	}
	if generateQuote {
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
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
