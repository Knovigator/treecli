package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Knovigator/knovigator/treectl/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var getMessagesCmd = &cobra.Command{
	Use:     "messages [message_id1] [message_id2] ...",
	Aliases: []string{"answers"},
	Short:   "Get information about specific messages",
	Long:    `Fetch and display information about one or more messages using their IDs.`,
	Args:    cobra.MinimumNArgs(1),
	Run:     runGetMessages,
}

func init() {
	// no flags needed for this command
}

func runGetMessages(cmd *cobra.Command, args []string) {
	messageIDs := args

	// load credentials from viper config
	accessToken := viper.GetString("access_token")
	client := viper.GetString("client")
	uid := viper.GetString("uid")
	backendURL := viper.GetString("backend_url")

	if accessToken == "" || client == "" || uid == "" || backendURL == "" {
		fmt.Println("Error: Missing credentials. Please login first.")
		return
	}

	messagesInfo, err := api.GetMessages(backendURL, accessToken, client, uid, messageIDs)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// pretty print the messages info
	prettyJSON, err := json.MarshalIndent(messagesInfo, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting JSON: %v\n", err)
		return
	}

	fmt.Println(string(prettyJSON))
}
