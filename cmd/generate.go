package cmd

import (
	"encoding/json"
	"fmt"
	neturl "net/url"
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
	Args:              cobra.MinimumNArgs(1),
	RunE:              runGenerate,
	ValidArgsFunction: completeGenerateArgs,
}

func init() {
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
	action, prompt, err := parseGenerateInvocation(args)
	if err != nil {
		return err
	}
	if !generateQuote && strings.TrimSpace(generateOut) == "" {
		return fmt.Errorf("--out <path> is required (or use --quote to just see the price)")
	}
	if generateDuration < 0 {
		return fmt.Errorf("--duration must be zero or greater")
	}
	if generatePollInterval <= 0 {
		return fmt.Errorf("--poll-interval must be greater than zero")
	}
	if generateTimeout <= 0 {
		return fmt.Errorf("--timeout must be greater than zero")
	}
	paymentMode, err := normalizePaymentMode(generatePaymentMode)
	if err != nil {
		return err
	}

	settings := map[string]interface{}{}
	if strings.TrimSpace(generateSettingsRaw) != "" {
		if err := json.Unmarshal([]byte(generateSettingsRaw), &settings); err != nil {
			return fmt.Errorf("invalid --settings JSON: %w", err)
		}
	}
	if err := applyInputs(settings, generateInputs); err != nil {
		return err
	}
	if generateDuration > 0 {
		settings["duration_seconds"] = generateDuration
	}
	if cmd.Flags().Changed("instrumental") {
		settings["instrumental"] = generateInstrumental
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	// Quote-only: ask the backend for the price and stop.
	if generateQuote {
		res, err := api.CreateGeneration(
			profile.BackendURL, profile.AccessToken, profile.Client, profile.UID,
			action, prompt, settings, paymentMode, true, generateTimeout,
		)
		if err != nil {
			return err
		}
		return printQuote(action, res)
	}

	if err := resolveFileInputSettings(profile, settings); err != nil {
		return err
	}
	if strings.TrimSpace(generateReference) != "" {
		ref, err := resolveReference(profile, generateReference)
		if err != nil {
			return err
		}
		applyReferenceSettings(settings, ref)
	}

	result, err := api.CreateGeneration(
		profile.BackendURL, profile.AccessToken, profile.Client, profile.UID,
		action, prompt, settings, paymentMode, false, generateTimeout,
	)
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
		return "", "", fmt.Errorf("an AI action is required")
	}

	action := canonicalAIActionName(args[0])
	prompt := strings.TrimSpace(strings.Join(args[1:], " "))
	if action == "" {
		return "", "", fmt.Errorf("an AI action is required")
	}
	if prompt == "" {
		return "", "", fmt.Errorf("a prompt is required")
	}

	return action, prompt, nil
}

func completeGenerateArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return completeGenerationActionNames(toComplete, true)
}

type resolvedReference struct {
	URL         string
	ContentType string
	Kind        string
}

// resolveReference turns a --reference value into a URL the model can fetch. `run:<id>` reuses a
// prior generation's first output; a public URL passes through; @file uploads a local media file.
func resolveReference(profile profileConfig, ref string) (resolvedReference, error) {
	ref = strings.TrimSpace(ref)
	switch {
	case strings.HasPrefix(ref, "run:"):
		id := strings.TrimSpace(strings.TrimPrefix(ref, "run:"))
		if id == "" {
			return resolvedReference{}, fmt.Errorf("invalid --reference: run id is empty")
		}
		res, err := api.GetGeneration(profile.BackendURL, id, profile.AccessToken, profile.Client, profile.UID)
		if err != nil {
			return resolvedReference{}, fmt.Errorf("resolving reference run %s: %w", id, err)
		}
		media, ok := firstGenerationMedia(res)
		if !ok {
			return resolvedReference{}, fmt.Errorf("reference run %s has no output media (status %q)", id, res.Status)
		}
		return media, nil
	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return resolvedReference{URL: ref, Kind: mediaKindFromURL(ref)}, nil
	case strings.HasPrefix(ref, "@"):
		path := strings.TrimSpace(strings.TrimPrefix(ref, "@"))
		if path == "" {
			return resolvedReference{}, fmt.Errorf("invalid --reference: @path is empty")
		}
		uploaded, err := uploadReferenceFile(profile, path)
		if err != nil {
			return resolvedReference{}, fmt.Errorf("uploading reference %s: %w", path, err)
		}
		return uploaded, nil
	default:
		return resolvedReference{}, fmt.Errorf("invalid --reference %q: use run:<id>, a public URL, or @path", ref)
	}
}

func firstGenerationMedia(res api.GenerationResponse) (resolvedReference, bool) {
	for _, media := range res.MediaOutputs {
		if strings.TrimSpace(media.URL) == "" {
			continue
		}
		return resolvedReference{
			URL:         media.URL,
			ContentType: media.ContentType,
			Kind:        firstNonBlank(media.Kind, mediaKindFromContentType(media.ContentType), mediaKindFromURL(media.URL)),
		}, true
	}
	for _, mediaURL := range res.MediaURLs {
		if strings.TrimSpace(mediaURL) == "" {
			continue
		}
		return resolvedReference{URL: mediaURL, Kind: mediaKindFromURL(mediaURL)}, true
	}
	return resolvedReference{}, false
}

func uploadReferenceFile(profile profileConfig, path string) (resolvedReference, error) {
	res, err := api.UploadReference(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, path)
	if err != nil {
		return resolvedReference{}, err
	}
	return resolvedReference{
		URL:         res.URL,
		ContentType: res.ContentType,
		Kind:        firstNonBlank(res.Kind, mediaKindFromContentType(res.ContentType), mediaKindFromURL(res.URL)),
	}, nil
}

