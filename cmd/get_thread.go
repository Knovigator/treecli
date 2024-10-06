package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Knovigator/knovigator/treectl/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var getThreadCmd = &cobra.Command{
	Use:     "thread [thread_id]",
	Aliases: []string{"quest"},
	Short:   "Get information about a specific thread",
	Long:    `Fetch and display information about a thread using its ID.`,
	Args:    cobra.ExactArgs(1),
	Run:     runGetThread,
}

func init() {
	// no flags needed for this command
}

func runGetThread(cmd *cobra.Command, args []string) {
	threadID := args[0]

	// load credentials from viper config
	accessToken := viper.GetString("access_token")
	client := viper.GetString("client")
	uid := viper.GetString("uid")
	backendURL := viper.GetString("backend_url")

	if accessToken == "" || client == "" || uid == "" || backendURL == "" {
		fmt.Println("Error: Missing credentials. Please login first.")
		return
	}

	threadInfo, err := api.GetThread(backendURL, threadID, accessToken, client, uid)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// pretty print the thread info
	prettyJSON, err := json.MarshalIndent(threadInfo, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting JSON: %v\n", err)
		return
	}

	fmt.Println(string(prettyJSON))
}
