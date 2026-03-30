package cmd

import (
	"fmt"

	"github.com/Knovigator/knovigator/treectl/api"
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
var postSpaceID string
var postThreadType string
var postMessageType string
var postTeamID string
var postPublic bool
var postPrivate bool
var createOutputFormat string

func init() {
	newPostCmd.Flags().StringVarP(&postUrl, "url", "u", "", "Optional URL for the post")
	newPostCmd.Flags().StringVarP(&postAttachment, "attachment", "f", "", "Path to the file to attach")
	newPostCmd.Flags().StringVar(&postSpaceID, "space-id", "", "Space ID to create the post in")
	newPostCmd.Flags().StringVar(&postThreadType, "thread-type", "", "Optional thread_type for the new thread")
	newPostCmd.Flags().StringVar(&postMessageType, "message-type", "", "Optional message_type for the root answer")
	newPostCmd.Flags().StringVar(&postTeamID, "team-id", "", "Optional stream/team ID to post into")
	newPostCmd.Flags().BoolVar(&postPublic, "public", false, "Mark the new thread as public")
	newPostCmd.Flags().BoolVar(&postPrivate, "private", false, "Mark the new thread as private")
	newPostCmd.Flags().StringVarP(&createOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
}

func runNewPost(cmd *cobra.Command, args []string) {
	content := args[0]
	if content == "" {
		fmt.Println("Error: Content is required for a post.")
		return
	}

	if postUrl != "" {
		clipLink(postUrl, content, postAttachment, false)
		return
	}

	if postPublic && postPrivate {
		fmt.Println("Error: --public and --private cannot be used together.")
		return
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	spaceID, err := resolveSpaceID(profile, postSpaceID)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	questID, err := newUUID()
	if err != nil {
		fmt.Println("Error generating thread id:", err)
		return
	}

	parentAnswerID, err := newUUID()
	if err != nil {
		fmt.Println("Error generating answer id:", err)
		return
	}

	uploads, err := prepareAttachmentUploads(
		postAttachment,
		"parent_attributes[answer_image]",
		"parent_attributes[recording]",
		"parent_attributes[files][]",
	)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	var publicValue *bool
	if cmd.Flags().Changed("public") {
		publicValue = &postPublic
	}

	var privateValue *bool
	if cmd.Flags().Changed("private") {
		privateValue = &postPrivate
	}

	result, err := api.CreateQuest(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		api.CreateQuestRequest{
			QuestID:        questID,
			ParentAnswerID: parentAnswerID,
			SpaceID:        spaceID,
			Content:        content,
			MessageType:    postMessageType,
			ThreadType:     postThreadType,
			TeamID:         postTeamID,
			Public:         publicValue,
			Private:        privateValue,
			Uploads:        uploads,
		},
	)
	if err != nil {
		fmt.Println("Error creating post:", err)
		return
	}

	printCreateQuestResult(profile, result, createOutputFormat)
}
