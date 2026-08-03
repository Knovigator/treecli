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
		return fmt.Errorf("content is required for a reply")
	}

	resolvedOutputFormat := resolveOutputFormat(createOutputFormat, createJSONOutput)

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	replyToQuestID, err := normalizeReplyTarget(replyThreadID)
	if err != nil {
		return err
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
	answerID, err := resolveWriteID(options.WriteID)
	if err != nil {
		return api.CreateAnswerResponse{}, err
	}

	spaceID, err := resolveSpaceID(profile, options.SpaceID)
	if err != nil {
		return api.CreateAnswerResponse{}, withWriteID(answerID, err)
	}

	uploads, err := prepareAttachmentUploads(options.Attachment, "images[]", "recording", "files[]")
	if err != nil {
		return api.CreateAnswerResponse{}, withWriteID(answerID, err)
	}

	deltaJSON, err := textToDeltaJSONString(options.Content)
	if err != nil {
		return api.CreateAnswerResponse{}, withWriteID(answerID, err)
	}
	if strings.TrimSpace(options.DeltaJSON) != "" {
		deltaJSON = options.DeltaJSON
	}

	result, err := api.CreateAnswer(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		api.CreateAnswerRequest{
			AnswerID:           answerID,
			QuestID:            options.ReplyToQuestID,
			SpaceID:            spaceID,
			UserID:             profile.CurrentUserID,
			Content:            options.Content,
			DeltaJSON:          deltaJSON,
			ActionRequestsJSON: options.ActionRequestsJSON,
			MessageType:        options.MessageType,
			Uploads:            uploads,
		},
	)
	if err != nil {
		if shouldReconcileWrite(err) {
			if reconciled, ok := reconcileAnswerWrite(
				profile,
				answerID,
				options.ReplyToQuestID,
				spaceID,
				options.Content,
				deltaJSON,
			); ok {
				return reconciled, nil
			}
		}
		return api.CreateAnswerResponse{}, withWriteID(answerID, err)
	}

	return result, nil
}
