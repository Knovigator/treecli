package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var newPostCmd = &cobra.Command{
	Use:   "post <content>",
	Short: "Create a new post",
	Long:  `Create a new post with content and optional URL or attachment. Pass --reply-to <quest-id-or-link> to create a reply in an existing thread instead of a new root thread.`,
	Example: "  treecli new post \"hello world\"\n" +
		"  treecli new post --stream public \"hello world\"\n" +
		"  treecli new post --reply-to 7a5e85c9-9dca-4140-ba9a-f5db0030afca \"hello back\"\n" +
		"  treecli new post --reply-to http://localhost:5173/quest/7a5e85c9-9dca-4140-ba9a-f5db0030afca \"hello back\"",
	Args: cobra.MinimumNArgs(1),
	RunE: runNewPost,
}

var ActionCmd = &cobra.Command{
	Use:   "action <ai-action-or-invocation> [prompt...]",
	Short: "Create an AI action post or reply and wait for generated media",
	Long:  "Create an AI action root thread or reply, then poll the submitted answer until media generation completes, fails, or times out.\n\nRun `treecli action actions` to see the current model-backed AI actions.\n\nUse --duration to request a specific audio or video length in seconds; the backend clamps it to the model's allowed range.",
	Example: "  treecli action flux \"a red kite over Bangkok\"\n" +
		"  treecli action !kling \"camera orbit around a bonsai tree\"\n" +
		"  treecli action \"!veo3 slow dolly through a neon alley\"\n" +
		"  treecli action eleven_tts \"read this in a crisp narration voice\"\n" +
		"  treecli action sfx \"rain, tires on wet asphalt, distant thunder\"\n" +
		"  treecli action !stableaudio \"ambient build, 120 BPM\" --duration 90\n" +
		"  treecli action --reply-to 7a5e85c9-9dca-4140-ba9a-f5db0030afca flux \"make this warmer\"\n" +
		"  treecli action --reply-to http://localhost:5173/quest/7a5e85c9-9dca-4140-ba9a-f5db0030afca flux \"make this warmer\"",
	Args:              cobra.MinimumNArgs(1),
	RunE:              runAction,
	ValidArgsFunction: completeActionArgs,
}

var actionActionsCmd = &cobra.Command{
	Use:   "actions",
	Short: "List model-backed AI actions",
	Long:  "List the current model-backed AI actions from the authenticated backend profile.",
	RunE:  runActionActions,
}

var actionTagsCompatCmd = &cobra.Command{
	Use:    "tags",
	Hidden: true,
	RunE:   runActionActions,
}

