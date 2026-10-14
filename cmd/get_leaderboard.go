package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var leaderboardRange string
var leaderboardStartDate string
var leaderboardEndDate string
var leaderboardLimit int
var leaderboardOutputFormat string
var leaderboardJSONOutput bool

var getLeaderboardCmd = &cobra.Command{
	Use:   "leaderboard",
	Short: "Get leaderboard data",
	Long:  `Fetch leaderboard data from Treechat.`,
}

var getLeaderboardUpvaluesCmd = &cobra.Command{
	Use:     "upvalues",
	Aliases: []string{"upvalued-content", "content-upvalues"},
	Short:   "Get top public content by upvalue amount",
	Long: `Fetch public quest JSON for top content ranked by net upvalue sats.

Date ranges use UTC day boundaries. end_date is exclusive when supplied.`,
	Example: `  treecli get leaderboard upvalues --range last-week --limit 5
  treecli get leaderboard upvalues --start-date 2026-05-25 --end-date 2026-06-01 --limit 5 --json
  treecli get leaderboard upvalues --range today -o ascii`,
	Args: cobra.NoArgs,
	RunE: runGetLeaderboardUpvalues,
}

func init() {
	getLeaderboardCmd.AddCommand(getLeaderboardUpvaluesCmd)

	getLeaderboardUpvaluesCmd.Flags().StringVar(&leaderboardRange, "range", "today", "Time range: today, 1d, 7d, 1w, 1mo, 1y, all, last-week")
	getLeaderboardUpvaluesCmd.Flags().StringVar(&leaderboardStartDate, "start-date", "", "Start date in YYYY-MM-DD or YYYYMMDD format, UTC midnight")
	getLeaderboardUpvaluesCmd.Flags().StringVar(&leaderboardEndDate, "end-date", "", "Exclusive end date in YYYY-MM-DD or YYYYMMDD format, UTC midnight")
	getLeaderboardUpvaluesCmd.Flags().IntVar(&leaderboardLimit, "limit", 5, "Maximum results to return, capped by the backend at 20")
	getLeaderboardUpvaluesCmd.Flags().StringVarP(&leaderboardOutputFormat, "output", "o", "json", "Output format: json or ascii")
	getLeaderboardUpvaluesCmd.Flags().BoolVar(&leaderboardJSONOutput, "json", false, "Output JSON instead of human-readable text")
}

func runGetLeaderboardUpvalues(cmd *cobra.Command, args []string) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	raw, err := api.SharedOperation(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, "treechat_read", map[string]interface{}{
		"operation": "leaderboard", "arguments": map[string]interface{}{"range": leaderboardRange, "start_date": leaderboardStartDate, "end_date": leaderboardEndDate, "limit": leaderboardLimit},
	})
	if err != nil {
		return err
	}
	var leaderboard api.UpvaluedContentLeaderboardResponse
	err = json.Unmarshal(raw, &leaderboard)
	leaderboard.Raw = raw
	if err != nil {
		return err
	}

	switch resolveOutputFormat(leaderboardOutputFormat, leaderboardJSONOutput) {
	case "json":
		prettyJSON, err := api.PrettyJSON(leaderboard.Raw)
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(prettyJSON)
	case "ascii":
		printUpvaluedContentLeaderboardASCII(leaderboard)
	default:
		return invalidOutputFormatError(leaderboardOutputFormat)
	}

	return nil
}

func printUpvaluedContentLeaderboardASCII(leaderboard api.UpvaluedContentLeaderboardResponse) {
	startDate := strings.TrimSpace(leaderboard.Period.StartDate)
	endDate := strings.TrimSpace(leaderboard.Period.EndDate)
	if startDate == "" && endDate == "" {
		fmt.Println("Window: all time")
	} else if endDate == "" {
		fmt.Printf("Window: %s through now\n", startDate)
	} else {
		fmt.Printf("Window: %s through %s exclusive\n", startDate, endDate)
	}

	for index, item := range leaderboard.Items {
		content := strings.TrimSpace(item.Answer.DisplayContent)
		if content == "" {
			content = strings.TrimSpace(item.Answer.Content)
		}
		if content == "" {
			content = "(empty)"
		}

		fmt.Printf("%d. %d sats, %d upvalues\n", index+1, item.TotalSats, item.UpvalueCount)
		fmt.Printf("   user: %s (%s)\n", item.User.Name, item.User.ID)
		fmt.Printf("   quest: %s\n", item.QuestURL)
		fmt.Printf("   answer_id: %s\n", item.AnswerID)
		fmt.Printf("   content: %s\n", content)
	}
}
