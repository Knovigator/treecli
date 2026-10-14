package cmd

import (
	"fmt"
	"github.com/Knovigator/treecli/api"

	"github.com/spf13/cobra"
)

var newClipCmd = &cobra.Command{
	Use:   "clip <url>",
	Short: "Create a new clip",
	Long:  `Create a new clip from a URL or with an attachment.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runNewClip,
}

var clipContent string
var clipAttachment string
var clipStream string
var clipWriteID string

func init() {
	newClipCmd.Flags().StringVarP(&clipContent, "content", "c", "", "Additional content for the clip")
	newClipCmd.Flags().StringVarP(&clipAttachment, "attachment", "f", "", "Path to the file to attach")
	newClipCmd.Flags().StringVar(&clipStream, "stream", "", "Target stream name or UUID. Defaults to clips.")
	newClipCmd.Flags().StringVar(&clipWriteID, "id", "", "UUID for this clip post; reuse it to retry safely")
	newClipCmd.Flags().StringVarP(&createOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	newClipCmd.Flags().BoolVar(&createJSONOutput, "json", false, "Output JSON instead of human-readable text")
}

func runNewClip(cmd *cobra.Command, args []string) error {
	url := ""
	if len(args) > 0 {
		url = args[0]
	}
	format := resolveOutputFormat(createOutputFormat, createJSONOutput)
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}
	arguments := map[string]interface{}{"id": clipWriteID, "url": url, "content": clipContent}
	optional(arguments, "stream", clipStream)
	var out api.CreateQuestResponse
	if err := sharedNativeWrite(profile, "clip", arguments, clipAttachment, 0, &out); err != nil {
		return writeErrorForOutput(fmt.Errorf("creating clip: %w", err), format)
	}
	return printCreateQuestResult(profile, out, format)
}
