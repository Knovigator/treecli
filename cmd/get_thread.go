package cmd

import (
	"fmt"
	"net/url"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var getThreadCmd = &cobra.Command{
	Use:     "thread [thread_id]",
	Aliases: []string{"quest"},
	Short:   "Deprecated: use get threads <thread_id>",
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
	if cmd == nil {
		cmd = &cobra.Command{}
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Warning: get thread is deprecated; use get threads.")
	return fetchAndPrintThread(cmd, args[0], resolveOutputFormat(outputFormat, getJSONOutput))
}

func fetchAndPrintThread(cmd *cobra.Command, id, format string) error {
	if format != "ascii" && format != "json" {
		return invalidOutputFormatError(format)
	}
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}
	thread, err := api.GetThread(profile.BackendURL, url.PathEscape(id), profile.AccessToken, profile.Client, profile.UID)
	if err != nil {
		return err
	}
	if format == "json" {
		text, err := api.PrettyJSON(thread.Raw)
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), text)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), thread.Quest.ToASCII())
	}
	return nil
}
