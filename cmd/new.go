package cmd

import (
	"fmt"
	"strings"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var NewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create new resources",
	Long:  `Create new resources such as posts or clips.`,
}

var newReplyCmd = &cobra.Command{
	Use:    "reply <content>",
	Short:  "Create a reply in an existing thread",
	Long:   `Create a reply answer in an existing thread.`,
	Args:   cobra.MinimumNArgs(1),
	RunE:   runNewReply,
	Hidden: true,
}

var replyThreadID string
var replyAttachment string
var replySpaceID string
var replyMessageType string
var replyWriteID string

type replyCreateOptions struct {
	ReplyToAnswerID    string
	WriteID            string
	ReplyToQuestID     string
	Content            string
	DeltaJSON          string
	ActionRequestsJSON string
	Attachment         string
	SpaceID            string
	MessageType        string
}

func init() {
	NewCmd.AddCommand(newPostCmd)
	NewCmd.AddCommand(newReplyCmd)
	NewCmd.AddCommand(newClipCmd)

	newReplyCmd.Flags().StringVar(&replyThreadID, "reply-to", "", "Reply to the thread/quest with this id or link instead of creating a new root thread")
	newReplyCmd.Flags().StringVar(&replyThreadID, "thread", "", "Compatibility alias for --reply-to")
	newReplyCmd.Flags().StringVarP(&replyAttachment, "attachment", "f", "", "Path to the file to attach")
	newReplyCmd.Flags().StringVar(&replySpaceID, "space-id", "", "Space ID to create the reply in")
	newReplyCmd.Flags().StringVar(&replyMessageType, "message-type", "", "Optional message_type for the reply")
	newReplyCmd.Flags().StringVar(&replyWriteID, "id", "", "UUID for this reply; reuse it to retry safely")
	newReplyCmd.Flags().StringVarP(&createOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	newReplyCmd.Flags().BoolVar(&createJSONOutput, "json", false, "Output JSON instead of human-readable text")
	_ = newReplyCmd.MarkFlagRequired("reply-to")
	_ = newReplyCmd.Flags().MarkHidden("thread")
}

func runNewReply(cmd *cobra.Command, args []string) error {
	content := args[0]
	if content == "" {
		return usageErrorf("content is required for a reply")
	}

	resolvedOutputFormat := resolveOutputFormat(createOutputFormat, createJSONOutput)

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	replyToQuestID := strings.TrimSpace(replyThreadID)
	if replyToQuestID == "" {
		return fmt.Errorf("missing --reply-to value")
	}

	result, err := createReply(
		profile,
		replyCreateOptions{
			WriteID:        replyWriteID,
			ReplyToQuestID: replyToQuestID,
			Content:        content,
			Attachment:     replyAttachment,
			SpaceID:        replySpaceID,
			MessageType:    replyMessageType,
		},
	)
	if err != nil {
		return writeErrorForOutput(fmt.Errorf("creating reply: %w", err), resolvedOutputFormat)
	}

	return printCreateAnswerResult(profile, result, resolvedOutputFormat)
}

func createReply(profile profileConfig, options replyCreateOptions) (api.CreateAnswerResponse, error) {
	args := map[string]interface{}{"id": options.WriteID, "content": options.Content, "quest_id": options.ReplyToQuestID}
	optional(args, "space_id", options.SpaceID)
	optional(args, "message_type", options.MessageType)
	optional(args, "reply_to_answer_id", options.ReplyToAnswerID)
	optional(args, "delta_json", options.DeltaJSON)
	optional(args, "action_requests", options.ActionRequestsJSON)
	var out api.CreateAnswerResponse
	err := sharedNativeWrite(profile, "reply", args, options.Attachment, 0, &out)
	return out, err
}
