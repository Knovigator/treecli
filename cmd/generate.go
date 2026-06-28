package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knovigator/treectl/api"
	"github.com/spf13/cobra"
)

var (
	generateOut          string
	generateSettingsRaw  string
	generateDuration     int
	generateJSONOutput   bool
	generatePollInterval time.Duration
	generateTimeout      time.Duration
)

// GenerateCmd generates AI media via the direct (post-less) API and saves it locally.
var GenerateCmd = &cobra.Command{
	Use:   "generate <tag> [prompt...]",
	Short: "Generate AI media directly and download it locally (never creates a post)",
	Long: "Generate AI media through the direct generation API and save it to a local file.\n\n" +
		"This charges your account (USD/BSV) and NEVER touches the posting infra — no Answer, " +
		"no Quest, no thread, nothing on your feed. Use it for brand/promo assets.\n\n" +
		"Stage 1 supports image tags (e.g. flux). Audio/video tags land in a later stage.",
	Example: "  treectl generate flux \"soft-gradient app icon, violet to indigo\" --out icon.png\n" +
		"  treectl generate flux \"friendly founder avatar portrait\" --out avatar.png\n" +
		"  treectl generate flux \"wide hero banner\" --out banner.png --settings '{\"aspect_ratio\":\"3:1\"}'",
	Args: cobra.MinimumNArgs(1),
	RunE: runGenerate,
}

func init() {
	GenerateCmd.Flags().StringVarP(&generateOut, "out", "o", "", "Path to write the generated file (required)")
	GenerateCmd.Flags().StringVar(&generateSettingsRaw, "settings", "", "Optional JSON object of model settings (e.g. '{\"aspect_ratio\":\"1:1\"}')")
	GenerateCmd.Flags().IntVar(&generateDuration, "duration", 0, "Duration in seconds for audio/video models (sets settings.duration_seconds)")
	GenerateCmd.Flags().BoolVar(&generateJSONOutput, "json", false, "Print the generation result as JSON")
	GenerateCmd.Flags().DurationVar(&generatePollInterval, "poll-interval", 3*time.Second, "Polling interval if the generation runs async")
	GenerateCmd.Flags().DurationVar(&generateTimeout, "timeout", 5*time.Minute, "Maximum time to wait for generated media")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	tag := strings.TrimPrefix(strings.TrimSpace(args[0]), "!")
	prompt := strings.TrimSpace(strings.Join(args[1:], " "))
	if tag == "" {
		return fmt.Errorf("an action tag is required")
	}
	if prompt == "" {
		return fmt.Errorf("a prompt is required")
	}
	if strings.TrimSpace(generateOut) == "" {
		return fmt.Errorf("--out <path> is required")
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

	settings := map[string]interface{}{}
	if strings.TrimSpace(generateSettingsRaw) != "" {
		if err := json.Unmarshal([]byte(generateSettingsRaw), &settings); err != nil {
			return fmt.Errorf("invalid --settings JSON: %w", err)
		}
	}
	if generateDuration > 0 {
		settings["duration_seconds"] = generateDuration
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	result, err := api.CreateGeneration(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		tag,
		prompt,
		settings,
		generateTimeout,
	)
	if err != nil {
		return err
	}

	// The endpoint is synchronous and returns the finished run; poll only if it ever
	// comes back still in flight (async path / future audio+video tags).
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

	data, err := api.DownloadMedia(
		result.MediaURLs[0],
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
	)
	if err != nil {
		return err
	}
	if err := os.WriteFile(generateOut, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", generateOut, err)
	}

	if generateJSONOutput {
		payload := map[string]interface{}{
			"id":         result.ID,
			"status":     result.Status,
			"tag":        result.Tag,
			"out":        generateOut,
			"bytes":      len(data),
			"media_urls": result.MediaURLs,
		}
		encoded, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(string(encoded))
	} else {
		fmt.Printf("Saved %d bytes to %s (run %s)\n", len(data), generateOut, result.ID)
	}
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
