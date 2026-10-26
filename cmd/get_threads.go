package cmd

import (
	"fmt"
	"strings"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

func newGetThreadsCommand() *cobra.Command {
	var user, answer, output, order, before string
	var root, branch, public, jsonOutput bool
	var page, limit int
	command := &cobra.Command{
		Use:     "threads [thread_id]",
		Short:   "List authored threads or fetch a thread or an answer's child threads",
		Long:    "List the public feed with --public (newest first by default; continue with --before, which stays stable while new threads arrive; ranked orders use --page), or list your authored threads, newest-created first, or fetch by thread ID or --answer. Listing includes accessible private content and defaults to --user me. Roots have no containing thread; branches grow from a message in another thread. Page numbers can shift when threads are added or removed.",
		Example: "  treecli get threads --user me --root --limit 1\n  treecli get threads --user alice --branch --page 2 --limit 20\n  treecli get threads THREAD_ID --json\n  treecli get threads --answer ANSWER_ID --json",
		Args:    cobra.MaximumNArgs(1),
	}
	flags := command.Flags()
	flags.StringVar(&user, "user", "me", "Author username, user ID, or me (prefix @ to force a username)")
	flags.StringVar(&answer, "answer", "", "Fetch all accessible child threads of this answer ID")
	flags.BoolVar(&root, "root", false, "List only root threads")
	flags.BoolVar(&branch, "branch", false, "List only branch threads")
	flags.BoolVar(&public, "public", false, "List the public feed (any author) instead of one author's threads")
	flags.StringVar(&order, "order", "time", "Public feed order: time (newest first), upvalue, or upvalueTrending (requires --public)")
	flags.StringVar(&before, "before", "", "Public time feed cursor: the Next value from the previous page (requires --public)")
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
			return usageErrorf("--root and --branch are mutually exclusive")
		}
		if flags.Changed("answer") && strings.TrimSpace(answer) == "" {
			return usageErrorf("--answer must not be empty")
		}
		if len(args) == 1 && strings.TrimSpace(args[0]) == "" {
			return usageErrorf("thread ID must not be empty")
		}
		if len(args) == 1 && flags.Changed("answer") {
			return usageErrorf("thread ID and --answer are mutually exclusive")
		}
		if flags.Changed("order") && !public {
			return usageErrorf("--order requires --public")
		}
		if flags.Changed("before") && !public {
			return usageErrorf("--before requires --public")
		}
		if public {
			if order != "time" && order != "upvalue" && order != "upvalueTrending" {
				return usageErrorf("--order must be time, upvalue, or upvalueTrending")
			}
			if len(args) == 1 || flags.Changed("answer") {
				return usageErrorf("--public cannot be combined with a thread ID or --answer")
			}
			for _, name := range []string{"user", "root", "branch"} {
				if flags.Changed(name) {
					return usageErrorf("--%s cannot be combined with --public", name)
				}
			}
			if order == "time" {
				if flags.Changed("page") {
					return usageErrorf("--page does not apply to the time feed; continue with --before")
				}
				if limit < 1 || limit > 100 {
					return usageErrorf("--limit must be between 1 and 100")
				}
			} else {
				if flags.Changed("before") || flags.Changed("limit") {
					return usageErrorf("--before and --limit apply only to --order time; use --page")
				}
				if page < 1 {
					return usageErrorf("--page must be positive")
				}
			}
		}
		lookup := len(args) == 1 || flags.Changed("answer")
		if lookup {
			for _, name := range []string{"user", "root", "branch", "page", "limit"} {
				if flags.Changed(name) {
					return usageErrorf("--%s cannot be combined with a thread ID or --answer", name)
				}
			}
		} else if !public {
			if strings.TrimSpace(user) == "" {
				return usageErrorf("--user must not be empty")
			}
			if page < 1 || limit < 1 || limit > 100 {
				return usageErrorf("--page must be positive and --limit must be between 1 and 100")
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
		if public {
			result, err = api.ListThreads(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, api.ThreadListOptions{Public: true, Order: order, Page: page, Limit: limit, Before: before})
		} else if flags.Changed("answer") {
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
			if result.Pagination != nil && result.Pagination.NextCursor != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Next page: --before %s\n", *result.Pagination.NextCursor)
			} else if result.Pagination != nil && result.Pagination.NextPage != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Next page: --page %d\n", *result.Pagination.NextPage)
			}
		}
		return nil
	}
	return command
}
