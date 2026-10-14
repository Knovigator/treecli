package cmd

import (
	"fmt"
	"strings"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var BranchReplyCmd = newPostReplyCommand(true)
var QuoteReplyCmd = newPostReplyCommand(false)

func newPostReplyCommand(branch bool) *cobra.Command {
	name, description := "quote-reply", "Quote a post and reply in its containing thread"
	if branch {
		name, description = "branch-reply", "Reply in a post's child branch, quoting its title by default"
	}
	var noQuote, jsonOutput bool
	var output, writeID, attachment string
	command := &cobra.Command{
		Use: name + " POST_ID_OR_LINK CONTENT", Short: description, Args: cobra.ExactArgs(2),
		Example: "  treecli " + name + " POST_ID \"Reply text\" --json",
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(args[1]) == "" {
				return fmt.Errorf("reply content is required")
			}
			format := resolveOutputFormat(output, jsonOutput)
			if format != "ascii" && format != "json" {
				return invalidOutputFormatError(format)
			}
			profile, err := requireAuthenticatedProfile()
			if err != nil {
				return err
			}
			operation := "quote_reply"
			if branch {
				operation = "branch_reply"
			}
			arguments := map[string]interface{}{"id": writeID, "source_post_id": args[0], "content": args[1], "no_quote": noQuote}
			var result api.CreateAnswerResponse
			if err := sharedNativeWrite(profile, operation, arguments, attachment, 0, &result); err != nil {
				return err
			}

			return printCreateAnswerResult(profile, result, format)
		},
	}
	if branch {
		command.Flags().BoolVar(&noQuote, "no-quote", false, "Reply in the branch without quoting its title")
	}
	command.Flags().StringVar(&writeID, "id", "", "UUID for this reply; reuse it to retry safely")
	command.Flags().StringVarP(&attachment, "attachment", "f", "", "Path to the file to attach")
	command.Flags().StringVarP(&output, "output", "o", "ascii", "Output format: ascii or json")
	command.Flags().BoolVar(&jsonOutput, "json", false, "Output JSON instead of human-readable text")
	return command
}
