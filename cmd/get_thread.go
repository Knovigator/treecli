package cmd

import (
	"fmt"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var getThreadCmd = &cobra.Command{
	Use:     "thread [thread_id]",
	Aliases: []string{"quest"},
	Short:   "Get information about a specific thread",
	Long:    `Fetch and display information about a thread using its ID.`,
	Args:    cobra.ExactArgs(1),
	RunE:    runGetThread,
}

var noRehydrate bool

// var outputFormat string

func init() {
	getThreadCmd.Flags().BoolVarP(&noRehydrate, "no-rehydrate", "n", false, "Deprecated: quest responses already include hydrated parent and sorted_answers")
	getThreadCmd.Flags().StringVarP(&outputFormat, "output", "o", "ascii", "Output format: ascii or json")
	getThreadCmd.Flags().BoolVar(&getJSONOutput, "json", false, "Output JSON instead of human-readable text")
}

func runGetThread(cmd *cobra.Command, args []string) error {
	threadID := args[0]

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	threadInfo, err := api.GetThread(profile.BackendURL, threadID, profile.AccessToken, profile.Client, profile.UID)
	if err != nil {
		return err
	}

	switch resolveOutputFormat(outputFormat, getJSONOutput) {
	case "json":
		prettyJSON, err := api.PrettyJSON(threadInfo.Raw)
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(prettyJSON)
	case "ascii":
		fmt.Println(threadInfo.Quest.ToASCII())
	default:
		return invalidOutputFormatError(outputFormat)
	}

	return nil
}
