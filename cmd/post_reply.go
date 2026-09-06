package cmd

import (
	"fmt"
	"strings"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var BranchReplyCmd = newPostReplyCommand(true)
var QuoteReplyCmd = newPostReplyCommand(false)

func newPostReplyCommand(branch bool) *cobra.Command {
	name, description := "quote-reply", "Quote a post and reply in its containing thread"
	if branch {
		name, description = "branch-reply", "Reply in a post's child branch, quoting its title by default"
	}
	var noQuote, jsonOutput bool
	var output, writeID, attachment string
	command := &cobra.Command{
		Use: name + " POST_ID_OR_LINK CONTENT", Short: description, Args: cobra.ExactArgs(2),
		Example: "  treecli " + name + " POST_ID \"Reply text\" --json",
		RunE: func(command *cobra.Command, args []string) error {
			target, err := normalizeAnswerTarget(args[0])
			if err != nil {
				return err
			}
			target = strings.ToLower(target)
			if strings.TrimSpace(args[1]) == "" {
				return fmt.Errorf("reply content is required")
			}
			format := resolveOutputFormat(output, jsonOutput)
			if format != "ascii" && format != "json" {
				return invalidOutputFormatError(format)
			}
			id, err := resolveWriteID(writeID)
			if err != nil {
				return err
			}
			profile, err := requireAuthenticatedProfile()
			if err != nil {
				return err
			}
			destination, err := resolvePostReplyDestination(profile, target, branch)
			if err != nil {
				return err
			}
			quote := target
			if noQuote {
				quote = ""
			}
			result, err := createReply(profile, replyCreateOptions{WriteID: id, ReplyToQuestID: destination.ID,
				ReplyToAnswerID: quote, SpaceID: destination.SpaceID, Content: args[1], Attachment: attachment})
			if err != nil {
				return err
			}
			if result.Answer.ID != id || result.Answer.QuestID != destination.ID || result.Answer.ReplyToAnswerID != quote {
				return withWriteID(id, fmt.Errorf("backend did not confirm the requested reply destination and quote; post %s may already exist: inspect it before retrying", id))
			}
			return printCreateAnswerResult(profile, result, format)
		},
	}
	if branch {
		command.Flags().BoolVar(&noQuote, "no-quote", false, "Reply in the branch without quoting its title")
	}
	command.Flags().StringVar(&writeID, "id", "", "UUID for this reply; reuse it to retry safely")
	command.Flags().StringVarP(&attachment, "attachment", "f", "", "Path to the file to attach")
	command.Flags().StringVarP(&output, "output", "o", "ascii", "Output format: ascii or json")
	command.Flags().BoolVar(&jsonOutput, "json", false, "Output JSON instead of human-readable text")
	return command
}

func resolvePostReplyDestination(profile profileConfig, target string, branch bool) (api.Quest, error) {
	post, err := api.GetAnswer(profile.BackendURL, target, profile.AccessToken, profile.Client, profile.UID)
	if err != nil {
		return api.Quest{}, err
	}
	if post.Answer.ID != target {
		return api.Quest{}, fmt.Errorf("backend did not return the requested post")
	}
	if !branch {
		if post.Answer.QuestID == "" {
			return api.Quest{}, fmt.Errorf("post has no containing thread; use branch-reply to reply beneath this root post")
		}
		thread, err := api.GetThread(profile.BackendURL, post.Answer.QuestID, profile.AccessToken, profile.Client, profile.UID)
		if err != nil {
			return api.Quest{}, err
		}
		if thread.Quest.ID != post.Answer.QuestID {
			return api.Quest{}, fmt.Errorf("backend did not return the containing thread")
		}
		return thread.Quest, nil
	}
	// Child references alone do not identify the discussion branch. Validate
	// their parent relationship and prefer the canonical side quest.
	candidates, discussions := []api.Quest{}, []api.Quest{}
	seen := map[string]bool{}
	for _, child := range post.Answer.ChildQuests {
		if child.ID == "" || seen[child.ID] {
			continue
		}
		seen[child.ID] = true
		thread, err := api.GetThread(profile.BackendURL, child.ID, profile.AccessToken, profile.Client, profile.UID)
		if err != nil {
			return api.Quest{}, err
		}
		if thread.Quest.ID != child.ID || thread.Quest.ParentID != target {
			return api.Quest{}, fmt.Errorf("child thread does not belong to the target post")
		}
		candidates = append(candidates, thread.Quest)
		if thread.Quest.SideQuest {
			discussions = append(discussions, thread.Quest)
		}
	}
	if len(discussions) == 1 {
		return discussions[0], nil
	}
	if len(discussions) == 0 && len(candidates) == 1 {
		return candidates[0], nil
	}
	return api.Quest{}, fmt.Errorf("post has no unambiguous accessible reply branch; inspect its child threads before posting")
}