func applyReferenceSettings(settings map[string]interface{}, ref resolvedReference) {
	if strings.TrimSpace(ref.URL) == "" {
		return
	}
	settings["reference_url"] = ref.URL
	if strings.TrimSpace(ref.ContentType) != "" {
		settings["reference_content_type"] = ref.ContentType
	}
	if strings.TrimSpace(ref.Kind) != "" {
		settings["reference_kind"] = ref.Kind
	}
}

func resolveFileInputSettings(profile profileConfig, settings map[string]interface{}) error {
	for key, value := range settings {
		resolved, refs, err := resolveFileInputValue(profile, value)
		if err != nil {
			return fmt.Errorf("resolving --input %s: %w", key, err)
		}
		settings[key] = resolved
		if len(refs) == 0 {
			continue
		}
		firstRef := refs[0]
		if strings.TrimSpace(firstRef.ContentType) != "" {
			settings[key+"_content_type"] = firstRef.ContentType
		}
		if strings.TrimSpace(firstRef.Kind) != "" {
			settings[key+"_kind"] = firstRef.Kind
		}
		if isReferenceInputKey(key) {
			if _, exists := settings["reference_url"]; !exists {
				applyReferenceSettings(settings, firstRef)
			}
		}
	}
	return nil
}

func resolveFileInputValue(profile profileConfig, value interface{}) (interface{}, []resolvedReference, error) {
	switch typed := value.(type) {
	case string:
		if !strings.HasPrefix(strings.TrimSpace(typed), "@") {
			return value, nil, nil
		}
		path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(typed), "@"))
		if path == "" {
			return value, nil, fmt.Errorf("@path is empty")
		}
		ref, err := uploadReferenceFile(profile, path)
		if err != nil {
			return value, nil, err
		}
		return ref.URL, []resolvedReference{ref}, nil
	case []interface{}:
		resolved := make([]interface{}, 0, len(typed))
		refs := []resolvedReference{}
		for _, item := range typed {
			resolvedItem, itemRefs, err := resolveFileInputValue(profile, item)
			if err != nil {
				return value, nil, err
			}
			resolved = append(resolved, resolvedItem)
			refs = append(refs, itemRefs...)
		}
		return resolved, refs, nil
	case map[string]interface{}:
		resolved := make(map[string]interface{}, len(typed))
		refs := []resolvedReference{}
		for itemKey, item := range typed {
			resolvedItem, itemRefs, err := resolveFileInputValue(profile, item)
			if err != nil {
				return value, nil, err
			}
			resolved[itemKey] = resolvedItem
			refs = append(refs, itemRefs...)
		}
		return resolved, refs, nil
	default:
		return value, nil, nil
	}
}

func isReferenceInputKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "reference", "reference_url", "image", "video", "audio", "input_image",
		"reference_image", "reference_video", "reference_audio", "start_image",
		"start_image_url", "first_frame_image", "input_reference":
		return true
	default:
		return false
	}
}

func mediaKindFromContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	case strings.HasPrefix(contentType, "audio/"):
		return "audio"
	default:
		return ""
	}
}

func mediaKindFromURL(rawURL string) string {
	parsed, err := neturl.Parse(rawURL)
	path := strings.ToLower(rawURL)
	if err == nil && parsed.Path != "" {
		path = strings.ToLower(parsed.Path)
	}
	switch filepath.Ext(path) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".heic":
		return "image"
	case ".mp4", ".mov", ".webm", ".m4v":
		return "video"
	case ".mp3", ".wav", ".m4a", ".aac", ".flac", ".ogg":
		return "audio"
	default:
		return ""
	}
}

// applyInputs merges repeatable --input key=value pairs into settings, parsing each value as JSON
// when possible (so numbers, bools, arrays and objects work) and falling back to a string.
func applyInputs(settings map[string]interface{}, inputs []string) error {
	for _, kv := range inputs {
		i := strings.Index(kv, "=")
		if i <= 0 {
			return fmt.Errorf("invalid --input %q: expected key=value", kv)
		}
		key := strings.TrimSpace(kv[:i])
		if key == "" {
			return fmt.Errorf("invalid --input %q: empty key", kv)
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

func printQuote(action string, res api.GenerationResponse) error {
	sats := res.AmountSats
	usd := res.AmountUSD
	if res.Quote != nil {
		sats = res.Quote.AmountSats
		usd = res.Quote.AmountUSD
	}
	paymentMode := res.PaymentMode
	if res.Quote != nil && res.Quote.PaymentMode != "" {
		paymentMode = res.Quote.PaymentMode
	}
	if generateJSONOutput {
		payload := map[string]interface{}{
			"action":      action,
			"tag":         action,
			"quote":       true,
			"amount_sats": sats,
			"amount_usd":  usd,
			"provider":    res.Provider,
		}
		if paymentMode != "" {
			payload["payment_mode"] = paymentMode
		}
		encoded, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		return nil
	}
	paymentSuffix := ""
	if paymentMode != "" {
		paymentSuffix = " via " + paymentModeDisplay(paymentMode)
	}
	fmt.Printf("Quote for %q%s: %d sats ($%.4f)\n", action, paymentSuffix, sats, usd)
	return nil
}

func pollGeneration(profile profileConfig, id string) (api.GenerationResponse, error) {
	deadline := time.Now().Add(generateTimeout)
	for {
		res, err := api.GetGeneration(profile.BackendURL, id, profile.AccessToken, profile.Client, profile.UID)
		if err != nil {
			return api.GenerationResponse{}, err
		}
		switch res.Status {
		case "succeeded", "failed", "canceled":
			return res, nil
		}
		if time.Now().After(deadline) {
			return res, fmt.Errorf("timed out waiting for generation %s", id)
		}
		time.Sleep(generatePollInterval)
	}
}
