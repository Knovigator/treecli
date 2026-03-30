package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Knovigator/knovigator/treectl/api"
	"github.com/spf13/cobra"
)

var newPostCmd = &cobra.Command{
	Use:   "post <content>",
	Short: "Create a new post",
	Long:  `Create a new post with content and optional URL or attachment. Pass --reply-to <quest-id> to create a reply in an existing thread instead of a new root thread.`,
	Example: "  treectl new post \"hello world\"\n" +
		"  treectl new post --stream public \"hello world\"\n" +
		"  treectl new post --reply-to 7a5e85c9-9dca-4140-ba9a-f5db0030afca \"hello back\"",
	Args: cobra.MinimumNArgs(1),
	Run:  runNewPost,
}

var ActionCmd = &cobra.Command{
	Use:   "action <tag-or-invocation> [prompt...]",
	Short: "Create an action-tag post or reply and wait for generated media",
	Long:  "Create an action-tag root thread or reply, then poll the submitted answer until media generation completes, fails, or times out.\n\nRun `treectl action tags` to see the current model-backed action tags.",
	Example: "  treectl action flux \"a red kite over Bangkok\"\n" +
		"  treectl action !kling \"camera orbit around a bonsai tree\"\n" +
		"  treectl action \"!veo3 slow dolly through a neon alley\"\n" +
		"  treectl action --reply-to 7a5e85c9-9dca-4140-ba9a-f5db0030afca flux \"make this warmer\"",
	Args: cobra.MinimumNArgs(1),
	Run:  runAction,
}

var actionTagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "List model-backed action tags",
	Long:  "List the current model-backed action tags from the authenticated backend profile.",
	Run:   runActionTags,
}

var postUrl string
var postAttachment string
var postStream string
var postReplyTo string
var postSpaceID string
var postThreadType string
var postMessageType string
var postTeamID string
var postPublic bool
var postPrivate bool
var createOutputFormat string
var actionAttachment string
var actionStream string
var actionReplyTo string
var actionSpaceID string
var actionThreadType string
var actionMessageType string
var actionTeamID string
var actionPublic bool
var actionPrivate bool
var actionAllowUnknownTag bool
var actionOutputFormat string
var actionPollInterval time.Duration
var actionTimeout time.Duration

