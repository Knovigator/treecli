package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Knovigator/knovigator/treectl/api"
	"github.com/spf13/cobra"
)

var newPostCmd = &cobra.Command{
	Use:   "post <content>",
	Short: "Create a new post",
	Long:  `Create a new post with content and optional URL or attachment. Pass --reply-to <quest-id-or-link> to create a reply in an existing thread instead of a new root thread.`,
	Example: "  treectl new post \"hello world\"\n" +
		"  treectl new post --stream public \"hello world\"\n" +
		"  treectl new post --reply-to 7a5e85c9-9dca-4140-ba9a-f5db0030afca \"hello back\"\n" +
		"  treectl new post --reply-to http://localhost:5173/quest/7a5e85c9-9dca-4140-ba9a-f5db0030afca \"hello back\"",
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
		"  treectl action --reply-to 7a5e85c9-9dca-4140-ba9a-f5db0030afca flux \"make this warmer\"\n" +
		"  treectl action --reply-to http://localhost:5173/quest/7a5e85c9-9dca-4140-ba9a-f5db0030afca flux \"make this warmer\"",
	Args:              cobra.MinimumNArgs(1),
	Run:               runAction,
	ValidArgsFunction: completeActionArgs,
}

var actionTagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "List model-backed action tags",
	Long:  "List the current model-backed action tags from the authenticated backend profile.",
	Run:   runActionTags,
}

var actionStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check an existing action result by answer or thread id",
	Long:  "Check an existing action answer by answer id or by the thread/quest id that owns it. Use --watch to keep polling until it completes, fails, or times out.",
	Example: "  treectl action status --answer aeaacf68-d1a7-4f78-8fb0-a39c66ca1cc7\n" +
		"  treectl action status --thread ec587036-f0f8-423a-8ffb-12658f7ac3ce\n" +
		"  treectl action status --answer aeaacf68-d1a7-4f78-8fb0-a39c66ca1cc7 --watch",
	Args: cobra.NoArgs,
	Run:  runActionStatus,
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
var createJSONOutput bool
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
var actionJSONOutput bool
var actionPollInterval time.Duration
var actionTimeout time.Duration
var actionNoWait bool
var actionStatusAnswerID string
var actionStatusThreadID string
var actionStatusWatch bool
var actionStatusOutputFormat string
var actionStatusJSONOutput bool
var actionStatusPollInterval time.Duration
var actionStatusTimeout time.Duration

type rootThreadCreateOptions struct {
	Content     string
	DeltaJSON   string
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
	ThreadUUID    string   `json:"thread_uuid"`
	PostUUID      string   `json:"post_uuid"`
	ThreadLink    string   `json:"thread_link"`
	MediaLinks    []string `json:"media_links"`
	RawMediaLinks []string `json:"raw_media_links"`
	FailureReason string   `json:"failure_reason,omitempty"`
}

type actionInvocation struct {
	Tag               string
	Prompt            string
	NormalizedContent string
}

type actionSpinner struct {
	done chan struct{}
}

type actionSubmission struct {
	ThreadID string
	Answer   api.Answer
}

