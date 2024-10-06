package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Knovigator/knovigator/treectl/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var getThreadCmd = &cobra.Command{
	Use:     "thread [thread_id]",
	Aliases: []string{"quest"},
	Short:   "Get information about a specific thread",
	Long:    `Fetch and display information about a thread using its ID.`,
	Args:    cobra.ExactArgs(1),
	Run:     runGetThread,
}

var noRehydrate bool

func init() {
	getThreadCmd.Flags().BoolVarP(&noRehydrate, "no-rehydrate", "n", false, "Do not rehydrate answers into the thread")
}

func runGetThread(cmd *cobra.Command, args []string) {
	threadID := args[0]

	// load credentials from viper config
	accessToken := viper.GetString("access_token")
	client := viper.GetString("client")
	uid := viper.GetString("uid")
	backendURL := viper.GetString("backend_url")

	if accessToken == "" || client == "" || uid == "" || backendURL == "" {
		fmt.Fprintln(os.Stderr, "Error: Missing credentials. Please login first.")
		return
	}

	threadInfo, err := api.GetThread(backendURL, threadID, accessToken, client, uid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	if !noRehydrate {
		fmt.Fprintln(os.Stderr, "Hydrating answers into the thread...")
		threadInfo, err = hydrateAnswersIntoQuest(threadInfo, backendURL, accessToken, client, uid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error hydrating answers: %v\n", err)
			return
		}
	} else {
		fmt.Fprintln(os.Stderr, "Skipping hydration of answers.")
	}

	// pretty print the thread info
	prettyJSON, err := json.MarshalIndent(threadInfo, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		return
	}

	fmt.Fprintln(os.Stderr, "Would've printed json of len: ", len(prettyJSON))
	fmt.Println(string(prettyJSON))
}

func hydrateAnswersIntoQuest(quest map[string]interface{}, backendURL, accessToken, client, uid string) (map[string]interface{}, error) {
	if quest == nil {
		fmt.Fprintln(os.Stderr, "Error: quest is nil")
		return nil, nil
	}
	fmt.Fprintln(os.Stderr, "Keys in quest object:")
	for key := range quest {
		fmt.Fprintln(os.Stderr, key)
	}

	var allAnswerIds []string

	// updated to access sorted_answer_ids from the quest key
	questData, ok := quest["quest"].(map[string]interface{})
	if ok {
		if sortedAnswerIds, ok := questData["sorted_answer_ids"].([]interface{}); ok {
			for _, id := range sortedAnswerIds {
				if strID, ok := id.(string); ok {
					allAnswerIds = append(allAnswerIds, strID)
				}
			}
		}
		fmt.Fprintln(os.Stderr, "Number of sorted answers:", len(allAnswerIds))

		if parentID, ok := questData["parent_id"].(string); ok && parentID != "" {
			allAnswerIds = append(allAnswerIds, parentID)
		}
	}
	fmt.Fprintln(os.Stderr, "retrieving", len(allAnswerIds), "answers")

	if len(allAnswerIds) == 0 {
		return quest, nil
	}

	fmt.Fprintln(os.Stderr, ">>> Getting answers")
	messagesInfo, err := api.GetMessages(backendURL, accessToken, client, uid, allAnswerIds)
	if err != nil {
		return nil, fmt.Errorf("error fetching answers: %v", err)
	}

	// extract the answers from the messagesInfo
	allAnswers, ok := messagesInfo["answers"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected format for answers in response")
	}

	// create a map for easy lookup
	answerMap := make(map[string]interface{})
	for _, answer := range allAnswers {
		if answerObj, ok := answer.(map[string]interface{}); ok {
			if id, ok := answerObj["id"].(string); ok {
				answerMap[id] = answerObj
			}
		}
	}

	if sortedAnswerIds, ok := questData["sorted_answer_ids"].([]interface{}); ok {
		var sortedAnswers []interface{}
		for _, id := range sortedAnswerIds {
			if strID, ok := id.(string); ok {
				if answer, found := answerMap[strID]; found {
					sortedAnswers = append(sortedAnswers, answer)
				}
			}
		}
		questData["sorted_answers"] = sortedAnswers
	}

	if parentID, ok := questData["parent_id"].(string); ok && parentID != "" {
		if parent, found := answerMap[parentID]; found {
			questData["parent"] = parent
		}
	}

	return quest, nil
}
