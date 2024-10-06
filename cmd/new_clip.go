package cmd

import (
	"github.com/spf13/cobra"
)

var newClipCmd = &cobra.Command{
	Use:   "clip <url>",
	Short: "Create a new clip",
	Long:  `Create a new clip from a URL or with an attachment.`,
	Args:  cobra.MaximumNArgs(1),
	Run:   runNewClip,
}

var clipContent string
var clipAttachment string

func init() {
	newClipCmd.Flags().StringVarP(&clipContent, "content", "c", "", "Additional content for the clip")
	newClipCmd.Flags().StringVarP(&clipAttachment, "attachment", "f", "", "Path to the file to attach")
}

func runNewClip(cmd *cobra.Command, args []string) {
	var url string
	if len(args) > 0 {
		url = args[0]
	}

	clipLink(url, clipContent, clipAttachment, true)
}
