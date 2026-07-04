package cmd

import (
	"fmt"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var getMessagesCmd = &cobra.Command{
	Use:     "messages [message_id1] [message_id2] ...",
	Aliases: []string{"answers"},
	Short:   "Get information about specific messages",
	Long:    `Fetch and display information about one or more messages using their IDs.`,
	Args:    cobra.MinimumNArgs(1),
	RunE:    runGetMessages,
}

var outputFormat string
var getJSONOutput bool

func init() {
	getMessagesCmd.Flags().StringVarP(&outputFormat, "output", "o", "ascii", "Output format: ascii or json")
	getMessagesCmd.Flags().BoolVar(&getJSONOutput, "json", false, "Output JSON instead of human-readable text")
}

func runGetMessages(cmd *cobra.Command, args []string) error {
	messageIDs := args

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	messagesInfo, err := api.GetMessages(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, messageIDs)
	if err != nil {
		return err
	}

	switch resolveOutputFormat(outputFormat, getJSONOutput) {
	case "json":
		prettyJSON, err := api.PrettyJSON(messagesInfo.Raw)
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(prettyJSON)
	case "ascii":
		for _, answer := range messagesInfo.Answers {
			fmt.Println(answer.ToASCII())
		}
	default:
		return invalidOutputFormatError(outputFormat)
	}

	return nil
}
