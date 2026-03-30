package cmd

import (
	"encoding/json"
	"fmt"
	"time"

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

var ActionCmd = &cobra.Command{
	Use:   "action <content>",
	Short: "Create a root action-tag thread and wait for generated media",
	Long:  `Create a root thread whose content contains an action tag, then poll the root answer until media generation completes, fails, or times out.`,
	Args:  cobra.MinimumNArgs(1),
	Run:   runAction,
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
var actionAttachment string
var actionSpaceID string
var actionThreadType string
var actionMessageType string
var actionTeamID string
var actionPublic bool
var actionPrivate bool
var actionOutputFormat string
var actionPollInterval time.Duration
var actionTimeout time.Duration

type rootThreadCreateOptions struct {
	Content     string
	Attachment  string
	SpaceID     string
	ThreadType  string
	MessageType string
	TeamID      string
	Public      *bool
	Private     *bool
}

type actionResult struct {
	Status        string   `json:"status"`
	ThreadID      string   `json:"thread_id"`
	AnswerID      string   `json:"answer_id"`
	ThreadURL     string   `json:"thread_url"`
	MediaURLs     []string `json:"media_urls"`
	FailureReason string   `json:"failure_reason,omitempty"`
}

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

	ActionCmd.Flags().StringVarP(&actionAttachment, "attachment", "f", "", "Path to the file to attach")
	ActionCmd.Flags().StringVar(&actionSpaceID, "space-id", "", "Space ID to create the action thread in")
	ActionCmd.Flags().StringVar(&actionThreadType, "thread-type", "", "Optional thread_type for the new thread")
	ActionCmd.Flags().StringVar(&actionMessageType, "message-type", "", "Optional message_type for the root answer")
	ActionCmd.Flags().StringVar(&actionTeamID, "team-id", "", "Optional stream/team ID to post into")
	ActionCmd.Flags().BoolVar(&actionPublic, "public", false, "Mark the new thread as public")
	ActionCmd.Flags().BoolVar(&actionPrivate, "private", false, "Mark the new thread as private")
	ActionCmd.Flags().StringVarP(&actionOutputFormat, "output", "o", "json", "Output format: ascii or json")
	ActionCmd.Flags().DurationVar(&actionPollInterval, "poll-interval", 3*time.Second, "Polling interval while waiting for generated media")
	ActionCmd.Flags().DurationVar(&actionTimeout, "timeout", 10*time.Minute, "Maximum time to wait for generated media")
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

	var publicValue *bool
	if cmd.Flags().Changed("public") {
		publicValue = &postPublic
	}

	var privateValue *bool
	if cmd.Flags().Changed("private") {
		privateValue = &postPrivate
	}

	result, err := createRootThread(
		profile,
		rootThreadCreateOptions{
			Content:     content,
			Attachment:  postAttachment,
			SpaceID:     postSpaceID,
			ThreadType:  postThreadType,
			MessageType: postMessageType,
			TeamID:      postTeamID,
			Public:      publicValue,
			Private:     privateValue,
		},
	)
	if err != nil {
		fmt.Println("Error creating post:", err)
		return
	}

	printCreateQuestResult(profile, result, createOutputFormat)
}

func runAction(cmd *cobra.Command, args []string) {
	content := args[0]
	if content == "" {
		fmt.Println("Error: Content is required for an action.")
		return
	}

	if actionPublic && actionPrivate {
		fmt.Println("Error: --public and --private cannot be used together.")
		return
	}
	if actionPollInterval <= 0 {
		fmt.Println("Error: --poll-interval must be greater than zero.")
		return
	}
	if actionTimeout <= 0 {
		fmt.Println("Error: --timeout must be greater than zero.")
		return
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	var publicValue *bool
	if cmd.Flags().Changed("public") {
		publicValue = &actionPublic
	}

	var privateValue *bool
	if cmd.Flags().Changed("private") {
		privateValue = &actionPrivate
	}

	createResult, err := createRootThread(
		profile,
		rootThreadCreateOptions{
			Content:     content,
			Attachment:  actionAttachment,
			SpaceID:     actionSpaceID,
			ThreadType:  actionThreadType,
			MessageType: actionMessageType,
			TeamID:      actionTeamID,
			Public:      publicValue,
			Private:     privateValue,
		},
	)
	if err != nil {
		fmt.Println("Error creating action thread:", err)
		return
	}

	if createResult.Quest.Parent == nil || createResult.Quest.Parent.ID == "" {
		fmt.Println("Error: action thread was created without a root answer id")
		return
	}

	if actionOutputFormat == "ascii" {
		fmt.Printf("Created thread %s. Polling answer %s for generated media...\n", createResult.Quest.ID, createResult.Quest.Parent.ID)
	}

	answerResult, timedOut, err := waitForGeneratedMedia(
		profile,
		createResult.Quest.Parent.ID,
		actionTimeout,
		actionPollInterval,
	)
	if err != nil {
		fmt.Println("Error polling action result:", err)
		return
	}

	result := actionResult{
		Status:        answerResult.Answer.GenerationStatus(),
		ThreadID:      createResult.Quest.ID,
		AnswerID:      answerResult.Answer.ID,
		ThreadURL:     threadLink(profile, createResult.Quest.ID),
		MediaURLs:     answerResult.Answer.CanonicalMediaURLs(profile.BackendURL),
		FailureReason: answerResult.Answer.GenerationFailureReason(),
	}

	if timedOut {
		result.Status = "pending"
	}

	printActionResult(result, actionOutputFormat)
}

func createRootThread(profile profileConfig, options rootThreadCreateOptions) (api.CreateQuestResponse, error) {
	spaceID, err := resolveSpaceID(profile, options.SpaceID)
	if err != nil {
		return api.CreateQuestResponse{}, err
	}

	questID, err := newUUID()
	if err != nil {
		return api.CreateQuestResponse{}, fmt.Errorf("error generating thread id: %w", err)
	}

	parentAnswerID, err := newUUID()
	if err != nil {
		return api.CreateQuestResponse{}, fmt.Errorf("error generating answer id: %w", err)
	}

	uploads, err := prepareAttachmentUploads(
		options.Attachment,
		"parent_attributes[answer_image]",
		"parent_attributes[recording]",
		"parent_attributes[files][]",
	)
	if err != nil {
		return api.CreateQuestResponse{}, err
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
			Content:        options.Content,
			MessageType:    options.MessageType,
			ThreadType:     options.ThreadType,
			TeamID:         options.TeamID,
			Public:         options.Public,
			Private:        options.Private,
			Uploads:        uploads,
		},
	)
	if err != nil {
		return api.CreateQuestResponse{}, err
	}

	return result, nil
}

func waitForGeneratedMedia(
	profile profileConfig,
	answerID string,
	timeout time.Duration,
	pollInterval time.Duration,
) (api.AnswerResponse, bool, error) {
	deadline := time.Now().Add(timeout)

	for {
		answerResult, err := api.GetAnswer(
			profile.BackendURL,
			answerID,
			profile.AccessToken,
			profile.Client,
			profile.UID,
		)
		if err != nil {
			return api.AnswerResponse{}, false, err
		}

		if answerResult.Answer.GenerationStatus() != "pending" {
			return answerResult, false, nil
		}

		if time.Now().After(deadline) {
			return answerResult, true, nil
		}

		time.Sleep(pollInterval)
	}
}

func printActionResult(result actionResult, outputFormat string) {
	switch outputFormat {
	case "json":
		prettyJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Printf("Error formatting JSON: %v\n", err)
			return
		}
		fmt.Println(string(prettyJSON))
	case "ascii":
		fmt.Printf("Status: %s\n", result.Status)
		fmt.Printf("Thread: %s\n", result.ThreadID)
		fmt.Printf("Answer: %s\n", result.AnswerID)
		fmt.Printf("Link: %s\n", result.ThreadURL)
		if result.FailureReason != "" {
			fmt.Printf("Failure: %s\n", result.FailureReason)
		}
		for _, mediaURL := range result.MediaURLs {
			fmt.Printf("Media: %s\n", mediaURL)
		}
	default:
		fmt.Printf("Invalid output format: %s. Use 'ascii' or 'json'.\n", outputFormat)
	}
}
