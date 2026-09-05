package cmd

import (
	"fmt"
	"strings"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

func newGetThreadsCommand() *cobra.Command {
	var user, answer, output string
	var root, branch, jsonOutput bool
	var page, limit int
	command := &cobra.Command{
		Use:     "threads [thread_id]",
		Short:   "List authored threads or fetch a thread or an answer's child threads",
		Long:    "List your authored threads, newest-created first, or fetch by thread ID or --answer. Listing includes accessible private content and defaults to --user me. Roots have no containing thread; branches grow from a message in another thread. Page numbers can shift when threads are added or removed.",
		Example: "  treecli get threads --user me --root --limit 1\n  treecli get threads --user alice --branch --page 2 --limit 20\n  treecli get threads THREAD_ID --json\n  treecli get threads --answer ANSWER_ID --json",
		Args:    cobra.MaximumNArgs(1),
	}
	flags := command.Flags()
	flags.StringVar(&user, "user", "me", "Author username, user ID, or me (prefix @ to force a username)")
	flags.StringVar(&answer, "answer", "", "Fetch all accessible child threads of this answer ID")
	flags.BoolVar(&root, "root", false, "List only root threads")
	flags.BoolVar(&branch, "branch", false, "List only branch threads")
	flags.IntVar(&page, "page", 1, "Page number, starting at 1")
	flags.IntVar(&limit, "limit", 20, "Threads per page (1–100)")
	flags.StringVarP(&output, "output", "o", "ascii", "Output format: ascii or json")
	flags.BoolVar(&jsonOutput, "json", false, "Output JSON instead of human-readable text")
	command.RunE = func(cmd *cobra.Command, args []string) error {
		format := resolveOutputFormat(output, jsonOutput)
		if format != "ascii" && format != "json" {
			return invalidOutputFormatError(output)
		}
		if root && branch {
			return fmt.Errorf("--root and --branch are mutually exclusive")
		}
		if flags.Changed("answer") && strings.TrimSpace(answer) == "" {
			return fmt.Errorf("--answer must not be empty")
		}
		if len(args) == 1 && strings.TrimSpace(args[0]) == "" {
			return fmt.Errorf("thread ID must not be empty")
		}
		if len(args) == 1 && flags.Changed("answer") {
			return fmt.Errorf("thread ID and --answer are mutually exclusive")
		}
		lookup := len(args) == 1 || flags.Changed("answer")
		if lookup {
			for _, name := range []string{"user", "root", "branch", "page", "limit"} {
				if flags.Changed(name) {
					return fmt.Errorf("--%s cannot be combined with a thread ID or --answer", name)
				}
			}
		} else {
			if strings.TrimSpace(user) == "" {
				return fmt.Errorf("--user must not be empty")
			}
			if page < 1 || limit < 1 || limit > 100 {
				return fmt.Errorf("--page must be positive and --limit must be between 1 and 100")
			}
		}
		if len(args) == 1 {
			return fetchAndPrintThread(cmd, args[0], format)
		}
		profile, err := requireAuthenticatedProfile()
		if err != nil {
			return err
		}
		var result api.ThreadsResponse
		if flags.Changed("answer") {
			result, err = api.ThreadsForAnswer(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, answer)
		} else {
			scope := "all"
			if root {
				scope = "root"
			}
			if branch {
				scope = "branch"
			}
			result, err = api.ListThreads(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, api.ThreadListOptions{User: user, Scope: scope, Page: page, Limit: limit})
		}
		if err != nil {
			return err
		}
		if format == "json" {
			text, err := api.PrettyJSON(result.Raw)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), text)
		} else {
			for _, thread := range result.Threads {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n%s\n\n", thread.ID, thread.ToASCII())
			}
			if len(result.Threads) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No threads found.")
			}
			if result.Pagination != nil && result.Pagination.NextPage != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Next page: --page %d\n", *result.Pagination.NextPage)
			}
		}
		return nil
	}
	return command
}
