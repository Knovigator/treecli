package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var billingBudgetJSON bool

var billingBudgetCmd = &cobra.Command{
	Use: "budget", Short: "Show the shared daily generation cap and UTC reset",
	Long: "Inspect the account's daily generation budget in USD cents. An unset cap is explicitly shown as unset. The backend owns reservation accounting and resets the admission day at midnight UTC.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBillingBudget(cmd, "get_generation_budget", map[string]interface{}{})
	},
}

var billingBudgetSetCmd = &cobra.Command{
	Use: "set <daily-limit-cents>", Short: "Set the shared daily generation cap in USD cents", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := strconv.ParseInt(args[0], 10, 32)
		if err != nil || value < 0 {
			return usageErrorf("daily limit must be an integer between 0 and 2147483647 USD cents")
		}
		return runBillingBudget(cmd, "set_generation_budget", map[string]interface{}{"daily_limit_cents": value})
	},
}

var billingBudgetUnsetCmd = &cobra.Command{
	Use: "unset", Short: "Remove the daily generation cap; per-action caps still apply", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBillingBudget(cmd, "set_generation_budget", map[string]interface{}{"daily_limit_cents": nil})
	},
}

func init() {
	billingBudgetCmd.PersistentFlags().BoolVar(&billingBudgetJSON, "json", false, "Output the backend budget and accounting details as JSON")
	billingBudgetCmd.AddCommand(billingBudgetSetCmd, billingBudgetUnsetCmd)
	BillingCmd.AddCommand(billingBudgetCmd)
}

func runBillingBudget(cmd *cobra.Command, operation string, arguments map[string]interface{}) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}
	raw, err := api.SharedOperation(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, operation, arguments)
	if err != nil {
		return err
	}
	if billingBudgetJSON {
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}
	var budget struct {
		Daily      *int64          `json:"daily_limit_cents"`
		Reserved   int64           `json:"reserved_cents"`
		Remaining  *int64          `json:"remaining_cents"`
		Currency   string          `json:"currency"`
		Day        string          `json:"admission_day"`
		ResetsAt   string          `json:"resets_at"`
		Scope      string          `json:"scope"`
		Accounting json.RawMessage `json:"accounting"`
		Exclusions json.RawMessage `json:"exclusions"`
	}
	if err := json.Unmarshal(raw, &budget); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if budget.Daily == nil {
		fmt.Fprintln(out, "Daily cap: unset (no daily limit)")
	} else {
		fmt.Fprintf(out, "Daily cap: %d %s cents\n", *budget.Daily, budget.Currency)
	}
	fmt.Fprintf(out, "Reserved: %d %s cents\n", budget.Reserved, budget.Currency)
	if budget.Remaining == nil {
		fmt.Fprintln(out, "Remaining: uncapped")
	} else {
		fmt.Fprintf(out, "Remaining: %d %s cents\n", *budget.Remaining, budget.Currency)
	}
	fmt.Fprintf(out, "Admission day (UTC): %s\nResets at (UTC): %s\nScope: %s\n", budget.Day, budget.ResetsAt, budget.Scope)
	if len(budget.Accounting) > 0 {
		fmt.Fprintf(out, "Accounting: %s\n", budget.Accounting)
	}
	if len(budget.Exclusions) > 0 {
		fmt.Fprintf(out, "Exclusions: %s\n", budget.Exclusions)
	}
	return nil
}