func init() {
	newPostCmd.Flags().StringVarP(&postUrl, "url", "u", "", "Optional URL for the post")
	newPostCmd.Flags().StringVarP(&postAttachment, "attachment", "f", "", "Path to the file to attach")
	newPostCmd.Flags().StringVar(&postStream, "stream", "", "Target stream name or UUID. Defaults to private.")
	newPostCmd.Flags().StringVar(&postReplyTo, "reply-to", "", "Reply to the thread/quest with this id or link instead of creating a new root thread")
	newPostCmd.Flags().StringVar(&postReplyTo, "thread", "", "Compatibility alias for --reply-to")
	newPostCmd.Flags().StringVar(&postSpaceID, "space-id", "", "Space ID to create the post in")
	newPostCmd.Flags().StringVar(&postThreadType, "thread-type", "", "Optional thread_type for the new thread")
	newPostCmd.Flags().StringVar(&postMessageType, "message-type", "", "Optional message_type for the submitted answer")
	newPostCmd.Flags().StringVar(&postTeamID, "team-id", "", "Optional stream/team ID to post into")
	newPostCmd.Flags().BoolVar(&postPublic, "public", false, "Mark the new thread as public")
	newPostCmd.Flags().BoolVar(&postPrivate, "private", false, "Mark the new thread as private")
	newPostCmd.Flags().StringVarP(&createOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	newPostCmd.Flags().BoolVar(&createJSONOutput, "json", false, "Output JSON instead of human-readable text")
	_ = newPostCmd.Flags().MarkHidden("thread")

	ActionCmd.Flags().StringVarP(&actionAttachment, "attachment", "f", "", "Path to the file to attach")
	ActionCmd.Flags().StringVar(&actionStream, "stream", "", "Target stream name or UUID. Defaults to private.")
	ActionCmd.Flags().StringVar(&actionReplyTo, "reply-to", "", "Reply to the thread/quest with this id or link instead of creating a new root thread")
	ActionCmd.Flags().StringVar(&actionReplyTo, "thread", "", "Compatibility alias for --reply-to")
	ActionCmd.Flags().StringVar(&actionSpaceID, "space-id", "", "Space ID to create the action in")
	ActionCmd.Flags().StringVar(&actionThreadType, "thread-type", "", "Optional thread_type for the new thread")
	ActionCmd.Flags().StringVar(&actionMessageType, "message-type", "", "Optional message_type for the submitted answer")
	ActionCmd.Flags().StringVar(&actionTeamID, "team-id", "", "Optional stream/team ID to post into")
	ActionCmd.Flags().BoolVar(&actionPublic, "public", false, "Mark the new thread as public")
	ActionCmd.Flags().BoolVar(&actionPrivate, "private", false, "Mark the new thread as private")
	ActionCmd.Flags().BoolVar(&actionAllowUnknownTag, "allow-unknown-tag", false, "Submit the action even if the tag is not present in the current model-backed tag list")
	ActionCmd.Flags().StringVarP(&actionOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	ActionCmd.Flags().BoolVar(&actionJSONOutput, "json", false, "Output JSON instead of human-readable text")
	ActionCmd.Flags().DurationVar(&actionPollInterval, "poll-interval", 3*time.Second, "Polling interval while waiting for generated media")
	ActionCmd.Flags().DurationVar(&actionTimeout, "timeout", 10*time.Minute, "Maximum time to wait for generated media")
	ActionCmd.Flags().BoolVar(&actionNoWait, "no-wait", false, "Submit the action and return immediately without polling")
	_ = ActionCmd.Flags().MarkHidden("thread")
	actionStatusCmd.Flags().StringVar(&actionStatusAnswerID, "answer", "", "Check the action result for this answer id")
	actionStatusCmd.Flags().StringVar(&actionStatusThreadID, "thread", "", "Check the action result for this thread/quest id")
	actionStatusCmd.Flags().BoolVar(&actionStatusWatch, "watch", false, "Keep polling until the action completes, fails, or times out")
	actionStatusCmd.Flags().StringVarP(&actionStatusOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	actionStatusCmd.Flags().BoolVar(&actionStatusJSONOutput, "json", false, "Output JSON instead of human-readable text")
	actionStatusCmd.Flags().DurationVar(&actionStatusPollInterval, "poll-interval", 3*time.Second, "Polling interval while waiting in watch mode")
	actionStatusCmd.Flags().DurationVar(&actionStatusTimeout, "timeout", 10*time.Minute, "Maximum time to wait in watch mode")
	ActionCmd.AddCommand(actionTagsCmd)
	ActionCmd.AddCommand(actionStatusCmd)
}

func runNewPost(cmd *cobra.Command, args []string) {
	content := args[0]
	if content == "" {
		fmt.Println("Error: Content is required for a post.")
		return
	}

	resolvedOutputFormat := resolveOutputFormat(createOutputFormat, createJSONOutput)

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if strings.TrimSpace(postReplyTo) != "" {
		replyToQuestID, err := normalizeReplyTarget(postReplyTo)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		err = rejectRootOnlyPostFlags(cmd, postReplyTo)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		result, replyErr := createReply(
			profile,
			replyCreateOptions{
				ReplyToQuestID: replyToQuestID,
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

		printCreateAnswerResult(profile, result, resolvedOutputFormat)
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

	printCreateQuestResult(profile, result, resolvedOutputFormat)
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
	resolvedOutputFormat := resolveOutputFormat(actionOutputFormat, actionJSONOutput)

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

	actionResult, err := createAndMaybePollAction(
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

	printActionResult(actionResult, resolvedOutputFormat)
}

func runActionStatus(cmd *cobra.Command, args []string) {
	if strings.TrimSpace(actionStatusAnswerID) == "" && strings.TrimSpace(actionStatusThreadID) == "" {
		fmt.Println("Error: pass either --answer <answer-id> or --thread <quest-id>.")
		return
	}
	if strings.TrimSpace(actionStatusAnswerID) != "" && strings.TrimSpace(actionStatusThreadID) != "" {
		fmt.Println("Error: pass only one of --answer or --thread.")
		return
	}
	if actionStatusPollInterval <= 0 {
		fmt.Println("Error: --poll-interval must be greater than zero.")
		return
	}
	if actionStatusTimeout <= 0 {
		fmt.Println("Error: --timeout must be greater than zero.")
		return
	}
	resolvedOutputFormat := resolveOutputFormat(actionStatusOutputFormat, actionStatusJSONOutput)

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	result, err := fetchActionStatus(profile, strings.TrimSpace(actionStatusThreadID), strings.TrimSpace(actionStatusAnswerID))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if actionStatusWatch && result.Status == "pending" {
		result, err = pollActionResult(
			profile,
			result.ThreadUUID,
			result.PostUUID,
			resolvedOutputFormat,
			actionStatusTimeout,
			actionStatusPollInterval,
		)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
	}

	printActionResult(result, resolvedOutputFormat)
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
	if strings.TrimSpace(options.DeltaJSON) != "" {
		deltaJSON = options.DeltaJSON
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
		fmt.Printf("Thread UUID: %s\n", result.ThreadUUID)
		fmt.Printf("Post UUID: %s\n", result.PostUUID)
		fmt.Printf("Thread link: %s\n", result.ThreadLink)
		if result.FailureReason != "" {
			fmt.Printf("Failure: %s\n", result.FailureReason)
		}
		maxLinks := len(result.MediaLinks)
		if len(result.RawMediaLinks) > maxLinks {
			maxLinks = len(result.RawMediaLinks)
		}
		for index := 0; index < maxLinks; index++ {
			if index < len(result.MediaLinks) {
				fmt.Printf("Media link: %s\n", result.MediaLinks[index])
			}
			if index < len(result.RawMediaLinks) {
				fmt.Printf("Raw media link: %s\n", result.RawMediaLinks[index])
			}
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

	models, err := fetchVisibleActionModels(profile)
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
	models, err := fetchVisibleActionModels(profile)
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

func fetchVisibleActionModels(profile profileConfig) ([]api.AIModelRef, error) {
	models, err := api.ListAIModels(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID)
	if err != nil {
		return nil, err
	}

	visibleModels := make([]api.AIModelRef, 0, len(models))
	for _, model := range models {
		if shouldHideActionModel(model) {
			continue
		}
		visibleModels = append(visibleModels, model)
	}

	return visibleModels, nil
}

func shouldHideActionModel(model api.AIModelRef) bool {
	if strings.EqualFold(strings.TrimSpace(model.Provider), "openclaw") {
		return true
	}

	tagName := strings.ToLower(strings.TrimSpace(model.ActionTagName))
	return strings.HasPrefix(tagName, "openclaw")
}

func completeActionArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	models, err := fetchVisibleActionModels(profile)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	normalizedPrefix := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(toComplete, "!")))
	wantBangPrefix := strings.HasPrefix(strings.TrimSpace(toComplete), "!")
	completions := []string{}
	seenTags := map[string]bool{}

	for _, model := range models {
		tagName := strings.TrimSpace(model.ActionTagName)
		if tagName == "" {
			continue
		}

		normalizedTag := strings.ToLower(tagName)
		if normalizedPrefix != "" && !strings.HasPrefix(normalizedTag, normalizedPrefix) {
			continue
		}
		if seenTags[normalizedTag] {
			continue
		}
		seenTags[normalizedTag] = true

		completionTag := tagName
		if wantBangPrefix {
			completionTag = "!" + tagName
		}
		completions = append(completions, completionTag)
	}

	sort.Strings(completions)
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func createAndMaybePollAction(
	cmd *cobra.Command,
	profile profileConfig,
	invocation actionInvocation,
	publicValue *bool,
	privateValue *bool,
) (actionResult, error) {
	submission, err := createActionSubmission(cmd, profile, invocation, publicValue, privateValue)
	if err != nil {
		return actionResult{}, err
	}

	if actionNoWait {
		return actionResultFromAnswer(profile, submission.ThreadID, submission.Answer), nil
	}

	return pollActionResult(
		profile,
		submission.ThreadID,
		submission.Answer.ID,
		actionOutputFormat,
		actionTimeout,
		actionPollInterval,
	)
}

func createActionSubmission(
	cmd *cobra.Command,
	profile profileConfig,
	invocation actionInvocation,
	publicValue *bool,
	privateValue *bool,
) (actionSubmission, error) {
	actionDeltaJSON, err := actionTextToDeltaJSONString(invocation.NormalizedContent)
	if err != nil {
		return actionSubmission{}, fmt.Errorf("building action delta_json: %w", err)
	}

	if strings.TrimSpace(actionReplyTo) != "" {
		replyToQuestID, err := normalizeReplyTarget(actionReplyTo)
		if err != nil {
			return actionSubmission{}, err
		}

		err = rejectRootOnlyPostFlags(cmd, actionReplyTo)
		if err != nil {
			return actionSubmission{}, err
		}

		replyResult, err := createReply(
			profile,
			replyCreateOptions{
				ReplyToQuestID: replyToQuestID,
				Content:        invocation.NormalizedContent,
				DeltaJSON:      actionDeltaJSON,
				Attachment:     actionAttachment,
				SpaceID:        actionSpaceID,
				MessageType:    actionMessageType,
			},
		)
		if err != nil {
			return actionSubmission{}, fmt.Errorf("creating action reply: %w", err)
		}

		threadID := replyResult.Answer.QuestID
		if threadID == "" && replyResult.Quest != nil {
			threadID = replyResult.Quest.ID
		}

		return actionSubmission{
			ThreadID: threadID,
			Answer:   replyResult.Answer,
		}, nil
	}

	createResult, err := createRootThread(
		profile,
		rootThreadCreateOptions{
			Content:     invocation.NormalizedContent,
			DeltaJSON:   actionDeltaJSON,
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
		return actionSubmission{}, fmt.Errorf("creating action thread: %w", err)
	}

	if createResult.Quest.Parent == nil || createResult.Quest.Parent.ID == "" {
		return actionSubmission{}, fmt.Errorf("action thread was created without a root answer id")
	}

	return actionSubmission{
		ThreadID: createResult.Quest.ID,
		Answer:   *createResult.Quest.Parent,
	}, nil
}

func pollActionResult(
	profile profileConfig,
	threadID string,
	answerID string,
	outputFormat string,
	timeout time.Duration,
	pollInterval time.Duration,
) (actionResult, error) {
	spinner := startActionSpinner(answerID)
	if spinner != nil {
		defer spinner.Stop()
	} else if outputFormat == "ascii" {
		fmt.Printf("Polling answer %s in thread %s for generated media...\n", answerID, threadID)
	}

	answerResult, timedOut, err := waitForGeneratedMedia(
		profile,
		answerID,
		timeout,
		pollInterval,
	)
	if err != nil {
		return actionResult{}, fmt.Errorf("polling action result: %w", err)
	}

	result := actionResultFromAnswer(profile, threadID, answerResult.Answer)

	if timedOut {
		result.Status = "pending"
	}

	return result, nil
}

func fetchActionStatus(profile profileConfig, threadID string, answerID string) (actionResult, error) {
	if strings.TrimSpace(answerID) != "" {
		answerResult, err := api.GetAnswer(
			profile.BackendURL,
			answerID,
			profile.AccessToken,
			profile.Client,
			profile.UID,
		)
		if err != nil {
			return actionResult{}, fmt.Errorf("loading answer %s: %w", answerID, err)
		}

		resolvedThreadID := strings.TrimSpace(threadID)
		if resolvedThreadID == "" {
			resolvedThreadID = strings.TrimSpace(answerResult.Answer.QuestID)
		}

		return actionResultFromAnswer(profile, resolvedThreadID, answerResult.Answer), nil
	}

	threadResult, err := api.GetThread(
		profile.BackendURL,
		threadID,
		profile.AccessToken,
		profile.Client,
		profile.UID,
	)
	if err != nil {
		return actionResult{}, fmt.Errorf("loading thread %s: %w", threadID, err)
	}

	answer, err := deriveThreadStatusAnswer(threadResult.Quest)
	if err != nil {
		return actionResult{}, err
	}

	return actionResultFromAnswer(profile, threadResult.Quest.ID, answer), nil
}

func deriveThreadStatusAnswer(quest api.Quest) (api.Answer, error) {
	if quest.Parent != nil && strings.TrimSpace(quest.Parent.ID) != "" {
		return *quest.Parent, nil
	}

	if len(quest.SortedAnswers) > 0 && strings.TrimSpace(quest.SortedAnswers[0].ID) != "" {
		return quest.SortedAnswers[0], nil
	}

	return api.Answer{}, fmt.Errorf("thread %s does not have a readable answer to inspect", quest.ID)
}

func actionResultFromAnswer(profile profileConfig, threadID string, answer api.Answer) actionResult {
	resolvedThreadID := strings.TrimSpace(threadID)
	if resolvedThreadID == "" {
		resolvedThreadID = strings.TrimSpace(answer.QuestID)
	}
	mediaLinkPairs := api.ResolveAnswerMediaLinks(answer, profile.BackendURL)

	return actionResult{
		Status:        answer.GenerationStatus(),
		ThreadUUID:    resolvedThreadID,
		PostUUID:      answer.ID,
		ThreadLink:    threadLink(profile, resolvedThreadID),
		MediaLinks:    mediaLinksFromResolvedPairs(mediaLinkPairs),
		RawMediaLinks: rawMediaLinksFromResolvedPairs(mediaLinkPairs),
		FailureReason: answer.GenerationFailureReason(),
	}
}

func mediaLinksFromResolvedPairs(mediaLinkPairs []api.ResolvedMediaLink) []string {
	mediaLinks := make([]string, 0, len(mediaLinkPairs))
	for _, mediaLinkPair := range mediaLinkPairs {
		if strings.TrimSpace(mediaLinkPair.MediaLink) == "" {
			continue
		}
		mediaLinks = append(mediaLinks, mediaLinkPair.MediaLink)
	}

	return mediaLinks
}

func rawMediaLinksFromResolvedPairs(mediaLinkPairs []api.ResolvedMediaLink) []string {
	rawMediaLinks := make([]string, 0, len(mediaLinkPairs))
	for _, mediaLinkPair := range mediaLinkPairs {
		if strings.TrimSpace(mediaLinkPair.RawMediaLink) == "" {
			continue
		}
		rawMediaLinks = append(rawMediaLinks, mediaLinkPair.RawMediaLink)
	}

	return rawMediaLinks
}

func startActionSpinner(answerID string) *actionSpinner {
	if !stderrIsTTY() {
		return nil
	}

	spinner := &actionSpinner{
		done: make(chan struct{}),
	}

	go spinner.run(answerID)
	return spinner
}

func (spinner *actionSpinner) Stop() {
	if spinner == nil {
		return
	}

	close(spinner.done)
}

func (spinner *actionSpinner) run(answerID string) {
	frames := []string{"|", "/", "-", `\`}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	startedAt := time.Now()
	frameIndex := 0

	for {
		elapsed := time.Since(startedAt).Round(time.Second)
		if elapsed < time.Second {
			elapsed = time.Second
		}

		fmt.Fprintf(
			os.Stderr,
			"\r\033[K%s Waiting for generated media on answer %s (%s elapsed)",
			frames[frameIndex],
			shortActionID(answerID),
			elapsed,
		)

		select {
		case <-spinner.done:
			fmt.Fprint(os.Stderr, "\r\033[K")
			return
		case <-ticker.C:
			frameIndex = (frameIndex + 1) % len(frames)
		}
	}
}

func shortActionID(id string) string {
	trimmedID := strings.TrimSpace(id)
	if len(trimmedID) <= 8 {
		return trimmedID
	}

	return trimmedID[:8]
}

func stderrIsTTY() bool {
	stderrInfo, err := os.Stderr.Stat()
	if err != nil {
		return false
	}

	return (stderrInfo.Mode() & os.ModeCharDevice) != 0
}