var actionStatusCmd = &cobra.Command{
	Use:   "status [quest-id-or-link]",
	Short: "Check an existing action result at the thread or post level",
	Long:  "Check an existing action result at the thread/quest level by default, or inspect a specific post with --post or --answer. Use --watch to keep polling until it completes, fails, or times out.",
	Example: "  treecli action status ec587036-f0f8-423a-8ffb-12658f7ac3ce\n" +
		"  treecli action status http://localhost:5173/quest/ec587036-f0f8-423a-8ffb-12658f7ac3ce\n" +
		"  treecli action status --post aeaacf68-d1a7-4f78-8fb0-a39c66ca1cc7\n" +
		"  treecli action status --answer aeaacf68-d1a7-4f78-8fb0-a39c66ca1cc7 --watch",
	Args: cobra.MaximumNArgs(1),
	RunE: runActionStatus,
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
var postWriteID string
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
var actionWriteID string
var actionAllowUnknownTag bool
var actionPaymentMode string
var actionOutputFormat string
var actionJSONOutput bool
var actionPollInterval time.Duration
var actionTimeout time.Duration
var actionNoWait bool
var actionDuration int
var actionStatusAnswerID string
var actionStatusThreadID string
var actionStatusWatch bool
var actionStatusOutputFormat string
var actionStatusJSONOutput bool
var actionStatusPollInterval time.Duration
var actionStatusTimeout time.Duration

type rootThreadCreateOptions struct {
	WriteID            string
	Content            string
	DeltaJSON          string
	ActionRequestsJSON string
	URL                string
	Attachment         string
	Stream             string
	SpaceID            string
	ThreadType         string
	MessageType        string
	TeamID             string
	Public             *bool
	Private            *bool
}

type actionResult struct {
	Status        string   `json:"status"`
	WriteID       string   `json:"write_id,omitempty"`
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
	WriteID  string
	Answer   api.Answer
}

var actionMaxPriceCents int

func init() {
	ActionCmd.Flags().IntVar(&actionMaxPriceCents, "max-price-cents", 0, "Maximum aggregate generation fee in USD cents; required explicit authorization")
	newPostCmd.Flags().StringVarP(&postUrl, "url", "u", "", "Optional URL for the post")
	newPostCmd.Flags().StringVarP(&postAttachment, "attachment", "f", "", "Path to the file to attach")
	newPostCmd.Flags().StringVar(&postStream, "stream", "", "Target stream name or UUID. Defaults to private.")
	newPostCmd.Flags().StringVar(&postReplyTo, "reply-to", "", "Reply to the thread/quest with this id or link instead of creating a new root thread")
	newPostCmd.Flags().StringVar(&postReplyTo, "thread", "", "Post in this thread without quoting a post (thread UUID or link)")
	newPostCmd.Flags().StringVar(&postSpaceID, "space-id", "", "Space ID to create the post in")
	newPostCmd.Flags().StringVar(&postThreadType, "thread-type", "", "Optional thread_type for the new thread")
	newPostCmd.Flags().StringVar(&postMessageType, "message-type", "", "Optional message_type for the submitted answer")
	newPostCmd.Flags().StringVar(&postTeamID, "team-id", "", "Optional stream/team ID to post into")
	newPostCmd.Flags().BoolVar(&postPublic, "public", false, "Mark the new thread as public")
	newPostCmd.Flags().BoolVar(&postPrivate, "private", false, "Mark the new thread as private")
	newPostCmd.Flags().StringVar(&postWriteID, "id", "", "UUID for this post or reply; reuse it to retry safely")
	newPostCmd.Flags().StringVarP(&createOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	newPostCmd.Flags().BoolVar(&createJSONOutput, "json", false, "Output JSON instead of human-readable text")

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
	ActionCmd.Flags().StringVar(&actionWriteID, "id", "", "UUID for this action post or reply; reuse it to retry safely")
	ActionCmd.Flags().BoolVar(&actionAllowUnknownTag, "allow-unknown-action", false, "Submit the action even if it is not present in the current model-backed AI action list")
	ActionCmd.Flags().BoolVar(&actionAllowUnknownTag, "allow-unknown-tag", false, "Compatibility alias for --allow-unknown-action")
	ActionCmd.Flags().StringVar(&actionPaymentMode, "payment", "", "Payment rail for this action: usd or bsv/bitcoinsv (default: account setting)")
	ActionCmd.Flags().StringVarP(&actionOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	ActionCmd.Flags().BoolVar(&actionJSONOutput, "json", false, "Output JSON instead of human-readable text")
	ActionCmd.Flags().DurationVar(&actionPollInterval, "poll-interval", 3*time.Second, "Polling interval while waiting for generated media")
	ActionCmd.Flags().DurationVar(&actionTimeout, "timeout", 10*time.Minute, "Maximum time to wait for generated media")
	ActionCmd.Flags().BoolVar(&actionNoWait, "no-wait", false, "Submit the action and return immediately without polling")
	ActionCmd.Flags().IntVar(&actionDuration, "duration", 0, "Duration in seconds for audio/video generation; the model clamps to its allowed range (0 = model default)")
	_ = ActionCmd.Flags().MarkHidden("thread")
	_ = ActionCmd.Flags().MarkHidden("allow-unknown-tag")
	actionStatusCmd.Flags().StringVar(&actionStatusAnswerID, "answer", "", "Check the action result for this post/answer id or link")
	actionStatusCmd.Flags().StringVar(&actionStatusAnswerID, "post", "", "Check the action result for this post/answer id or link")
	actionStatusCmd.Flags().StringVar(&actionStatusThreadID, "thread", "", "Compatibility alias for the default quest/thread target")
	actionStatusCmd.Flags().BoolVar(&actionStatusWatch, "watch", false, "Keep polling until the action completes, fails, or times out")
	actionStatusCmd.Flags().StringVarP(&actionStatusOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	actionStatusCmd.Flags().BoolVar(&actionStatusJSONOutput, "json", false, "Output JSON instead of human-readable text")
	actionStatusCmd.Flags().DurationVar(&actionStatusPollInterval, "poll-interval", 3*time.Second, "Polling interval while waiting in watch mode")
	actionStatusCmd.Flags().DurationVar(&actionStatusTimeout, "timeout", 10*time.Minute, "Maximum time to wait in watch mode")
	_ = actionStatusCmd.Flags().MarkHidden("thread")
	ActionCmd.AddCommand(actionActionsCmd)
	ActionCmd.AddCommand(actionTagsCompatCmd)
	ActionCmd.AddCommand(actionStatusCmd)
}

func runNewPost(cmd *cobra.Command, args []string) error {
	content := args[0]
	if content == "" {
		return fmt.Errorf("content is required for a post")
	}

	resolvedOutputFormat := resolveOutputFormat(createOutputFormat, createJSONOutput)

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	if strings.TrimSpace(postReplyTo) != "" {
		replyToQuestID := strings.TrimSpace(postReplyTo)

		err = rejectRootOnlyPostFlags(cmd, postReplyTo)
		if err != nil {
			return err
		}

		result, replyErr := createReply(
			profile,
			replyCreateOptions{
				WriteID:        postWriteID,
				ReplyToQuestID: replyToQuestID,
				Content:        content,
				Attachment:     postAttachment,
				SpaceID:        postSpaceID,
				MessageType:    postMessageType,
			},
		)
		if replyErr != nil {
			return writeErrorForOutput(
				fmt.Errorf("creating post reply: %w", replyErr),
				resolvedOutputFormat,
			)
		}

		return printCreateAnswerResult(profile, result, resolvedOutputFormat)
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
			WriteID:     postWriteID,
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
		return writeErrorForOutput(fmt.Errorf("creating post: %w", err), resolvedOutputFormat)
	}

	return printCreateQuestResult(profile, result, resolvedOutputFormat)
}

func runAction(cmd *cobra.Command, args []string) error {
	if (cmd == nil || !cmd.Flags().Changed("max-price-cents")) || actionMaxPriceCents < 0 {
		return fmt.Errorf("--max-price-cents is required and must be nonnegative")
	}
	invocation, err := parseActionInvocation(args)
	if err != nil {
		return err
	}

	if actionPollInterval <= 0 {
		return fmt.Errorf("--poll-interval must be greater than zero")
	}
	if actionTimeout <= 0 {
		return fmt.Errorf("--timeout must be greater than zero")
	}
	if actionDuration < 0 {
		return fmt.Errorf("--duration must be zero or greater")
	}
	paymentMode, err := normalizePaymentMode(actionPaymentMode)
	if err != nil {
		return err
	}
	resolvedOutputFormat := resolveOutputFormat(actionOutputFormat, actionJSONOutput)

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
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
		resolvedOutputFormat,
		paymentMode,
	)
	if err != nil {
		return writeErrorForOutput(err, resolvedOutputFormat)
	}

	return printActionResult(actionResult, resolvedOutputFormat)
}

func runActionStatus(cmd *cobra.Command, args []string) error {
	threadTarget := ""
	if len(args) > 0 {
		threadTarget = args[0]
	}
	if strings.TrimSpace(threadTarget) == "" {
		threadTarget = strings.TrimSpace(actionStatusThreadID)
	}

	if strings.TrimSpace(actionStatusAnswerID) == "" && strings.TrimSpace(threadTarget) == "" {
		return fmt.Errorf("pass a quest id/link, or use --post/--answer <post-id-or-link>")
	}
	if strings.TrimSpace(actionStatusAnswerID) != "" && strings.TrimSpace(threadTarget) != "" {
		return fmt.Errorf("pass either a quest target or --post/--answer, but not both")
	}
	if actionStatusPollInterval <= 0 {
		return fmt.Errorf("--poll-interval must be greater than zero")
	}
	if actionStatusTimeout <= 0 {
		return fmt.Errorf("--timeout must be greater than zero")
	}
	resolvedOutputFormat := resolveOutputFormat(actionStatusOutputFormat, actionStatusJSONOutput)

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	result, err := fetchActionStatus(profile, threadTarget, actionStatusAnswerID)
	if err != nil {
		return err
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
			return err
		}
	}

	return printActionResult(result, resolvedOutputFormat)
}

func createRootThread(profile profileConfig, options rootThreadCreateOptions) (api.CreateQuestResponse, error) {
	args := map[string]interface{}{"id": options.WriteID, "content": options.Content}
	optional(args, "space_id", options.SpaceID)
	optional(args, "stream", options.Stream)
	optional(args, "team_id", options.TeamID)
	optional(args, "thread_type", options.ThreadType)
	optional(args, "message_type", options.MessageType)
	optional(args, "url", options.URL)
	optional(args, "delta_json", options.DeltaJSON)
	optional(args, "action_requests", options.ActionRequestsJSON)
	optionalBool(args, "public", options.Public)
	optionalBool(args, "private", options.Private)
	var out api.CreateQuestResponse
	err := sharedNativeWrite(profile, "post", args, options.Attachment, 0, &out)
	return out, err
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

func boolPtr(value bool) *bool {
	return &value
}

func printActionResult(result actionResult, outputFormat string) error {
	switch outputFormat {
	case "json":
		prettyJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(string(prettyJSON))
	case "ascii":
		fmt.Printf("Status: %s\n", result.Status)
		if result.WriteID != "" {
			fmt.Printf("Write ID: %s\n", result.WriteID)
		}
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
		return invalidOutputFormatError(outputFormat)
	}

	return nil
}

func runActionActions(cmd *cobra.Command, args []string) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}
	rows, err := fetchActionCatalog(profile, false)
	if err != nil {
		return fmt.Errorf("loading AI actions: %w", err)
	}
	grouped := map[string][]generationActionRow{}
	for _, row := range rows {
		kind := row.Kind
		if kind == "" {
			kind = "other"
		}
		grouped[kind] = append(grouped[kind], row)
	}
	kinds := make([]string, 0, len(grouped))
	for kind := range grouped {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for index, kind := range kinds {
		if index > 0 {
			fmt.Println()
		}
		fmt.Println(strings.ToUpper(kind))
		entries := grouped[kind]
		sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Action) < strings.ToLower(entries[j].Action) })
		for _, row := range entries {
			fmt.Printf("  !%s", row.Action)
			if row.Name != "" {
				fmt.Printf("  %s", row.Name)
			}
			if row.Provider != "" {
				fmt.Printf("  [%s]", row.Provider)
			}
			fmt.Println()
			if len(row.Aliases) > 0 {
				fmt.Printf("    aliases: %s\n", strings.Join(prefixAIActionNames(row.Aliases, true), ", "))
			}
			if row.Description != "" {
				fmt.Printf("    %s\n", row.Description)
			}
		}
	}
	return nil
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
		return actionInvocation{}, fmt.Errorf("an AI action is required")
	}

	combinedArgs := strings.TrimSpace(strings.Join(args, " "))
	if combinedArgs == "" {
		return actionInvocation{}, fmt.Errorf("an AI action is required")
	}

	fields := strings.Fields(combinedArgs)
	if len(fields) == 0 {
		return actionInvocation{}, fmt.Errorf("an AI action is required")
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
		return actionInvocation{}, fmt.Errorf("an AI action is required")
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

func completeActionArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return completeAIActionNames(toComplete)
}

