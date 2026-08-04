package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var upvaluesPage int
var upvaluesPerPage int
var upvaluesOutputFormat string
var upvaluesJSONOutput bool

var getUpvaluesCmd = &cobra.Command{
	Use:     "upvalues",
	Aliases: []string{"upvalue-history", "bsv-history"},
	Short:   "Get authenticated upvalue history",
	Long:    `Fetch a page of upvalues sent or received by the authenticated Treechat user.`,
	Example: `  treecli get upvalues
  treecli get upvalues --page 2 --per-page 25
  treecli get upvalues --json`,
	Args: cobra.NoArgs,
	RunE: runGetUpvalues,
}

func init() {
	getUpvaluesCmd.Flags().IntVar(&upvaluesPage, "page", 1, "Upvalue history page to fetch")
	getUpvaluesCmd.Flags().IntVar(&upvaluesPerPage, "per-page", 50, "Upvalues per page, capped by the backend at 100")
	getUpvaluesCmd.Flags().StringVarP(&upvaluesOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	getUpvaluesCmd.Flags().BoolVar(&upvaluesJSONOutput, "json", false, "Output JSON instead of human-readable text")
}

func runGetUpvalues(cmd *cobra.Command, args []string) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}
	if upvaluesPage < 1 {
		return fmt.Errorf("--page must be 1 or greater")
	}
	if upvaluesPerPage < 1 || upvaluesPerPage > 100 {
		return fmt.Errorf("--per-page must be between 1 and 100")
	}

	history, err := api.GetUpvalueHistory(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		upvaluesPage,
		upvaluesPerPage,
	)
	if err != nil {
		return err
	}

	switch resolveOutputFormat(upvaluesOutputFormat, upvaluesJSONOutput) {
	case "json":
		prettyJSON, err := api.PrettyJSON(history.Raw)
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(prettyJSON)
	case "ascii":
		printUpvalueHistoryASCII(profile, history)
	default:
		return invalidOutputFormatError(upvaluesOutputFormat)
	}

	return nil
}

func printUpvalueHistoryASCII(profile profileConfig, history api.UpvalueHistoryResponse) {
	fmt.Printf("Upvalue history: page %d (%d per page)\n", history.Page, history.PerPage)

	if len(history.Upvalues) == 0 {
		fmt.Println("No upvalues found.")
		return
	}

	for index, upvalue := range history.Upvalues {
		fmt.Printf("\n%d. %s %s sats\n", index+1, upvalueDirection(upvalue, profile.CurrentUserID), formatSats(upvalue.Amount))
		if strings.TrimSpace(upvalue.Fee) != "" {
			fmt.Printf("   fee: %s sats\n", formatSats(upvalue.Fee))
		}
		fmt.Printf("   created: %s\n", upvalue.CreatedAt)
		fmt.Printf("   from: %s\n", formatUpvalueUser(upvalue.FromUser, upvalue.FromUserID))
		fmt.Printf("   to: %s\n", formatUpvalueUser(upvalue.ToUser, upvalue.ToUserID))
		if upvalue.Answer.ID != "" {
			fmt.Printf("   answer: %s\n", upvalue.Answer.ID)
		}
		if len(upvalue.Answer.ChildQuests) > 0 && upvalue.Answer.ChildQuests[0].ID != "" {
			fmt.Printf("   thread: %s\n", threadLink(profile, upvalue.Answer.ChildQuests[0].ID))
		}
		if upvalue.Status != "" {
			fmt.Printf("   status: %s\n", upvalue.Status)
		}
		if upvalue.TxID != "" {
			fmt.Printf("   tx: %s\n", upvalue.TxID)
		}
	}

	if history.HasMore {
		fmt.Printf("\nMore results are available on page %d.\n", history.Page+1)
	}
}

func upvalueDirection(upvalue api.BsvUpvalue, currentUserID string) string {
	if upvalue.IsBoost && upvalue.FromUserID == currentUserID {
		return "boosted"
	}
	if currentUserID != "" && upvalue.ToUserID == currentUserID {
		return "received"
	}
	if currentUserID != "" && upvalue.FromUserID == currentUserID {
		return "sent"
	}
	if upvalue.IsBoost {
		return "boost"
	}
	return "upvalue"
}

func formatUpvalueUser(user api.User, fallbackID string) string {
	name := strings.TrimSpace(user.Name)
	id := strings.TrimSpace(user.ID)
	if id == "" {
		id = strings.TrimSpace(fallbackID)
	}
	if name == "" {
		return id
	}
	if id == "" {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, id)
}

func formatSats(rawAmount string) string {
	amount := strings.TrimSpace(rawAmount)
	if amount == "" {
		return "0"
	}

	parts := strings.SplitN(amount, ".", 2)
	whole := parts[0]
	if len(parts) == 2 && strings.Trim(parts[1], "0") != "" {
		return amount
	}

	sign := ""
	if strings.HasPrefix(whole, "-") || strings.HasPrefix(whole, "+") {
		sign = whole[:1]
		whole = whole[1:]
	}
	if _, err := strconv.ParseUint(whole, 10, 64); err != nil {
		return amount
	}

	for index := len(whole) - 3; index > 0; index -= 3 {
		whole = whole[:index] + "," + whole[index:]
	}
	return sign + whole
}
