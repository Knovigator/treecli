package cmd

import (
	"github.com/Knovigator/knovigator/treectl/api"
	"github.com/spf13/cobra"
)

var getMessagesCmd = &cobra.Command{
	Use:     "messages [message_id1] [message_id2] ...",
	Aliases: []string{"answers"},
	Short:   "Get information about specific messages",
	Long:    `Fetch and display information about one or more messages using their IDs.`,
	Args:    cobra.MinimumNArgs(1),
	Run:     api.GetMessages,
}

func init() {
	// no flags needed for this command
}