func completeAIActionNames(toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeGenerationActionNames(toComplete, false)
}

func createAndMaybePollAction(
	cmd *cobra.Command,
	profile profileConfig,
	invocation actionInvocation,
	publicValue *bool,
	privateValue *bool,
	outputFormat string,
	paymentMode string,
) (actionResult, error) {
	submission, err := createActionSubmission(cmd, profile, invocation, publicValue, privateValue, paymentMode)
	if err != nil {
		return actionResult{}, err
	}

	if actionNoWait {
		result := actionResultFromAnswer(profile, submission.ThreadID, submission.Answer)
		result.WriteID = submission.WriteID
		return result, nil
	}

	result, err := pollActionResult(
		profile,
		submission.ThreadID,
		submission.Answer.ID,
		outputFormat,
		actionTimeout,
		actionPollInterval,
	)
	if err != nil {
		return actionResult{}, withWriteID(submission.WriteID, err)
	}
	result.WriteID = submission.WriteID
	return result, nil
}

func createActionSubmission(
	cmd *cobra.Command,
	profile profileConfig,
	invocation actionInvocation,
	publicValue *bool,
	privateValue *bool,
	paymentMode string,
) (actionSubmission, error) {
	arguments := map[string]interface{}{"id": actionWriteID, "action": invocation.Tag, "prompt": invocation.Prompt, "allow_unknown_action": actionAllowUnknownTag}
	optional(arguments, "payment_mode", paymentMode)
	optional(arguments, "quest_id", actionReplyTo)
	optional(arguments, "space_id", actionSpaceID)
	optional(arguments, "stream", actionStream)
	optional(arguments, "team_id", actionTeamID)
	optional(arguments, "thread_type", actionThreadType)
	optional(arguments, "message_type", actionMessageType)
	optionalBool(arguments, "public", publicValue)
	optionalBool(arguments, "private", privateValue)
	if actionDuration > 0 {
		arguments["duration_seconds"] = actionDuration
	}
	if actionReplyTo != "" {
		var out api.CreateAnswerResponse
		if err := sharedNativeWrite(profile, "action", arguments, actionAttachment, actionMaxPriceCents, &out); err != nil {
			return actionSubmission{}, err
		}
		return actionSubmission{ThreadID: out.Answer.QuestID, WriteID: out.Answer.ID, Answer: out.Answer}, nil
	}
	var out api.CreateQuestResponse
	if err := sharedNativeWrite(profile, "action", arguments, actionAttachment, actionMaxPriceCents, &out); err != nil {
		return actionSubmission{}, err
	}
	if out.Quest.Parent == nil {
		return actionSubmission{}, fmt.Errorf("backend omitted action root post")
	}
	return actionSubmission{ThreadID: out.Quest.ID, WriteID: out.Quest.ID, Answer: *out.Quest.Parent}, nil
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
