package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var newPostCmd = &cobra.Command{
	Use:   "post <content>",
	Short: "Create a new post",
	Long:  `Create a new post with content and optional URL or attachment.`,
	Args:  cobra.MinimumNArgs(1),
	Run:   runNewPost,
}

var postUrl string
var postAttachment string

func init() {
	newPostCmd.Flags().StringVarP(&postUrl, "url", "u", "", "Optional URL for the post")
	newPostCmd.Flags().StringVarP(&postAttachment, "attachment", "f", "", "Path to the file to attach")
}

func runNewPost(cmd *cobra.Command, args []string) {
	content := args[0]
	if content == "" {
		fmt.Println("Error: Content is required for a post.")
		return
	}

	clipLink(postUrl, content, postAttachment, false)
}