type rootThreadCreateOptions struct {
	Content     string
	URL         string
	Attachment  string
	Stream      string
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

type actionInvocation struct {
	Tag               string
	Prompt            string
	NormalizedContent string
}

func init() {
	newPostCmd.Flags().StringVarP(&postUrl, "url", "u", "", "Optional URL for the post")
	newPostCmd.Flags().StringVarP(&postAttachment, "attachment", "f", "", "Path to the file to attach")
	newPostCmd.Flags().StringVar(&postStream, "stream", "", "Target stream name or UUID. Defaults to private.")
	newPostCmd.Flags().StringVar(&postReplyTo, "reply-to", "", "Reply to the thread/quest with this id instead of creating a new root thread")
	newPostCmd.Flags().StringVar(&postReplyTo, "thread", "", "Compatibility alias for --reply-to")
	newPostCmd.Flags().StringVar(&postSpaceID, "space-id", "", "Space ID to create the post in")
	newPostCmd.Flags().StringVar(&postThreadType, "thread-type", "", "Optional thread_type for the new thread")
	newPostCmd.Flags().StringVar(&postMessageType, "message-type", "", "Optional message_type for the submitted answer")
	newPostCmd.Flags().StringVar(&postTeamID, "team-id", "", "Optional stream/team ID to post into")
	newPostCmd.Flags().BoolVar(&postPublic, "public", false, "Mark the new thread as public")
	newPostCmd.Flags().BoolVar(&postPrivate, "private", false, "Mark the new thread as private")
	newPostCmd.Flags().StringVarP(&createOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	_ = newPostCmd.Flags().MarkHidden("thread")

	ActionCmd.Flags().StringVarP(&actionAttachment, "attachment", "f", "", "Path to the file to attach")
	ActionCmd.Flags().StringVar(&actionStream, "stream", "", "Target stream name or UUID. Defaults to private.")
	ActionCmd.Flags().StringVar(&actionReplyTo, "reply-to", "", "Reply to the thread/quest with this id instead of creating a new root thread")
	ActionCmd.Flags().StringVar(&actionReplyTo, "thread", "", "Compatibility alias for --reply-to")
	ActionCmd.Flags().StringVar(&actionSpaceID, "space-id", "", "Space ID to create the action in")
	ActionCmd.Flags().StringVar(&actionThreadType, "thread-type", "", "Optional thread_type for the new thread")
	ActionCmd.Flags().StringVar(&actionMessageType, "message-type", "", "Optional message_type for the submitted answer")
	ActionCmd.Flags().StringVar(&actionTeamID, "team-id", "", "Optional stream/team ID to post into")
	ActionCmd.Flags().BoolVar(&actionPublic, "public", false, "Mark the new thread as public")
	ActionCmd.Flags().BoolVar(&actionPrivate, "private", false, "Mark the new thread as private")
	ActionCmd.Flags().BoolVar(&actionAllowUnknownTag, "allow-unknown-tag", false, "Submit the action even if the tag is not present in the current model-backed tag list")
	ActionCmd.Flags().StringVarP(&actionOutputFormat, "output", "o", "json", "Output format: ascii or json")
	ActionCmd.Flags().DurationVar(&actionPollInterval, "poll-interval", 3*time.Second, "Polling interval while waiting for generated media")
	ActionCmd.Flags().DurationVar(&actionTimeout, "timeout", 10*time.Minute, "Maximum time to wait for generated media")
	_ = ActionCmd.Flags().MarkHidden("thread")
	ActionCmd.AddCommand(actionTagsCmd)
}

func runNewPost(cmd *cobra.Command, args []string) {
	content := args[0]
	if content == "" {
		fmt.Println("Error: Content is required for a post.")
		return
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if strings.TrimSpace(postReplyTo) != "" {
		err = rejectRootOnlyPostFlags(cmd, postReplyTo)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		result, replyErr := createReply(
			profile,
			replyCreateOptions{
				ReplyToQuestID: strings.TrimSpace(postReplyTo),
				Content:        content,
				Attachment:     postAttachment,
				SpaceID:        postSpaceID,
				MessageType:    postMessageType,
			},
		)
		if replyErr != nil {
			fmt.Println("Error creating post reply:", replyErr)
			return
		}

		printCreateAnswerResult(profile, result, createOutputFormat)
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
			URL:         postUrl,
			Attachment:  postAttachment,
			Stream:      postStream,
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
	invocation, err := parseActionInvocation(args)
	if err != nil {
		fmt.Println("Error:", err)
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

	validTags, err := fetchKnownActionTags(profile)
	if err != nil {
		fmt.Println("Error loading action tags:", err)
		return
	}

	if !validTags[strings.ToLower(invocation.Tag)] && !actionAllowUnknownTag {
		fmt.Printf("Error: unknown action tag %q\n", invocation.Tag)
		fmt.Println("Run `treectl action tags` to inspect the current model-backed tags, or pass --allow-unknown-tag to submit anyway.")
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

	actionResult, err := createAndPollAction(
		cmd,
		profile,
		invocation,
		publicValue,
		privateValue,
	)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	printActionResult(actionResult, actionOutputFormat)
}

func createRootThread(profile profileConfig, options rootThreadCreateOptions) (api.CreateQuestResponse, error) {
	spaceID, err := resolveSpaceID(profile, options.SpaceID)
	if err != nil {
		return api.CreateQuestResponse{}, err
	}

	resolvedTarget, err := resolveRootThreadTarget(profile, options)
	if err != nil {
		return api.CreateQuestResponse{}, err
	}

	if options.URL != "" || resolvedTarget.Kind == "clips" {
		return createClipQuest(profile, options.URL, options.Content, options.Attachment, resolvedTarget)
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

	deltaJSON, err := textToDeltaJSONString(options.Content)
	if err != nil {
		return api.CreateQuestResponse{}, err
	}

	var publicValue *bool
	var privateValue *bool
	teamID := ""

	switch resolvedTarget.Kind {
	case "public":
		publicValue = boolPtr(true)
	case "private":
		privateValue = boolPtr(true)
	case "team":
		teamID = resolvedTarget.ID
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
			DeltaJSON:      deltaJSON,
			MessageType:    options.MessageType,
			ThreadType:     options.ThreadType,
			TeamID:         teamID,
			Public:         publicValue,
			Private:        privateValue,
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

func resolveRootThreadTarget(profile profileConfig, options rootThreadCreateOptions) (streamTarget, error) {
	if strings.TrimSpace(options.Stream) != "" {
		return resolveStreamTarget(profile, options.Stream, defaultPrivateStreamTarget())
	}

	if strings.TrimSpace(options.TeamID) != "" {
		return resolveStreamTarget(profile, options.TeamID, defaultPrivateStreamTarget())
	}

	if options.Public != nil && *options.Public && options.Private != nil && *options.Private {
		return streamTarget{}, fmt.Errorf("conflicting stream flags")
	}

	if options.Public != nil && *options.Public {
		return resolveStreamTarget(profile, "public", defaultPrivateStreamTarget())
	}

	if options.Private != nil && *options.Private {
		return resolveStreamTarget(profile, "private", defaultPrivateStreamTarget())
	}

	return defaultPrivateStreamTarget(), nil
}

func boolPtr(value bool) *bool {
	return &value
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

func runActionTags(cmd *cobra.Command, args []string) {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	models, err := api.ListAIModels(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID)
	if err != nil {
		fmt.Println("Error loading action tags:", err)
		return
	}

	groupedModels := map[string][]api.AIModelRef{}
	for _, model := range models {
		tagName := strings.TrimSpace(model.ActionTagName)
		if tagName == "" {
			continue
		}

		groupKey := strings.TrimSpace(model.ModelType)
		if groupKey == "" {
			groupKey = "other"
		}
		groupedModels[groupKey] = append(groupedModels[groupKey], model)
	}

	groupNames := make([]string, 0, len(groupedModels))
	for groupName := range groupedModels {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)

	for groupIndex, groupName := range groupNames {
		if groupIndex > 0 {
			fmt.Println()
		}
		fmt.Printf("%s\n", strings.ToUpper(groupName))

		modelsInGroup := groupedModels[groupName]
		sort.Slice(modelsInGroup, func(left int, right int) bool {
			return strings.ToLower(modelsInGroup[left].ActionTagName) < strings.ToLower(modelsInGroup[right].ActionTagName)
		})

		for _, model := range modelsInGroup {
			displayName := strings.TrimSpace(model.HumanName)
			if displayName == "" {
				displayName = strings.TrimSpace(model.Name)
			}
			description := strings.TrimSpace(model.DescriptionShort)
			if description == "" {
				description = strings.TrimSpace(model.Description)
			}

			fmt.Printf("  !%s", model.ActionTagName)
			if displayName != "" {
				fmt.Printf("  %s", displayName)
			}
			if strings.TrimSpace(model.Provider) != "" {
				fmt.Printf("  [%s]", model.Provider)
			}
			fmt.Println()
			if description != "" {
				fmt.Printf("    %s\n", description)
			}
		}
	}
}

func rejectRootOnlyPostFlags(cmd *cobra.Command, replyTo string) error {
	if strings.TrimSpace(replyTo) == "" {
		return nil
	}

	replyRestrictedFlags := []string{"stream", "team-id", "public", "private", "thread-type", "url"}
	for _, flagName := range replyRestrictedFlags {
		if cmd.Flags().Changed(flagName) {
			return fmt.Errorf("--%s cannot be used with --reply-to because replies inherit the existing thread placement", flagName)
		}
	}

	return nil
}

func parseActionInvocation(args []string) (actionInvocation, error) {
	if len(args) == 0 {
		return actionInvocation{}, fmt.Errorf("an action tag is required")
	}

	combinedArgs := strings.TrimSpace(strings.Join(args, " "))
	if combinedArgs == "" {
		return actionInvocation{}, fmt.Errorf("an action tag is required")
	}

	fields := strings.Fields(combinedArgs)
	if len(fields) == 0 {
		return actionInvocation{}, fmt.Errorf("an action tag is required")
	}

	rawTag := strings.TrimSpace(fields[0])
	if strings.HasPrefix(rawTag, "!") {
		rawTag = strings.TrimPrefix(rawTag, "!")
	} else if len(args) > 1 {
		rawTag = strings.TrimSpace(args[0])
		fields = append([]string{rawTag}, args[1:]...)
	}

	normalizedTag := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(rawTag, "!")))
	if normalizedTag == "" {
		return actionInvocation{}, fmt.Errorf("an action tag is required")
	}

	promptFields := fields[1:]
	normalizedContent := "!" + normalizedTag
	if len(promptFields) > 0 {
		prompt := strings.TrimSpace(strings.Join(promptFields, " "))
		if prompt != "" {
			normalizedContent = normalizedContent + " " + prompt
		}
	}

	return actionInvocation{
		Tag:               normalizedTag,
		Prompt:            strings.TrimSpace(strings.TrimPrefix(normalizedContent, "!"+normalizedTag)),
		NormalizedContent: normalizedContent,
	}, nil
}

func fetchKnownActionTags(profile profileConfig) (map[string]bool, error) {
	models, err := api.ListAIModels(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID)
	if err != nil {
		return nil, err
	}

	knownTags := map[string]bool{}
	for _, model := range models {
		tagName := strings.ToLower(strings.TrimSpace(model.ActionTagName))
		if tagName == "" {
			continue
		}
		knownTags[tagName] = true
	}

	return knownTags, nil
}

func createAndPollAction(
	cmd *cobra.Command,
	profile profileConfig,
	invocation actionInvocation,
	publicValue *bool,
	privateValue *bool,
) (actionResult, error) {
	if strings.TrimSpace(actionReplyTo) != "" {
		err := rejectRootOnlyPostFlags(cmd, actionReplyTo)
		if err != nil {
			return actionResult{}, err
		}

		replyResult, err := createReply(
			profile,
			replyCreateOptions{
				ReplyToQuestID: strings.TrimSpace(actionReplyTo),
				Content:        invocation.NormalizedContent,
				Attachment:     actionAttachment,
				SpaceID:        actionSpaceID,
				MessageType:    actionMessageType,
			},
		)
		if err != nil {
			return actionResult{}, fmt.Errorf("creating action reply: %w", err)
		}

		threadID := replyResult.Answer.QuestID
		if threadID == "" && replyResult.Quest != nil {
			threadID = replyResult.Quest.ID
		}
		return pollActionResult(profile, threadID, replyResult.Answer.ID)
	}

	createResult, err := createRootThread(
		profile,
		rootThreadCreateOptions{
			Content:     invocation.NormalizedContent,
			Stream:      actionStream,
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
		return actionResult{}, fmt.Errorf("creating action thread: %w", err)
	}

	if createResult.Quest.Parent == nil || createResult.Quest.Parent.ID == "" {
		return actionResult{}, fmt.Errorf("action thread was created without a root answer id")
	}

	return pollActionResult(profile, createResult.Quest.ID, createResult.Quest.Parent.ID)
}

func pollActionResult(profile profileConfig, threadID string, answerID string) (actionResult, error) {
	if actionOutputFormat == "ascii" {
		fmt.Printf("Created action in thread %s. Polling answer %s for generated media...\n", threadID, answerID)
	}

	answerResult, timedOut, err := waitForGeneratedMedia(
		profile,
		answerID,
		actionTimeout,
		actionPollInterval,
	)
	if err != nil {
		return actionResult{}, fmt.Errorf("polling action result: %w", err)
	}

	result := actionResult{
		Status:        answerResult.Answer.GenerationStatus(),
		ThreadID:      threadID,
		AnswerID:      answerResult.Answer.ID,
		ThreadURL:     threadLink(profile, threadID),
		MediaURLs:     answerResult.Answer.CanonicalMediaURLs(profile.BackendURL),
		FailureReason: answerResult.Answer.GenerationFailureReason(),
	}

	if timedOut {
		result.Status = "pending"
	}

	return result, nil
}
