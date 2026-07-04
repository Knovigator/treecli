package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var notificationsSpaceID string
var notificationsPage int
var notificationsAll bool
var notificationsClear bool
var notificationsOutputFormat string
var notificationsJSONOutput bool

var getNotificationsCmd = &cobra.Command{
	Use:     "notifications",
	Aliases: []string{"notis", "unseen", "unseen-notifications"},
	Short:   "Get unseen notifications",
	Long:    `Fetch unseen notification threads for the active Treechat profile and resolved space.`,
	Example: `  treecli get notifications
  treecli get notifications --space-id <space-id> --json
  treecli get notifications --all
  treecli get notifications --clear`,
	Args: cobra.NoArgs,
	Run:  runGetNotifications,
}

func init() {
	getNotificationsCmd.Flags().StringVar(&notificationsSpaceID, "space-id", "", "Space ID to read notifications from")
	getNotificationsCmd.Flags().IntVar(&notificationsPage, "page", 1, "Notification page to fetch")
	getNotificationsCmd.Flags().BoolVar(&notificationsAll, "all", false, "Fetch all notifications instead of unseen notifications only")
	getNotificationsCmd.Flags().BoolVar(&notificationsClear, "clear", false, "Mark fetched notifications as seen")
	getNotificationsCmd.Flags().StringVarP(&notificationsOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	getNotificationsCmd.Flags().BoolVar(&notificationsJSONOutput, "json", false, "Output JSON instead of human-readable text")
}

func runGetNotifications(cmd *cobra.Command, args []string) {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	if notificationsPage < 1 {
		fmt.Fprintln(os.Stderr, "Error: --page must be 1 or greater")
		return
	}

	resolvedOutputFormat := resolveOutputFormat(notificationsOutputFormat, notificationsJSONOutput)
	if resolvedOutputFormat != "ascii" && resolvedOutputFormat != "json" {
		fmt.Fprintf(os.Stderr, "Invalid output format: %s. Use 'ascii' or 'json'.\n", notificationsOutputFormat)
		return
	}

	unseenOnly := !notificationsAll
	spaceID := strings.TrimSpace(notificationsSpaceID)
	notifications, err := api.GetNotifications(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		spaceID,
		notificationsPage,
		unseenOnly,
		notificationsClear,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	switch resolvedOutputFormat {
	case "json":
		prettyJSON, err := api.PrettyJSON(notifications.Raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
			return
		}
		fmt.Println(prettyJSON)
	case "ascii":
		count, countErr := api.GetNotificationsCount(
			profile.BackendURL,
			profile.AccessToken,
			profile.Client,
			profile.UID,
			spaceID,
		)
		printNotificationsASCII(profile, notifications, count, countErr, unseenOnly)
	}
}

func printNotificationsASCII(
	profile profileConfig,
	notifications api.NotificationsResponse,
	count api.NotificationsCountResponse,
	countErr error,
	unseenOnly bool,
) {
	label := "Notifications"
	if unseenOnly {
		label = "Unseen notifications"
	}

	if countErr == nil && count.NumNotifications != nil {
		fmt.Printf("%s: %d\n", label, *count.NumNotifications)
	} else {
		fmt.Println(label)
	}

	if len(notifications.Quests) == 0 {
		fmt.Println("No notifications found.")
		return
	}

	for index, quest := range notifications.Quests {
		reason := strings.TrimSpace(quest.NotificationReasonLabel)
		if reason == "" {
			reason = "notification"
		}

		fmt.Printf("\n%d. %s\n", index+1, reason)
		fmt.Printf("   thread: %s\n", notificationQuestLink(profile, quest))

		content := notificationQuestContent(quest)
		if content != "" {
			fmt.Printf("   content: %s\n", content)
		}
	}
}

func notificationQuestLink(profile profileConfig, quest api.Quest) string {
	if strings.TrimSpace(quest.QuestURL) != "" {
		return strings.TrimSpace(quest.QuestURL)
	}

	if strings.TrimSpace(quest.Path) != "" {
		path := strings.TrimSpace(quest.Path)
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			return path
		}

		linkBase := strings.TrimRight(profile.AppHost, "/")
		if linkBase == "" {
			linkBase = strings.TrimRight(profile.BackendURL, "/")
		}
		return linkBase + "/" + strings.TrimLeft(path, "/")
	}

	return threadLink(profile, quest.ID)
}

func notificationQuestContent(quest api.Quest) string {
	candidates := make([]string, 0, 2+len(quest.SortedAnswers))
	if quest.Parent != nil {
		candidates = append(candidates, quest.Parent.DisplayContent, quest.Parent.Content)
	}
	for _, answer := range quest.SortedAnswers {
		candidates = append(candidates, answer.DisplayContent, answer.Content)
	}

	for _, candidate := range candidates {
		content := strings.TrimSpace(candidate)
		if content != "" {
			return content
		}
	}

	return ""
}
