package cmd

import (
	"fmt"
	"strings"

	"github.com/Knovigator/knovigator/treectl/api"
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
	Run:    runNewReply,
	Hidden: true,
}

var replyThreadID string
var replyAttachment string
var replySpaceID string
var replyMessageType string

type replyCreateOptions struct {
	ReplyToQuestID string
	Content        string
	DeltaJSON      string
	Attachment     string
	SpaceID        string
	MessageType    string
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
	newReplyCmd.Flags().StringVarP(&createOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	newReplyCmd.Flags().BoolVar(&createJSONOutput, "json", false, "Output JSON instead of human-readable text")
	_ = newReplyCmd.MarkFlagRequired("reply-to")
	_ = newReplyCmd.Flags().MarkHidden("thread")
}

func runNewReply(cmd *cobra.Command, args []string) {
	content := args[0]
	if content == "" {
		fmt.Println("Error: Content is required for a reply.")
		return
	}

	resolvedOutputFormat := resolveOutputFormat(createOutputFormat, createJSONOutput)

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	replyToQuestID, err := normalizeReplyTarget(replyThreadID)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	result, err := createReply(
		profile,
		replyCreateOptions{
			ReplyToQuestID: replyToQuestID,
			Content:        content,
			Attachment:     replyAttachment,
			SpaceID:        replySpaceID,
			MessageType:    replyMessageType,
		},
	)
	if err != nil {
		fmt.Println("Error creating reply:", err)
		return
	}

	printCreateAnswerResult(profile, result, resolvedOutputFormat)
}

func createReply(profile profileConfig, options replyCreateOptions) (api.CreateAnswerResponse, error) {
	spaceID, err := resolveSpaceID(profile, options.SpaceID)
	if err != nil {
		return api.CreateAnswerResponse{}, err
	}

	answerID, err := newUUID()
	if err != nil {
		return api.CreateAnswerResponse{}, fmt.Errorf("error generating answer id: %w", err)
	}

	childQuestID, err := newUUID()
	if err != nil {
		return api.CreateAnswerResponse{}, fmt.Errorf("error generating child quest id: %w", err)
	}

	uploads, err := prepareAttachmentUploads(options.Attachment, "images[]", "recording", "files[]")
	if err != nil {
		return api.CreateAnswerResponse{}, err
	}

	deltaJSON, err := textToDeltaJSONString(options.Content)
	if err != nil {
		return api.CreateAnswerResponse{}, err
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
			AnswerID:     answerID,
			ChildQuestID: childQuestID,
			QuestID:      options.ReplyToQuestID,
			SpaceID:      spaceID,
			UserID:       profile.CurrentUserID,
			Content:      options.Content,
			DeltaJSON:    deltaJSON,
			MessageType:  options.MessageType,
			Uploads:      uploads,
		},
	)
	if err != nil {
		return api.CreateAnswerResponse{}, err
	}

	return result, nil
}
