package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Knovigator/treectl/api"
	"github.com/spf13/cobra"
)

var generateTagsJSON bool

// generateTagsCmd lists the model tags the direct generation endpoint supports + what each accepts.
var generateTagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "List the model tags available for direct generation and what inputs each accepts",
	Long: "List the model tags the direct (post-less) generation endpoint supports, including each " +
		"tag's provider, media kind, whether it runs async, whether it accepts a --reference, " +
		"whether it supports --instrumental, its duration range, and notable inputs.",
	Args: cobra.NoArgs,
	RunE: runGenerateTags,
}

func init() {
	generateTagsCmd.Flags().BoolVar(&generateTagsJSON, "json", false, "Print the tags as JSON")
}

func runGenerateTags(cmd *cobra.Command, args []string) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	tags, err := api.ListGenerationTags(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID)
	if err != nil {
		return err
	}

	if generateTagsJSON {
		encoded, err := json.MarshalIndent(tags, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(string(encoded))
		return nil
	}

	if len(tags) == 0 {
		fmt.Println("No generation tags available.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TAG\tPROVIDER\tKIND\tASYNC\tREF\tINSTR\tDURATION\tINPUTS")
	for _, t := range tags {
		duration := "-"
		if t.DurationMax > 0 {
			duration = fmt.Sprintf("%d-%ds", t.DurationMin, t.DurationMax)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			t.Tag, t.Provider, dash(t.Kind), yesno(t.Async), yesno(t.AcceptsReference),
			yesno(t.SupportsInstrumental), duration, dash(strings.Join(t.Inputs, ",")))
	}
	return w.Flush()
}

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "-"
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
