package cmd

import (
	"fmt"

	"github.com/Knovigator/knovigator/treectl/api"
	"github.com/spf13/cobra"
)

var NewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create new resources",
	Long:  `Create new resources such as posts or clips.`,
}

var newReplyCmd = &cobra.Command{
	Use:   "reply <content>",
	Short: "Create a reply in an existing thread",
	Long:  `Create a reply answer in an existing thread.`,
	Args:  cobra.MinimumNArgs(1),
	Run:   runNewReply,
}

var replyThreadID string
var replyAttachment string
var replySpaceID string
var replyMessageType string

func init() {
	NewCmd.AddCommand(newPostCmd)
	NewCmd.AddCommand(newReplyCmd)
	NewCmd.AddCommand(newClipCmd)

	newReplyCmd.Flags().StringVar(&replyThreadID, "thread", "", "Thread ID to reply to")
	newReplyCmd.Flags().StringVarP(&replyAttachment, "attachment", "f", "", "Path to the file to attach")
	newReplyCmd.Flags().StringVar(&replySpaceID, "space-id", "", "Space ID to create the reply in")
	newReplyCmd.Flags().StringVar(&replyMessageType, "message-type", "", "Optional message_type for the reply")
	newReplyCmd.Flags().StringVarP(&createOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	_ = newReplyCmd.MarkFlagRequired("thread")
}

func runNewReply(cmd *cobra.Command, args []string) {
	content := args[0]
	if content == "" {
		fmt.Println("Error: Content is required for a reply.")
		return
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	spaceID, err := resolveSpaceID(profile, replySpaceID)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	answerID, err := newUUID()
	if err != nil {
		fmt.Println("Error generating answer id:", err)
		return
	}

	childQuestID, err := newUUID()
	if err != nil {
		fmt.Println("Error generating child quest id:", err)
		return
	}

	uploads, err := prepareAttachmentUploads(replyAttachment, "images[]", "recording", "files[]")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	result, err := api.CreateAnswer(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		api.CreateAnswerRequest{
			AnswerID:     answerID,
			ChildQuestID: childQuestID,
			QuestID:      replyThreadID,
			SpaceID:      spaceID,
			Content:      content,
			MessageType:  replyMessageType,
			Uploads:      uploads,
		},
	)
	if err != nil {
		fmt.Println("Error creating reply:", err)
		return
	}

	printCreateAnswerResult(profile, result, createOutputFormat)
}
